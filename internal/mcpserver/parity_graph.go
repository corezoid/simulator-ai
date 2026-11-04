package mcpserver

// graphTools are the legacy graph tools on single public methods (getEdge, updateEdge, deleteEdge,
// getEdgeTypes, getLayerActorsPaginated, getLayerActors, manageLayerActors, moveActors,
// cleanGraphLayer, existLayerElement, layerStats, searchLayerActors, getRelatedActors,
// getLinkedActors, getActorLinks). createLink, massLink, deleteEdgesByNodes, updateLayerPositions,
// getAllLayerPlacements, compactGraphLayout and pruneLongEdges have their own tools.
var graphTools = []parityTool{
	{Name: "edge_get", Method: "edge/get", Description: "Read one edge: type, source, target, label, linked actor, curve style, pinned."},
	{Name: "edge_exist", Method: "edge/by-node/get", Description: "Check whether an edge of a type joins two actors (legacy existLink): list holds the source -> target edge first, then with bidirected the target -> source one; an empty list means there is none."},
	{Name: "edge_update", Method: "edge/update", Description: "Change an edge's label (name), linked actor, curve style or pinned flag; only sent keys change."},
	{Name: "edge_endpoints_update", Method: "edge/endpoints/update", Description: "Re-point an edge to another source and/or target actor, keeping its id and placements."},
	{Name: "edge_delete", Method: "edge/delete", Description: "Delete an edge and its placements on every layer. Destructive: confirm with the user first."},
	{Name: "edge_types_list", Method: "edge_type/list", Description: "List the workspace's edge types (id, name, isSystem, isTree) one page at a time; hierarchy is the usual one."},
	{Name: "layer_placements_list", Method: "graph/layer/actors/list", Description: "Read one page of a layer's node placements, most recent first: laId, position, layerSettings and the actor (fields/with shape it). limit up to 1000; a denied actor is a stub with accessDenied. Edges are layer_edges_list; every node at once is layer_placements_all."},
	{Name: "layer_edges_list", Method: "graph/layer/edges/list", Description: "Read one page of a layer's edge placements: laId, the node placements it joins (laIdSource, laIdTarget), its layer look (layerSettings) and the edge. limit up to 1000."},
	{Name: "layer_skeleton_get", Method: "graph/layer/skeleton/get", Description: "Read a large layer's layout fast: compact node and edge placements, optionally only inside a box (x1,y1,x2,y2 in graph units)."},
	{Name: "layer_placements_manage", Method: "graph/layer/actors/manage", Description: "Add or remove placements on a layer in one transaction (1-500 items, deletes first): create node {id, position}, create edge {id, laIdSource, laIdTarget}, delete node/edge {id, laId?}. For flowchart blocks use layer_placements_create, which keeps them on the grid."},
	{Name: "layer_actors_move", Method: "graph/layer/actors/move", Description: "Move placements from one layer to another, with their edges between moved nodes; 1-10 items name actors (and laId) with an optional new position."},
	{Name: "layer_clean", Method: "graph/layer/clean", Description: "Remove every placement from a layer; actors and edges themselves stay. Destructive: confirm with the user first."},
	{Name: "layer_element_exist", Method: "graph/layer/element/exist", Description: "Check whether an actor (type node) or edge (type edge) is already placed on a layer, before placing it again."},
	{Name: "layer_stats_get", Method: "graph/layer/stats/get", Description: "Count a layer's nodes, edges and holes."},
	{Name: "layer_actors_search", Method: "graph/layer/actors/search/list", Description: "Search the actors placed on a layer by title (query); one page per call, fields/with shape each actor."},
	{Name: "graph_list", Method: "graph/list", Hierarchy: true, Description: "Walk one relation of an actor page by page: type children, parents or linked (any direction), along edgeTypeId (default hierarchy), filtered by the neighbours' formId/exceptFormId, title search and dates, sorted with orderBy/orderValue. This is getRelatedActors; for neighbours over every edge type (getLinkedActors) use graph_get."},
	{Name: "graph_get", Method: "graph/get", Description: "Read the graph one step around an actor: the edges touching it (either direction) and the actors at their ends, oldest first; edgeTypeIds and pinned filter edges; sysLinkTypes adds synthetic relations (workspace, creator, owner, layers, layerActors). This is getActorLinks and, over all edge types, getLinkedActors."},
}
