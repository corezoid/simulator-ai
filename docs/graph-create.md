# Recoverable graph creation profile

`SIMULATOR_AI_PROFILE=graph-create` is an explicit desktop write opt-in. It includes
all seven `graph-read` tools and six creation/recovery tools below plus the read-only [native planner](graph-planning.md) (14 total). Use the
same explicit origin/workspace/API-user key. No delegated credential, server Hermes,
Console dispatch or server billing is added. Other profiles do not gain writes.

| Tool | Public RPC | Effect / authorization |
| --- | --- | --- |
| `graph_actor_create` | `actor/create` | Create under an actual template; `actors.management`, current template access and context-layer Modify if supplied. No visibility/ACL overrides. |
| `graph_actor_operation_get` | `actor/operation/get` | Original actor snapshot, current Modify on the receipt actor. |
| `graph_edge_create` | `edge/create` | Create or reuse a natural-key relationship; `actors.management`, current View on endpoints and linked actor. |
| `graph_edge_operation_get` | `edge/operation/get` | Original resolved edge snapshot; current View on original/current endpoints and linked actor. |
| `graph_placements_create` | `graph/layer/actors/manage` | Create-only atomic placement packet; `actors.management`, current layer Modify and reference View. |
| `graph_placements_operation_get` | `graph/layer/operation/get` | Original placements for the exact layer; current primary layer/reference authorization. |

Every operation requires an explicit key matching `[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}`.
Keys are scoped by backend installation, principal, workspace and method. Save the exact
payload/key before sending. No tool generates a key implicitly, retries automatically,
rekeys a failure, or deletes partial work. Current authorization is checked by sim-api on
both replay and lookup. Missing lookup can mean the first write is still in flight; an
expired retained receipt never permits repeating the effect under a new key.

Before **each write**, MCP reads the credential-free `/api/2.0/doc/public` and checks the
six pinned public method parameter contracts/scopes/audiences. A lagging or incompatible
server is refused before POST, so it cannot silently ignore operationId. Operators must
ensure every replica behind the origin has the reviewed contract; one schema response
cannot certify a heterogeneous deployment. Reads/recovery do not need this preflight.
Transport/HTTP/malformed-success errors after POST retain an explicit uncertain-outcome
message. Recover the receipt before deciding whether to retry the identical payload.

## Creation inputs and results

Actor input: required formId > 0, title (1–1000 characters), object data and operationId;
optional description, hex color and contextLayerId. `data` is the public API's open
form-value document, including native `__form__<child-form-id>:<item-id>` fields. It
is not a credential or policy channel. System/private/public flags, arbitrary expansions,
custom workspace and credentials are not exposed. Backend default access behavior still
applies: creation does not make an actor private. Result is the normal metadata projection
(id/title/description/formId/status/ref), never full data or owner credentials.

Edge input: required source, target, positive edgeTypeId and operationId; optional name,
linkedActorId, curveStyle and forceDirection. Resolve actual edge types from graph-read.
An existing natural-key edge is returned unchanged; the requested name/style is not proof
it changed. A reverse hierarchy edge can be returned when forceDirection is false. Always
use the returned source/target and ID. This profile does not expose edge updates or deletes.

Placement input: actorId is the layer, 1–100 items, operationId. Each action is exactly
`create`. A node needs an existing actor ID and explicit `{x,y}` position; an edge needs
an existing edge ID and both positive laIdSource/laIdTarget. First create/read node
placements, then use their returned IDs in a separate edge-placement packet. No `laId`
delete field, mixed delete packet or arbitrary settings are accepted.

Optional node `areaPicture` follows the backend width/height/img object. Optional
`layerSettings` is a native-block subset: width/height >= 1, blockId (1–128 characters),
object shape and `{x,y,w,h}` textFrame (nonnegative width/height). The shape is the actual
frontend-managed definition, copied from a validated catalog; this generic operation
does not validate its drawing semantics. Do not substitute `{}` or invent a shape. No
other layer settings are forwarded. Nongraphic nodes may snap to the backend grid; read
returned positions instead of assuming the requested coordinates were stored unchanged.

Results reject missing/duplicate placement IDs within each node/edge namespace (their
PostgreSQL sequences are independent), incomplete coordinates, missing edge
anchors, wrong form identity or foreign node/edge bindings. Repeated actors are allowed
and checked as a multiset. Receipt coordinates and edge metadata are historical; later
human moves/rewires are not overwritten on replay. Missing/deleted/replaced objects fail
rather than being recreated. Calls are individually atomic where documented by sim-api;
a multi-call graph workflow is not one global transaction and must report partial progress.
Input is bounded to 512 KiB; transport responses remain limited to 4 MiB.

## Example sequence

Use actual template IDs from `graph_system_form_get` and real field definitions. Creating
a graph or layer is an actor creation, not a special internal route. A native block uses
the FlowchartBlock root formId plus the child definition's prefixed fields, with the view
JSON parsed as in the UI writer. Graph/layer relationships use the actual edge type.

```json
{"name":"graph_actor_create","arguments":{"operationId":"demo:layer:1","formId":123,"title":"Approval flow","data":{}}}
{"name":"graph_actor_operation_get","arguments":{"operationId":"demo:layer:1"}}
{"name":"graph_placements_create","arguments":{"operationId":"demo:nodes:1","actorId":"actual-layer-id","items":[{"action":"create","data":{"id":"actual-block-actor-id","type":"node","position":{"x":0,"y":0}}}]}}
```

`123` and IDs above are placeholders, not seeded IDs. Inspect the actual template fields
and fill them for the intended workspace. A plain node placement example does not establish
native flowchart parity. The deterministic planner now supplies native creation payloads and node layout. Updates with conflict
preconditions, same-tab refresh and live Graph Editor/Console acceptance remain open.

## Source and checks

Backend source `0178ed493ecc5e24b389c57d596fd22b48e618c9`; native placement field sources
UI `2e73d8c1d114d3334d3648d0a9ed93a47364e2eb`, graph-layer `flowchartCommands.ts`,
`layerSettings.ts` and `geometry/flowchartShape.ts`. `contracts/graph-create-source.json`
pins the generated public projection. Refresh with `cmd/gen-graph-contract -profile create`
and explicit `-input`, `-revision`, `-output contracts/graph-create-openrpc.json`; record
the digest and review named DTOs/schema narrowing together.

Targeted tests use scripted HTTP and real stdio. They exercise schema/profile fences,
receipt lookup after an uncertain write, stale backend rejection, native field bounds,
repeated actor placements and result binding. Actual database atomicity/ACL evidence
belongs to backend actor/edge/placement receipt functional tests (MR403/404/405). Neither
those tests nor this profile's mocks are final Console/native-flowchart acceptance.
