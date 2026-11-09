package simfake

import (
	"encoding/json"
	"fmt"
)

type point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

func (s *Server) id(prefix string) string {
	s.Seq++
	return fmt.Sprintf("%s-%06d", prefix, s.Seq)
}

func (s *Server) run(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "batch":
		return s.batch(params)
	case "actor/create":
		return s.actorCreate(params)
	case "actor/by-ref/get":
		var p struct {
			FormID int64  `json:"formId"`
			Ref    string `json:"ref"`
		}
		_ = json.Unmarshal(params, &p)
		a := s.Actors[s.Refs[fmt.Sprintf("%d|%s", p.FormID, p.Ref)]]
		if a == nil {
			return nil, fail("notFound", "actor not found")
		}
		return actorJSON(a), nil
	case "actor/get":
		var p struct {
			ActorID string `json:"actorId"`
		}
		_ = json.Unmarshal(params, &p)
		out := map[string]any{"id": p.ActorID, "data": map[string]any{"anisotropy": false}}
		if a := s.Actors[p.ActorID]; a != nil {
			out["color"], out["formId"] = a.Color, a.FormID
		}
		return out, nil
	case "actor/delete":
		return s.actorDelete(params)
	case "form/system/get":
		return map[string]any{"id": 7, "title": "Layers", "type": "system", "form": map[string]any{}}, nil
	case "form/flowchart-block/search":
		return json.RawMessage(s.Blocks), nil
	case "edge_type/get":
		var p struct {
			Name string `json:"name"`
		}
		_ = json.Unmarshal(params, &p)
		switch p.Name {
		case "hierarchy":
			return map[string]any{"id": 3, "name": p.Name}, nil
		case "flow":
			return map[string]any{"id": 4, "name": p.Name}, nil
		}
		return nil, fail("notFound", "edge type not found")
	case "edge/create":
		return s.edgeCreate(params)
	case "graph/layer/actors/manage":
		return s.manage(params)
	case "graph/layer/actors/list":
		return s.nodesList(params)
	case "graph/layer/edges/list":
		return s.edgesList(params)
	case "graph/layer/actors/positions/update":
		return s.positions(params)
	case "graph/layer/stats/get":
		var p struct {
			ActorID string `json:"actorId"`
		}
		_ = json.Unmarshal(params, &p)
		nodes, edges := 0, 0
		for _, n := range s.Nodes {
			if n.Layer == p.ActorID {
				nodes++
			}
		}
		for _, e := range s.Placements {
			if e.Layer == p.ActorID {
				edges++
			}
		}
		return map[string]any{"nodes": nodes, "edges": edges}, nil
	}
	if run, ok := appMethods[method]; ok {
		return run(s, params)
	}
	return nil, fail("methodNotFound", method)
}

func actorJSON(a *Actor) map[string]any {
	return map[string]any{"id": a.ID, "title": a.Title, "formId": a.FormID, "ref": a.Ref, "color": a.Color}
}

func (s *Server) actorCreate(params json.RawMessage) (any, *rpcError) {
	var p struct {
		FormID      int64           `json:"formId"`
		Title       string          `json:"title"`
		Ref         string          `json:"ref"`
		Color       string          `json:"color"`
		Data        json.RawMessage `json:"data"`
		AddToRecent *bool           `json:"addToRecent"`
	}
	_ = json.Unmarshal(params, &p)
	if p.FormID < 1 || len(p.Data) == 0 || p.FormID == s.BadForm {
		return nil, fail("invalidParams", "data")
	}
	key := fmt.Sprintf("%d|%s", p.FormID, p.Ref)
	if _, taken := s.Refs[key]; p.Ref != "" && taken {
		return nil, fail("invalidParams", "duplicate: ref, formId")
	}
	a := &Actor{ID: s.id("actor"), Title: p.Title, Ref: p.Ref, Color: p.Color, FormID: p.FormID, Recent: p.AddToRecent == nil || *p.AddToRecent}
	s.Actors[a.ID] = a
	if p.Ref != "" {
		s.Refs[key] = a.ID
	}
	return actorJSON(a), nil
}

