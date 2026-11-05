package graphplan

import (
	"fmt"
	"reflect"
	"testing"
)

func layerNode(laID int64, actor, title string, x, y float64) LayerNode {
	return LayerNode{LaID: laID, ActorID: actor, Title: title, X: x, Y: y}
}

func layerLink(id, source, target string, laSource, laTarget int64) LayerLink {
	return LayerLink{EdgeID: id, Source: source, Target: target, LaIDSource: laSource, LaIDTarget: laTarget}
}

func TestSnapLayerCoordRoundsHalvesUp(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, 0}, {24, 0}, {25, 50}, {74, 50}, {75, 100},
		{-24, 0}, {-25, 0}, {-26, -50}, {-75, -50}, {-76, -100},
	} {
		if got := snapLayerCoord(tc.in); got != tc.want {
			t.Errorf("snapLayerCoord(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
	// Positions decode like legacy jsonInt (math.Round) before snapping.
	for _, tc := range []struct {
		in   float64
		want int
	}{{24.5, 50}, {24.4, 0}, {-24.5, 0}, {-25.5, -50}} {
		if got := snapLayerCoord(legacyCoord(tc.in)); got != tc.want {
			t.Errorf("snap(legacyCoord(%v)) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestCompactOptionsDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   CompactOptions
		want compactConfig
	}{
		{"zero", CompactOptions{}, compactConfig{3, 4, 4, 130, 95}},
		{"fraction truncates to default", CompactOptions{NodeDX: 0.9, NodeDY: -0.5}, compactConfig{3, 4, 4, 130, 95}},
		{"explicit", CompactOptions{2, 1, 2, 100.7, 80}, compactConfig{2, 1, 2, 100, 80}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.resolve(); got != tc.want {
				t.Fatalf("resolve() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestCompactLayout(t *testing.T) {
	hubLinks := []LayerLink{
		layerLink("e1", "c1", "h", 11, 10),
		layerLink("e2", "c2", "h", 12, 10),
		layerLink("e3", "c3", "h", 13, 10),
	}
	hubNodes := []LayerNode{
		layerNode(10, "h", "Hub", 0, 0),
		layerNode(11, "c1", "c", 0, 0),
		layerNode(12, "c2", "a", 0, 0),
		layerNode(13, "c3", "B", 0, 0),
		layerNode(14, "z", "Zed", 0, 0),
	}
	for _, tc := range []struct {
		name  string
		nodes []LayerNode
		links []LayerLink
		opts  CompactOptions
		want  []Move
		stats CompactStats
	}{
		{
			name:  "empty layer",
			stats: CompactStats{},
		},
		{
			// The legacy handler test: three unlinked nodes at the origin all move.
			name: "leftovers only",
			nodes: []LayerNode{
				layerNode(3, "a3", "C", 0, 0), layerNode(1, "a1", "A", 0, 0), layerNode(2, "a2", "B", 0, 0),
			},
			want:  []Move{{1, 200, 350}, {2, 350, 350}, {3, 450, 350}},
			stats: CompactStats{LeftoverPositioned: 3, PlacementsMoved: 3},
		},
		{
			name:  "bucket cluster with Misc zone",
			nodes: hubNodes,
			links: hubLinks,
			// Hub (395,250); children a,B,c at y=345, x=200/330/460; Zed at (200,630).
			want:  []Move{{10, 400, 250}, {11, 450, 350}, {12, 200, 350}, {13, 350, 350}, {14, 200, 650}},
			stats: CompactStats{Clusters: 1, BucketActors: 1, ChildrenPositioned: 3, LeftoverPositioned: 1, PlacementsMoved: 5},
		},
		{
			name:  "threshold above in-degree makes everything leftover",
			nodes: hubNodes,
			links: hubLinks,
			opts:  CompactOptions{BucketThreshold: 4},
			// Title order: a, B, c, Hub, Zed.
			want:  []Move{{10, 600, 350}, {11, 450, 350}, {12, 200, 350}, {13, 350, 350}, {14, 700, 350}},
			stats: CompactStats{LeftoverPositioned: 5, PlacementsMoved: 5},
		},
		{
			name:  "custom options wrap cluster rows and grow cluster height",
			nodes: hubNodes,
			links: hubLinks,
			opts:  CompactOptions{BucketThreshold: 2, ClustersPerRow: 1, NodesPerRow: 2, NodeDX: 100, NodeDY: 100},
			// Hub (250,250); a (200,350), B (300,350), c (200,450); Zed at y=250+390+100.
			want:  []Move{{10, 250, 250}, {11, 200, 450}, {12, 200, 350}, {13, 300, 350}, {14, 200, 750}},
			stats: CompactStats{Clusters: 1, BucketActors: 1, ChildrenPositioned: 3, LeftoverPositioned: 1, PlacementsMoved: 5},
		},
		{
			name: "skip placements already on their cell",
			nodes: []LayerNode{
				layerNode(1, "a1", "A", 210, 340), layerNode(2, "a2", "B", 0, 0),
			},
			want:  []Move{{2, 350, 350}},
			stats: CompactStats{LeftoverPositioned: 2, PlacementsMoved: 1},
		},
		{
			// A is placed twice; its extra placement is nudged past B (stationary) and C.
			name: "occupied cells are avoided",
			nodes: []LayerNode{
				layerNode(1, "a", "A", 0, 0), layerNode(5, "a", "A", 0, 0),
				layerNode(2, "b", "B", 340, 360), layerNode(3, "c", "C", 0, 0),
			},
			want:  []Move{{1, 200, 350}, {3, 450, 350}, {5, 600, 350}},
			stats: CompactStats{LeftoverPositioned: 3, PlacementsMoved: 3},
		},
		{
			name: "nudge wraps to the next row after a full row width",
			nodes: []LayerNode{
				layerNode(1, "a", "A", 0, 0), layerNode(2, "a", "A", 0, 0), layerNode(3, "a", "A", 0, 0),
			},
			opts: CompactOptions{ClustersPerRow: 1, NodesPerRow: 2},
			// rowWidth 2: step 1 → (+130, 0) → x 350; step 2 → (0, +95) → y 450.
			want:  []Move{{1, 200, 350}, {2, 350, 350}, {3, 200, 450}},
			stats: CompactStats{LeftoverPositioned: 1, PlacementsMoved: 3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			moves, stats := CompactLayout(tc.nodes, tc.links, tc.opts)
			if len(moves) == 0 && len(tc.want) == 0 {
				moves = nil
			}
			if !reflect.DeepEqual(moves, tc.want) {
				t.Errorf("moves = %v, want %v", moves, tc.want)
			}
			if stats != tc.stats {
				t.Errorf("stats = %+v, want %+v", stats, tc.stats)
			}
		})
	}
}

func TestCompactLayoutGreedyClusterAndBucketOrder(t *testing.T) {
	nodes := []LayerNode{
		layerNode(1, "b2", "Second", 0, 0), layerNode(2, "b1", "First", 0, 0),
		layerNode(3, "p1", "p1", 0, 0), layerNode(4, "p2", "p2", 0, 0), layerNode(5, "p3", "p3", 0, 0),
		layerNode(6, "q1", "q1", 0, 0), layerNode(7, "q2", "q2", 0, 0), layerNode(8, "x", "x", 0, 0),
	}
	links := []LayerLink{
		layerLink("l1", "q1", "b2", 6, 1), layerLink("l2", "q2", "b2", 7, 1), layerLink("l3", "x", "b2", 8, 1),
		layerLink("l4", "p1", "b1", 3, 2), layerLink("l5", "p2", "b1", 4, 2), layerLink("l6", "p3", "b1", 5, 2),
		layerLink("l7", "x", "b1", 8, 2),
	}
	moves, stats := CompactLayout(nodes, links, CompactOptions{})
	want := CompactStats{Clusters: 2, BucketActors: 2, ChildrenPositioned: 6, PlacementsMoved: 8}
	if stats != want {
		t.Fatalf("stats = %+v, want %+v", stats, want)
	}
	got := map[int64]Move{}
	for _, m := range moves {
		got[m.LaID] = m
	}
	// b1 (in-degree 4) heads cluster 0 at x 200+195; b2 heads cluster 1 at 820+195.
	// x links to both and joins the earlier cluster as its fourth child (x 590 → 600).
	for laID, w := range map[int64]Move{2: {2, 400, 250}, 1: {1, 1000, 250}, 8: {8, 600, 350}, 6: {6, 800, 350}} {
		if got[laID] != w {
			t.Errorf("move for laId %d = %v, want %v", laID, got[laID], w)
		}
	}
}

func TestCompactLayoutIsDeterministicAndIdempotent(t *testing.T) {
	// Equal titles and in-degrees leave only the laId tie-break to order actors.
	var nodes []LayerNode
	var links []LayerLink
	for i := int64(1); i <= 12; i++ {
		nodes = append(nodes, layerNode(i, fmt.Sprintf("actor-%02d", i), "Same", 0, 0))
	}
	for i := int64(3); i <= 12; i++ {
		target := int64(1 + i%2)
		links = append(links, layerLink(fmt.Sprintf("e%d", i), fmt.Sprintf("actor-%02d", i),
			fmt.Sprintf("actor-%02d", target), i, target))
	}
	first, firstStats := CompactLayout(nodes, links, CompactOptions{})
	for run := 0; run < 5; run++ {
		reversed := make([]LayerNode, len(nodes))
		for i, n := range nodes {
			reversed[len(nodes)-1-i] = n
		}
		moves, stats := CompactLayout(reversed, links, CompactOptions{})
		if !reflect.DeepEqual(moves, first) || stats != firstStats {
			t.Fatalf("run %d differs: %v %+v vs %v %+v", run, moves, stats, first, firstStats)
		}
	}
	at := map[int64]Move{}
	for _, m := range first {
		at[m.LaID] = m
	}
	for i := range nodes {
		nodes[i].X, nodes[i].Y = at[nodes[i].LaID].X, at[nodes[i].LaID].Y
	}
	if again, stats := CompactLayout(nodes, links, CompactOptions{}); len(again) != 0 || stats.PlacementsMoved != 0 {
		t.Fatalf("second run moved %v (%+v), want none", again, stats)
	}
}

func pruneFixture() ([]LayerNode, []LayerLink) {
	nodes := []LayerNode{
		layerNode(1, "a", "A", 0, 0),
		layerNode(2, "b", "B", 500, 100),
		layerNode(3, "c", "C", 601, 0),
		layerNode(4, "h", "H", 2000, 0),
	}
	links := []LayerLink{
		layerLink("e1", "a", "b", 1, 2),     // d=600: short
		layerLink("e2", "a", "c", 1, 3),     // d=601: long
		layerLink("e3", "a", "h", 1, 4),     // d=2000, h is a bucket
		layerLink("e4", "b", "h", 2, 4),     // d=1600, h is a bucket
		layerLink("e5", "c", "h", 3, 4),     // d=1399, h is a bucket
		layerLink("e6", "a", "ghost", 1, 9), // target placement missing
	}
	return nodes, links
}

func TestPruneLongEdges(t *testing.T) {
	nodes, links := pruneFixture()
	for _, tc := range []struct {
		name  string
		links []LayerLink
		opts  PruneOptions
		ids   []string
		stats PruneStats
	}{
		{
			name: "defaults keep short, missing and parent edges",
			opts: PruneOptions{KeepParentEdges: true},
			ids:  []string{"e2"},
			stats: PruneStats{Scanned: 6, Deleted: 1, KeptShort: 2, KeptParent: 3,
				Examples: []string{"A → C (d=601)"}},
		},
		{
			name: "without parent preservation long bucket edges go",
			ids:  []string{"e2", "e3", "e4", "e5"},
			stats: PruneStats{Scanned: 6, Deleted: 4, KeptShort: 2, Examples: []string{
				"A → C (d=601)", "A → H (d=2000)", "B → H (d=1600)", "C → H (d=1399)"}},
		},
		{
			name:  "higher bucket threshold drops parent status",
			opts:  PruneOptions{KeepParentEdges: true, BucketThreshold: 4},
			ids:   []string{"e2", "e3", "e4", "e5"},
			stats: PruneStats{Scanned: 6, Deleted: 4, KeptShort: 2},
		},
		{
			name:  "large max distance keeps everything",
			opts:  PruneOptions{MaxDistance: 5000},
			stats: PruneStats{Scanned: 6, KeptShort: 6},
		},
		{
			name:  "fractional max distance truncates like legacy",
			opts:  PruneOptions{MaxDistance: 600.9, KeepParentEdges: true},
			ids:   []string{"e2"},
			stats: PruneStats{Scanned: 6, Deleted: 1, KeptShort: 2, KeptParent: 3},
		},
		{
			name:  "sub-pixel max distance takes the default",
			opts:  PruneOptions{MaxDistance: 0.5, KeepParentEdges: true},
			ids:   []string{"e2"},
			stats: PruneStats{Scanned: 6, Deleted: 1, KeptShort: 2, KeptParent: 3},
		},
		{
			name:  "missing source placement is kept short",
			links: []LayerLink{layerLink("m", "ghost", "c", 42, 3)},
			stats: PruneStats{Scanned: 1, KeptShort: 1},
		},
		{
			name:  "repeated edge id is deleted once",
			links: []LayerLink{layerLink("e2", "a", "c", 1, 3), layerLink("e2", "a", "c", 1, 3)},
			ids:   []string{"e2"},
			stats: PruneStats{Scanned: 2, Deleted: 1, Examples: []string{"A → C (d=601)"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := links
			if tc.links != nil {
				in = tc.links
			}
			ids, stats := PruneLongEdges(nodes, in, tc.opts)
			if !reflect.DeepEqual(ids, tc.ids) {
				t.Errorf("ids = %v, want %v", ids, tc.ids)
			}
			if tc.stats.Examples == nil && stats.Deleted > 0 {
				stats.Examples = nil // example text is asserted in the rows that list it
			}
			if !reflect.DeepEqual(stats, tc.stats) {
				t.Errorf("stats = %+v, want %+v", stats, tc.stats)
			}
		})
	}
}

func TestPruneLongEdgesUsesPlacementPositionsAndCapsExamples(t *testing.T) {
	// The same actor placed twice: the link's laIds pick which placement counts.
	nodes := []LayerNode{
		layerNode(1, "a", "A", 0, 0), layerNode(2, "a", "A", 5000, 0), layerNode(3, "b", "B", 100.4, 0),
	}
	near := []LayerLink{layerLink("near", "a", "b", 1, 3), layerLink("far", "a", "b", 2, 3)}
	ids, stats := PruneLongEdges(nodes, near, PruneOptions{MaxDistance: 100})
	if !reflect.DeepEqual(ids, []string{"far"}) || stats.KeptShort != 1 || stats.Examples[0] != "A → B (d=4900)" {
		t.Fatalf("ids = %v, stats = %+v", ids, stats)
	}

	var many []LayerLink
	for i := 0; i < 12; i++ {
		many = append(many, layerLink(fmt.Sprintf("e%02d", i), "a", "b", 2, 3))
	}
	ids, stats = PruneLongEdges(nodes, many, PruneOptions{})
	if len(ids) != 12 || stats.Deleted != 12 || len(stats.Examples) != maxPruneExamples {
		t.Fatalf("deleted %d, examples %d: %+v", len(ids), len(stats.Examples), stats)
	}
	if stats.DryRun || stats.Errors != nil {
		t.Fatalf("caller-owned fields set: %+v", stats)
	}
}
