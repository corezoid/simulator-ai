package graphplan

// Pure ports of the legacy compactGraphLayout and pruneLongEdges graph tools
// (simulator-ai-plugin engines/graph). The legacy handlers read a layer over HTTP,
// decided, and wrote back; these functions take the layer as arguments and return
// the decisions, keeping the legacy defaults, geometry and counters.

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// LayerNode is one node placement on a layer. An actor may be placed more than once.
type LayerNode struct {
	LaID    int64
	ActorID string
	Title   string
	X, Y    float64
}

// LayerLink is one edge placement on a layer. Source and Target are actor ids;
// LaIDSource and LaIDTarget are the endpoint node placements.
type LayerLink struct {
	EdgeID, Source, Target string
	LaIDSource, LaIDTarget int64
}

// CompactOptions tunes CompactLayout. A zero field takes the legacy default
// (3, 4, 4, 130, 95). NodeDX/NodeDY truncate toward zero first, like the legacy
// toInt argument decoding, so a value in (-1, 1) also takes the default.
type CompactOptions struct {
	BucketThreshold, ClustersPerRow, NodesPerRow int
	NodeDX, NodeDY                               float64
}

// CompactStats are the legacy compactGraphLayout counters.
type CompactStats struct {
	Clusters           int `json:"clusters"`
	BucketActors       int `json:"bucketActors"`
	ChildrenPositioned int `json:"childrenPositioned"`
	LeftoverPositioned int `json:"leftoverPositioned"`
	PlacementsMoved    int `json:"placementsMoved"`
}