func (s *Server) actorDelete(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID string `json:"actorId"`
	}
	_ = json.Unmarshal(params, &p)
	if s.Actors[p.ActorID] == nil {
		return nil, fail("notFound", "actor not found")
	}
	delete(s.Actors, p.ActorID)
	gone := map[int64]bool{}
	nodes := s.Nodes[:0:0]
	for _, n := range s.Nodes {
		if n.Actor == p.ActorID {
			gone[n.LaID] = true
			continue
		}
		nodes = append(nodes, n)
	}
	placements := s.Placements[:0:0]
	for _, e := range s.Placements {
		if !gone[e.Src] && !gone[e.Dst] {
			placements = append(placements, e)
		}
	}
	s.Nodes, s.Placements = nodes, placements
	for key, e := range s.EdgeKeys {
		if edge := s.Edges[e]; edge.Source == p.ActorID || edge.Target == p.ActorID {
			delete(s.Edges, e)
			delete(s.EdgeKeys, key)
		}
	}
	return map[string]any{}, nil
}

func (s *Server) edgeCreate(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Source, Target, Name string
		EdgeTypeID           int64 `json:"edgeTypeId"`
	}
	_ = json.Unmarshal(params, &p)
	if s.Actors[p.Source] == nil || s.Actors[p.Target] == nil {
		return nil, fail("notFound", "edge end not found")
	}
	key := fmt.Sprintf("%s>%s|%d", p.Source, p.Target, p.EdgeTypeID)
	e := s.Edges[s.EdgeKeys[key]]
	if e == nil { // the natural key: an existing edge is returned unchanged
		e = &Edge{ID: s.id("edge"), Source: p.Source, Target: p.Target, Name: p.Name, TypeID: p.EdgeTypeID}
		s.Edges[e.ID], s.EdgeKeys[key] = e, e.ID
	}
	return map[string]any{"id": e.ID, "edgeTypeId": e.TypeID, "source": e.Source, "target": e.Target, "name": e.Name}, nil
}

