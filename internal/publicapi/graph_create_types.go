package publicapi

import "encoding/json"

// GraphActorInput is the creation subset of public actor/create. Data is its documented
// open form-value object (including native __form__<childId>:<itemId> keys), not authority.
type GraphActorInput struct {
	Operation
	FormID         int64           `json:"formId"`
	Title          string          `json:"title"`
	Description    string          `json:"description,omitempty"`
	Data           json.RawMessage `json:"data"`
	Color          string          `json:"color,omitempty"`
	ContextLayerID string          `json:"contextLayerId,omitempty"`
}

// GraphEdgeInput is the noncredential public edge/create subset. Existing natural-key
// edges may be returned unchanged, including reverse hierarchy orientation.
type GraphEdgeInput struct {
	Operation
	Source         string `json:"source"`
	Target         string `json:"target"`
	EdgeTypeID     int64  `json:"edgeTypeId"`
	Name           string `json:"name,omitempty"`
	LinkedActorID  string `json:"linkedActorId,omitempty"`
	CurveStyle     string `json:"curveStyle,omitempty"`
	ForceDirection bool   `json:"forceDirection,omitempty"`
}

// GraphCreatedEdge projects rpc_edge.edgeRes, without owner or linked actor enrichment.
type GraphCreatedEdge struct {
	ID            string  `json:"id"`
	EdgeTypeID    int64   `json:"edgeTypeId"`
	Source        string  `json:"source"`
	Target        string  `json:"target"`
	Name          *string `json:"name,omitempty"`
	LinkedActorID *string `json:"linkedActorId,omitempty"`
	CurveStyle    *string `json:"curveStyle,omitempty"`
}

// GraphPoint follows rpc_graph.positionReq and e_graph.Position.
type GraphPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// GraphFrame follows UI geometry Rect, persisted in e_graph.ActorLayerSettings.TextFrame.
type GraphFrame struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// GraphAreaPicture follows e_graph.AreaPicture; graphic placement avoids grid snapping.
type GraphAreaPicture struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Img    string  `json:"img"`
}

// GraphNativeSettings follows e_graph.ActorLayerSettings and its open Extra blockId/shape.
// Shape is the frontend-managed native definition (UI geometry/flowchartShape.ts), not code.
type GraphNativeSettings struct {
	Width     float64         `json:"width"`
	Height    float64         `json:"height"`
	BlockID   string          `json:"blockId"`
	Shape     json.RawMessage `json:"shape"`
	TextFrame GraphFrame      `json:"textFrame"`
}

// GraphPlacementData follows rpc_graph.layerActionData, excluding all delete fields.
type GraphPlacementData struct {
	ID            string               `json:"id"`
	Type          string               `json:"type"`
	LaIDSource    *int64               `json:"laIdSource,omitempty"`
	LaIDTarget    *int64               `json:"laIdTarget,omitempty"`
	Position      *GraphPoint          `json:"position,omitempty"`
	AreaPicture   *GraphAreaPicture    `json:"areaPicture,omitempty"`
	LayerSettings *GraphNativeSettings `json:"layerSettings,omitempty"`
}

// GraphPlacementItem restricts layerActionItem.Action to create through its schema.
type GraphPlacementItem struct {
	Action string             `json:"action"`
	Data   GraphPlacementData `json:"data"`
}
type GraphPlacementsInput struct {
	Operation
	ActorID string               `json:"actorId"`
	Items   []GraphPlacementItem `json:"items"`
}
type GraphPlacementOperationInput struct {
	Operation
	ActorID string `json:"actorId"`
}

// GraphCreatedNode and GraphCreatedPlacementEdge mirror layerManageRes projections exactly.
type GraphCreatedNode struct {
	LaID     int64       `json:"laId"`
	ActorID  string      `json:"actorId"`
	Position *GraphPoint `json:"position,omitempty"`
}
type GraphCreatedPlacementEdge struct {
	LaID       int64  `json:"laId"`
	EdgeID     string `json:"edgeId"`
	LaIDSource *int64 `json:"laIdSource,omitempty"`
	LaIDTarget *int64 `json:"laIdTarget,omitempty"`
}
type GraphCreatedPlacements struct {
	Nodes []GraphCreatedNode          `json:"nodes"`
	Edges []GraphCreatedPlacementEdge `json:"edges"`
}