// Move sends one node placement to a snapped grid cell.
type Move struct {
	LaID int64   `json:"laId"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

// PruneOptions tunes PruneLongEdges. MaxDistance truncates toward zero and then
// takes 600 when <= 0; BucketThreshold 0 takes 3. KeepParentEdges has no implicit
// default here: callers pass the tool default (true) explicitly.
type PruneOptions struct {
	MaxDistance     float64
	BucketThreshold int
	KeepParentEdges bool
}

// PruneStats are the legacy pruneLongEdges counters.
type PruneStats struct {
	Scanned    int      `json:"scanned"`
	Deleted    int      `json:"deleted"`
	KeptShort  int      `json:"kept_short"`
	KeptParent int      `json:"kept_parent"`
	DryRun     bool     `json:"dryRun"`
	Examples   []string `json:"deletedExamples,omitempty"`
	Errors     []string `json:"errors,omitempty"`
}

// layerActorGrid mirrors pong-server's ACTOR_SIZE: positions snap to this grid.
const layerActorGrid = 50

// Legacy tool defaults.
const (
	defaultBucketThreshold = 3
	defaultClustersPerRow  = 4
	defaultNodesPerRow     = 4
	defaultNodeDX          = 130
	defaultNodeDY          = 95
	defaultMaxDistance     = 600
	maxPruneExamples       = 10
)

// Legacy super-grid geometry, in pixels.
const (
	clusterInset = 40  // extra width/height of a cluster cell
	clusterGapX  = 60  // horizontal gap between clusters
	clusterGapY  = 50  // vertical gap between cluster rows
	gridPadX     = 200 // left edge of the super-grid
	gridPadY     = 250 // top edge of the super-grid
	leftoverGapY = 100 // gap between the last cluster row and the Misc zone
)

// legacyArg mirrors the legacy toInt of a numeric tool argument (truncation toward zero).
func legacyArg(v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return int(v)
}

// legacyCoord mirrors the legacy jsonInt decoding of a position (nearest, halves away from zero).
func legacyCoord(v float64) int {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return int(math.Round(v))
}

// snapLayerCoord replicates pong-server's snapGeomCoord: JS Math.round rounds halves
// toward +Inf, so floor(v/size+0.5) rather than Go's math.Round.
func snapLayerCoord(v int) int {
	return int(math.Floor(float64(v)/layerActorGrid+0.5)) * layerActorGrid
}

func orDefault(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

type layerCell struct{ x, y int }

type compactConfig struct {
	bucketThreshold, clustersPerRow, nodesPerRow, nodeDX, nodeDY int
}

func (o CompactOptions) resolve() compactConfig {
	return compactConfig{
		bucketThreshold: orDefault(o.BucketThreshold, defaultBucketThreshold),
		clustersPerRow:  orDefault(o.ClustersPerRow, defaultClustersPerRow),
		nodesPerRow:     orDefault(o.NodesPerRow, defaultNodesPerRow),
		nodeDX:          orDefault(legacyArg(o.NodeDX), defaultNodeDX),
		nodeDY:          orDefault(legacyArg(o.NodeDY), defaultNodeDY),
	}
}

// layerActor is one distinct actor of a layer. Title is its last placement's title,
// as the legacy titleByActor map kept; minLaID is the stable tie-break.
type layerActor struct {
	id      string
	title   string
	minLaID int64
}

// collectActors lists distinct actors in first-placement order.
func collectActors(nodes []LayerNode) []*layerActor {
	byID := map[string]*layerActor{}
	var actors []*layerActor
	for _, n := range nodes {
		a, ok := byID[n.ActorID]
		if !ok {
			a = &layerActor{id: n.ActorID, minLaID: n.LaID}
			byID[n.ActorID] = a
			actors = append(actors, a)
		}
		a.title = n.Title
		if n.LaID < a.minLaID {
			a.minLaID = n.LaID
		}
	}
	return actors
}

// byTitle is the legacy case-insensitive title order with deterministic tie-breaks.
func byTitle(a, b *layerActor) bool {
	ta, tb := strings.ToLower(a.title), strings.ToLower(b.title)
	if ta != tb {
		return ta < tb
	}
	if a.minLaID != b.minLaID {
		return a.minLaID < b.minLaID
	}
	return a.id < b.id
}

// CompactLayout repositions every placement into a domain-clustered grid: actors with
// at least BucketThreshold incoming links head clusters, their linked actors fill a grid
// below them, and the rest stack into a Misc zone. It returns one Move per placement that
// is not already on its snapped cell, in laId order, each on a distinct free cell.
func CompactLayout(nodes []LayerNode, links []LayerLink, o CompactOptions) ([]Move, CompactStats) {
	cfg := o.resolve()
	actors := collectActors(nodes)
	incoming := map[string]int{}
	neighbours := map[string][]string{} // any incident link makes a parent candidate
	for _, l := range links {
		incoming[l.Target]++
		neighbours[l.Source] = append(neighbours[l.Source], l.Target)
		neighbours[l.Target] = append(neighbours[l.Target], l.Source)
	}
	buckets := pickBuckets(actors, incoming, cfg.bucketThreshold)
	children := assignClusters(actors, buckets, neighbours)
	positions, clusterH := placeClusters(cfg, buckets, children)
	leftover := placeLeftovers(cfg, actors, positions, len(buckets), clusterH)

	stats := CompactStats{Clusters: len(buckets), BucketActors: len(buckets), LeftoverPositioned: leftover}
	for _, kids := range children {
		stats.ChildrenPositioned += len(kids)
	}
	moves := planMoves(cfg, nodes, positions)
	stats.PlacementsMoved = len(moves)
	return moves, stats
}

// pickBuckets returns the bucket actors by descending in-degree, then title.
func pickBuckets(actors []*layerActor, incoming map[string]int, threshold int) []*layerActor {
	var buckets []*layerActor
	for _, a := range actors {
		if incoming[a.id] >= threshold {
			buckets = append(buckets, a)
		}
	}
	sort.SliceStable(buckets, func(i, j int) bool {
		di, dj := incoming[buckets[i].id], incoming[buckets[j].id]
		if di != dj {
			return di > dj
		}
		return byTitle(buckets[i], buckets[j])
	})
	return buckets
}

// assignClusters puts each non-bucket actor under the earliest bucket it links to and
// returns the title-sorted children per bucket index.
func assignClusters(actors, buckets []*layerActor, neighbours map[string][]string) [][]*layerActor {
	index := make(map[string]int, len(buckets))
	for i, b := range buckets {
		index[b.id] = i
	}
	children := make([][]*layerActor, len(buckets))
	for _, a := range actors {
		if _, isBucket := index[a.id]; isBucket {
			continue
		}
		best := -1
		for _, p := range neighbours[a.id] {
			if i, ok := index[p]; ok && (best < 0 || i < best) {
				best = i
			}
		}
		if best >= 0 {
			children[best] = append(children[best], a)
		}
	}
	for _, kids := range children {
		sort.SliceStable(kids, func(i, j int) bool { return byTitle(kids[i], kids[j]) })
	}
	return children
}

// placeClusters lays buckets in the super-grid and children in rows beneath them. The
// cluster height fits the largest cluster so cluster rows never overlap.
func placeClusters(cfg compactConfig, buckets []*layerActor, children [][]*layerActor) (map[string]layerCell, int) {
	maxChildren := 0
	for _, kids := range children {
		maxChildren = max(maxChildren, len(kids))
	}
	rowsPerCluster := max((maxChildren+cfg.nodesPerRow-1)/cfg.nodesPerRow, 1)
	clusterW := cfg.nodesPerRow*cfg.nodeDX + clusterInset
	clusterH := (rowsPerCluster+1)*cfg.nodeDY + clusterInset

	positions := map[string]layerCell{}
	for idx, b := range buckets {
		cx := gridPadX + (idx%cfg.clustersPerRow)*(clusterW+clusterGapX)
		cy := gridPadY + (idx/cfg.clustersPerRow)*(clusterH+clusterGapY)
		positions[b.id] = layerCell{x: cx + (cfg.nodesPerRow-1)*cfg.nodeDX/2, y: cy}
		for k, child := range children[idx] {
			positions[child.id] = layerCell{
				x: cx + (k%cfg.nodesPerRow)*cfg.nodeDX,
				y: cy + cfg.nodeDY + (k/cfg.nodesPerRow)*cfg.nodeDY,
			}
		}
	}
	return positions, clusterH
}

// placeLeftovers stacks unclustered actors in title order below the super-grid and
// returns how many it placed.
func placeLeftovers(cfg compactConfig, actors []*layerActor, positions map[string]layerCell, buckets, clusterH int) int {
	var leftover []*layerActor
	for _, a := range actors {
		if _, placed := positions[a.id]; !placed {
			leftover = append(leftover, a)
		}
	}
	sort.SliceStable(leftover, func(i, j int) bool { return byTitle(leftover[i], leftover[j]) })
	leftoverRow := (buckets + cfg.clustersPerRow - 1) / cfg.clustersPerRow
	top := gridPadY + leftoverRow*(clusterH+clusterGapY) + leftoverGapY
	perRow := cfg.clustersPerRow * cfg.nodesPerRow
	for k, a := range leftover {
		positions[a.id] = layerCell{x: gridPadX + (k%perRow)*cfg.nodeDX, y: top + (k/perRow)*cfg.nodeDY}
	}
	return len(leftover)
}

// planMoves snaps every placement's target. A placement already on its cell is skipped
// and reserves the cell (pong rejects a move onto a stationary node, including itself);
// movers then take free cells in laId order, nudged right and down on a collision.
func planMoves(cfg compactConfig, nodes []LayerNode, positions map[string]layerCell) []Move {
	ordered := append([]LayerNode(nil), nodes...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].LaID < ordered[j].LaID })

	type pendingMove struct {
		laID   int64
		target layerCell
	}
	occupied := map[layerCell]bool{}
	var movers []pendingMove
	for _, n := range ordered {
		pos, ok := positions[n.ActorID]
		if !ok {
			continue
		}
		target := layerCell{x: snapLayerCoord(pos.x), y: snapLayerCoord(pos.y)}
		current := layerCell{x: snapLayerCoord(legacyCoord(n.X)), y: snapLayerCoord(legacyCoord(n.Y))}
		if target == current {
			occupied[current] = true
			continue
		}
		movers = append(movers, pendingMove{laID: n.LaID, target: target})
	}

	rowWidth := max(cfg.nodesPerRow*cfg.clustersPerRow, 1)
	moves := make([]Move, 0, len(movers))
	for _, m := range movers {
		cell := m.target
		for step := 1; occupied[cell]; step++ {
			cell = layerCell{
				x: snapLayerCoord(m.target.x + (step%rowWidth)*cfg.nodeDX),
				y: snapLayerCoord(m.target.y + (step/rowWidth)*cfg.nodeDY),
			}
		}
		occupied[cell] = true
		moves = append(moves, Move{LaID: m.laID, X: float64(cell.x), Y: float64(cell.y)})
	}
	return moves
}

// PruneLongEdges returns the edge ids to delete (distinct, in input order) and the stats as if all deletions succeed (Deleted = len(ids)); the caller sets DryRun and appends Errors.
//
// A link is kept short when an endpoint placement is missing or the Manhattan distance
// between its endpoint placements is at most MaxDistance; with KeepParentEdges it is kept
// as a parent edge when either endpoint actor has at least BucketThreshold incoming links.
// Examples hold up to ten "Source → Target (d=N)" titles. A repeated edge id is scanned
// but neither deleted again nor counted.
func PruneLongEdges(nodes []LayerNode, links []LayerLink, o PruneOptions) ([]string, PruneStats) {
	maxDistance := legacyArg(o.MaxDistance)
	if maxDistance <= 0 {
		maxDistance = defaultMaxDistance
	}
	threshold := orDefault(o.BucketThreshold, defaultBucketThreshold)
	byLaID := make(map[int64]LayerNode, len(nodes))
	for _, n := range nodes {
		byLaID[n.LaID] = n
	}
	incoming := map[string]int{}
	for _, l := range links {
		incoming[l.Target]++
	}

	var stats PruneStats
	var ids []string
	seen := map[string]bool{}
	for _, l := range links {
		stats.Scanned++
		src, sourceOK := byLaID[l.LaIDSource]
		dst, targetOK := byLaID[l.LaIDTarget]
		if !sourceOK || !targetOK {
			stats.KeptShort++
			continue
		}
		dist := manhattan(src, dst)
		if dist <= maxDistance {
			stats.KeptShort++
			continue
		}
		if o.KeepParentEdges && (incoming[l.Source] >= threshold || incoming[l.Target] >= threshold) {
			stats.KeptParent++
			continue
		}
		if seen[l.EdgeID] {
			continue
		}
		seen[l.EdgeID] = true
		ids = append(ids, l.EdgeID)
		if len(stats.Examples) < maxPruneExamples {
			stats.Examples = append(stats.Examples, fmt.Sprintf("%s → %s (d=%d)", src.Title, dst.Title, dist))
		}
	}
	stats.Deleted = len(ids)
	return ids, stats
}

// manhattan is the legacy integer distance between two placements' decoded positions.
func manhattan(a, b LayerNode) int {
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	return abs(legacyCoord(a.X)-legacyCoord(b.X)) + abs(legacyCoord(a.Y)-legacyCoord(b.Y))
}