func (s *Server) manage(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID string `json:"actorId"`
		Items   []struct {
			Action string `json:"action"`
			Data   struct {
				ID            string          `json:"id"`
				Type          string          `json:"type"`
				Position      *point          `json:"position"`
				LaIDSource    int64           `json:"laIdSource"`
				LaIDTarget    int64           `json:"laIdTarget"`
				LayerSettings json.RawMessage `json:"layerSettings"`
				AreaPicture   json.RawMessage `json:"areaPicture"`
			} `json:"data"`
		} `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.Items) < 1 || len(p.Items) > MaxLayerItems {
		return nil, fail("invalidParams", "items: 1-500")
	}
	nodes, edges := []any{}, []any{}
	onLayer := map[int64]bool{}
	for _, n := range s.Nodes {
		if n.Layer == p.ActorID {
			onLayer[n.LaID] = true
		}
	}
	for _, it := range p.Items {
		d := it.Data
		switch {
		case it.Action != "create":
			return nil, fail("invalidParams", "the fake only creates")
		case d.Type == "node":
			if s.Actors[d.ID] == nil || d.Position == nil {
				return nil, fail("notFound", "actor to place not found: "+d.ID)
			}
			s.Seq++
			n := Node{LaID: s.Seq, Layer: p.ActorID, Actor: d.ID, X: d.Position.X, Y: d.Position.Y, LayerSettings: d.LayerSettings, AreaPicture: d.AreaPicture}
			if len(d.AreaPicture) == 0 { // a grid placement snaps to its cell; a graphic one keeps its place
				n.X, n.Y = cellOf(n.X), cellOf(n.Y)
			}
			s.Nodes, onLayer[n.LaID] = append(s.Nodes, n), true
			nodes = append(nodes, map[string]any{"laId": n.LaID, "actorId": n.Actor, "position": point{n.X, n.Y}})
		default:
			if s.Edges[d.ID] == nil || !onLayer[d.LaIDSource] || !onLayer[d.LaIDTarget] {
				return nil, fail("notFound", "edge or its end placements not found: "+d.ID)
			}
			s.Seq++
			s.Placements = append(s.Placements, Placement{LaID: s.Seq, Layer: p.ActorID, Edge: d.ID, Src: d.LaIDSource, Dst: d.LaIDTarget})
			edges = append(edges, map[string]any{"laId": s.Seq, "edgeId": d.ID, "laIdSource": d.LaIDSource, "laIdTarget": d.LaIDTarget})
		}
	}
	return map[string]any{"nodes": nodes, "edges": edges}, nil
}

type page struct {
	ActorID string `json:"actorId"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

func window(total int, p page) (int, int, *rpcError) {
	if p.Limit < 1 || p.Limit > 1000 {
		return 0, 0, fail("invalidParams", "limit: 1-1000")
	}
	return min(p.Offset, total), min(p.Offset+p.Limit, total), nil
}

func (s *Server) nodesList(params json.RawMessage) (any, *rpcError) {
	var p page
	_ = json.Unmarshal(params, &p)
	var rows []Node
	for _, n := range s.Nodes {
		if n.Layer == p.ActorID {
			rows = append(rows, n)
		}
	}
	from, to, err := window(len(rows)+s.Ghosts, p)
	if err != nil {
		return nil, err
	}
	list := []any{}
	for i := from; i < to; i++ {
		if i < s.Ghosts {
			continue // paged over, then dropped
		}
		n := rows[i-s.Ghosts]
		row := map[string]any{"laId": n.LaID, "id": n.Actor, "position": point{n.X, n.Y}}
		if len(n.LayerSettings) > 0 {
			row["layerSettings"] = n.LayerSettings
		}
		if len(n.AreaPicture) > 0 {
			row["areaPicture"] = n.AreaPicture
		}
		if a := s.Actors[n.Actor]; a != nil {
			row["actor"] = actorJSON(a)
		}
		list = append(list, row)
	}
	return map[string]any{"list": list}, nil
}

func (s *Server) edgesList(params json.RawMessage) (any, *rpcError) {
	var p page
	_ = json.Unmarshal(params, &p)
	var rows []Placement
	for _, e := range s.Placements {
		if e.Layer == p.ActorID {
			rows = append(rows, e)
		}
	}
	from, to, err := window(len(rows), p)
	if err != nil {
		return nil, err
	}
	list := []any{}
	for _, e := range rows[from:to] {
		row := map[string]any{"laId": e.LaID, "id": e.Edge, "laIdSource": e.Src, "laIdTarget": e.Dst}
		if edge := s.Edges[e.Edge]; edge != nil {
			row["edge"] = map[string]any{"source": edge.Source, "target": edge.Target, "name": edge.Name}
		}
		list = append(list, row)
	}
	return map[string]any{"list": list}, nil
}

// positions moves placements together; a target cell another placement keeps is refused.
func (s *Server) positions(params json.RawMessage) (any, *rpcError) {
	var p struct {
		ActorID string `json:"actorId"`
		Items   []struct {
			LaActorID int64 `json:"laActorId"`
			Position  point `json:"position"`
		} `json:"items"`
	}
	_ = json.Unmarshal(params, &p)
	if len(p.Items) < 1 || len(p.Items) > MaxLayerItems {
		return nil, fail("invalidParams", "items: 1-500")
	}
	target := map[int64]point{}
	for _, it := range p.Items {
		if _, twice := target[it.LaActorID]; twice {
			return nil, fail("invalidParams", "a placement twice in one call")
		}
		target[it.LaActorID] = point{cellOf(it.Position.X), cellOf(it.Position.Y)}
	}
	held := map[point]bool{}
	for _, n := range s.Nodes {
		if _, moving := target[n.LaID]; n.Layer == p.ActorID && !moving {
			held[point{cellOf(n.X), cellOf(n.Y)}] = true
		}
	}
	for _, t := range target {
		if held[t] {
			return nil, fail("invalidParams", "cell occupied")
		}
		held[t] = true
	}
	for i, n := range s.Nodes {
		if t, ok := target[n.LaID]; ok && n.Layer == p.ActorID {
			s.Nodes[i].X, s.Nodes[i].Y = t.X, t.Y
		}
	}
	return map[string]any{}, nil
}
