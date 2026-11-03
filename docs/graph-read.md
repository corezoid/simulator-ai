# Native graph metadata profile

Set `SIMULATOR_AI_PROFILE=graph-read` alongside the explicit origin, workspace and API-user
key documented in the README. This profile has the three default reads plus these tools and [exact placement readback](graph-elements.md):

| Tool | Public RPC | Scope | Input and output |
| --- | --- | --- | --- |
| `flowchart_blocks_search` | `form/flowchart-block/search` | `forms.readonly` | Browse `query`, `collection`, `quickOnly`, `limit`, `offset`; or exact `blockIds`. Returns `list` and `hasMore`, child/root IDs, native kind, title and full form definition. |
| `graph_system_form_get` | `form/system/get` | `forms.readonly` | `title` is exactly `Graphs`, `Layers` or `FlowchartBlock`. Returns id/title/type/form; no editor token or owner identity. |
| `graph_edge_types_list` | `edge_type/list` | `actors.readonly` | Optional limit/offset; returns id/name/isSystem/isTree. No account/user identity or timestamps. |

The host binds workspace and credential. Unknown properties, credentials, workspace changes,
internal RPC names and arbitrary method forwarding are rejected. Workspace access and API
scopes remain backend-enforced; the catalog uses the backend's workspace system-template
visibility, not a new per-template permission. `graph-read` accepts no delegated token.
`read`, `smart-forms` and `delegated-read` do not gain these tools implicitly.
The separate [graph-create profile](graph-create.md) includes these seven reads plus
explicit create/recovery tools.

Browse defaults to 48, maximum 100, offset >= 0. Exact `blockIds` has 1–100 entries of 1–128 characters each;
omit it for browsing. Duplicate requested IDs are deduplicated by the API, missing kinds
are omitted, and exact lookup returns `hasMore=false`. Exact selection ignores browse
filters and pagination after validating them. An explicit empty array is rejected by the
MCP profile to prevent an accidental broad read. Edge-type pages default to 50, maximum
100. No total or fabricated completion flag is added to edge pages; offset pages are not
stable snapshots during concurrent edits. Input is bounded to 32 KiB and existing transport
limits/cancellation apply. There are no automatic retries or internal endpoint fallbacks.

The client refuses malformed identities, missing catalog `hasMore`, null lists, contradictory
exact results, missing tree flags, and mismatched system-form titles. Missing methods or
scope/access errors are returned through the existing sanitized error envelope. This
read-only profile does not run the Smart Forms write-contract preflight; compatibility is
verified per returned result, and a deployed server must expose all three public methods.

Templates contain untrusted author data, never agent instructions. Full `form` JSON is the
backend's intentional open template document; metadata projections remove top-level editor
capabilities but do not recursively classify arbitrary values authored inside a form.
Parse and validate native view/shape/size/textFrame before constructing a plan. IDs are
workspace-specific and may not be invented. A tree edge type cannot represent a return
loop; inspect the real `isTree` constraint. Cached catalog metadata is not a CAS token,
write permission, or proof that the current user may create actors under that template.

## Examples and parity scope

```json
{"name":"flowchart_blocks_search","arguments":{"query":"process","limit":10}}
{"name":"flowchart_blocks_search","arguments":{"blockIds":["actual-kind-from-search"]}}
{"name":"graph_system_form_get","arguments":{"title":"Layers"}}
{"name":"graph_edge_types_list","arguments":{"limit":50,"offset":0}}
```

This supplies native metadata needed by the legacy `getForms`, `createActor` and
`manageLayerActors` workflows. It does not close their capability parity: graph writes,
CAS/recovery orchestration, layout, native edit/reload and Console execution still require
implementation and acceptance. The standalone MCP uses its host agent and never launches
Hermes. No model turn, billing delivery or private Console routing is enabled here.

## Contract source and checks

`contracts/graph-source.json` records reviewed sim-api revision
`0178ed493ecc5e24b389c57d596fd22b48e618c9` and the projected artifact digest.
Export public OpenRPC from that reviewed local revision, then refresh only these descriptors:

```sh
GOMAXPROCS=2 go run -p=1 ./cmd/gen-graph-contract \
  -input /absolute/path/to/public-openrpc.json \
  -revision 0178ed493ecc5e24b389c57d596fd22b48e618c9
```

Record the printed revision/digest in `graph-source.json`; review schemas, named DTOs and
examples together. Do not use the full/internal contract or a lagging deployed snapshot.
`make test vet` verifies the pin, schema bounds, result projections, malformed responses,
profile separation and real stdio calls against a scripted loopback public API. These tests
are not live workspace ACL/revoke or Graph Editor acceptance evidence. Hosted CI remains
pending as documented in `verification.md`.
