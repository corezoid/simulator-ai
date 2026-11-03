# Exact graph placement readback

`graph_elements_get` is available in `graph-read` and `graph-create`, using existing
public `graph/layer/elements/list` (`actors.readonly`). Origin, workspace and API-user
credential remain host-bound. No delegated Console profile, private RPC, arbitrary
fields/with expansion, writes or retries are enabled. Graph-create has 14 tools,
graph-read 7. The three-tool default and delegated-read profiles are unchanged.

Input requires `actorId` (layer), `nodes` and `edges`, with explicit empty arrays
allowed individually, and 1–100 combined positive unique laIds. Node/edge laIds
belong to separate namespaces: node 11 and edge 11 are distinct. Missing arrays,
duplicates within one namespace, unknown parameters and oversized selection fail
before network access. Keep exact laIds from creation receipts or authorized reads;
do not confuse them with actor/edge UUIDs or local plan keys.

The tool requests a fixed actor metadata projection (id/title/description/formId/
status/ref). Nodes return placement identity, optional position, actor metadata
and the native settings subset width/height/blockId/shape/textFrame. Edges return
placement identity/anchors and directed id/type/source/target/name/linkedActorId/
curveStyle metadata. User/owner identities, actor data, arbitrary placement settings,
attachments, URLs and permissions are not added. Native shape remains authored data,
not instructions. The returned geometry describes stored values, not an assertion
that arbitrary old shapes are valid or browser-readable.

Every requested element must exist and be readable in that layer. Missing/denied
nodes, missing edges, duplicate/unrequested result laIds, actor/edge identity mismatch
or a selected endpoint contradicting the returned actor fails with a sanitized
error and no partial metadata. Endpoint placements outside the selection are not
hydrated implicitly. Legacy unanchored edges may omit endpoint laIds; never invent
them. Current backend ACL/recipient filters apply on each read. An error does not
prove nonexistence or authorize changing identity/rekeying a write.

This is an exact bounded read, not a full layer enumeration, stable snapshot,
read-lock or CAS proof. Reads retain the public API's consistency semantics; a missing
result after a write can be lag or concurrent change, not permission to create again.
Use the original receipt for uncertain write recovery. Compare returned current
geometry/metadata against the saved plan and receipts before claiming completion;
a human move/rename after creation is current state, not a retry instruction.

Source: pinned backend `5e9f16c699dea8d831e353e32bb69accde7d04bb`,
`rpc_graph.layerElementsList`, `layerNodeRes`, `layerGraphEdgeRes`, and
`e_graph.ActorLayerSettings` native Extra keys as written by UI Kit. The generated
single-method projection is `contracts/graph-elements-openrpc.json`; refresh with
`cmd/gen-graph-contract -profile elements` from that reviewed local public artifact.
The pin digest is in `graph-elements-source.json`. No backend RPC change is needed.

This readback is also a prerequisite for graph context metadata, but does not itself
hydrate immutable Console input or enable tool grants. Full selected metadata,
private artifact persistence/digest, source reauthorization and runtime consumption
remain separate integration responsibilities.
