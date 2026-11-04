# Simulator AI

Public MCP tools and portable skills for the new Simulator JSON-RPC API. This repository
is separate from [simulator-ai-plugin](https://github.com/corezoid/simulator-ai-plugin),
which continues to serve legacy. It does not change legacy configuration or routing.

**Development profiles:** desktop stdio, API-user key, three metadata reads by default.
Opt in to thirteen recoverable Smart Forms tools with `SIMULATOR_AI_PROFILE=smart-forms`;
see [Smart Forms setup and guarantees](docs/smart-forms.md). It is not a
full legacy replacement or an accepted production release. Claude Code/Codex use their own
agent; this binary launches no Hermes, model, LiteLLM or server AI billing.

Native graph metadata is available through the eight-tool `graph-read` profile; see
[graph metadata and constraints](docs/graph-read.md). This prepares native flowchart
planning and [exact node/edge readback](docs/graph-elements.md). The [native planner and portable graph skill](docs/graph-planning.md) calculate creation payloads and geometry. The separate [graph-create profile](docs/graph-create.md) adds six public
create/recovery tools with mandatory receipt keys and per-write contract preflight.
A native generator/layout and Console execution are still pending.

Reaction counts are available through the four-tool `reactions-read` profile and
the portable `simulator-ai-reactions` skill; see [reaction statistics](docs/reactions-read.md).

The separate [Console delegated-read profile](docs/delegated-read.md) accepts only
manager-delivered scoped credentials and exposes `actor_get`/`form_get`. It does not
change desktop configuration, provision a runtime or enable Console routing.

| MCP tool | Public RPC | Returned subset |
| --- | --- | --- |
| `actor_get` | `actor/get` | id, title, description, formId, status, ref |
| `form_get` | `form/get` | id, title, description, type, parentId; no field schema/editor token |
| `layer_actors_list` | `graph/layer/actors/list` | One node-placement page, optional authorized actor metadata/position; no edges or total |

API scopes are `actors.readonly` / `forms.readonly`, plus existing workspace and object ACLs.
The server binds one explicit origin/workspace/key per process. Tools cannot supply another
workspace, arbitrary RPC, URL, credentials or `with` expansion. Read requests are never
coalesced with counts. No automatic retries, paid tests or remote MCP. Writes require the explicit `smart-forms`, `graph-create` or `console` profile.

## Local setup

Requires Go 1.27.2+ for source builds. From the checkout:

```sh
make test vet build
# Install the reviewed source's executable into your normal Go binary directory:
go install ./cmd/simulator-ai
```

Put that binary directory on the host client's PATH, or configure an absolute binary path.
Set these variables in the **client's process environment**, through its secret-management
mechanism. No default production endpoint or workspace is assumed:

- `SIMULATOR_AI_ORIGIN`: explicit `https://your-simulator-origin.example` (origin only).
- `SIMULATOR_AI_WORKSPACE`: workspace ID for the API user.
- `SIMULATOR_AI_PROFILE`: `read` (default), `graph-read` (native graph metadata), `graph-create` (create-only graph operations), or
  `smart-forms` (Smart Form writes), `reactions-read` (reaction statistics), `console` (AI Console, see
  [below](#ai-console-profile)). Console `delegated-read` has separate setup.
- `SIMULATOR_AI_API_KEY`: API-user key; never paste it into chat, a tool call or committed config.
- `SIMULATOR_AI_AUTHORIZATION`: instead of the API key, a complete `Authorization` header value —
  `Bearer <token>`, `Simulator <jwt>` or `ApiKey <key>` — for a host that starts this process per request under
  the requesting user's own credential (the Hermes relay in sim-api `docs/hermes-local.md`). Not with
  `SIMULATOR_AI_API_KEY`, `SIMULATOR_AI_DELEGATED_TOKEN` or `delegated-read`.
- `SIMULATOR_AI_ACTOR`: optional host-bound target for `reactions-read` or `graph-read`; exposes
  only the respective reaction/layer statistics tool with empty arguments. Leave unset for workspace mode.

Only literal loopback HTTP is accepted for local scripted development. HTTPS verifies TLS;
redirects are refused, including same-origin redirects, so credentials cannot be forwarded.
No login/OAuth is claimed. Normal human Bearer sessions are not accepted by this profile.

The `plugins/simulator-ai` directory includes Claude Code and Codex manifests, MCP config
and the portable `simulator-ai-read` / `simulator-ai-smart-forms` skills. Its MCP command is the separately installed
`simulator-ai` binary. Install that plugin directory using your client's plugin mechanism,
or configure the MCP directly (the latter does not install the skill automatically):

```json
{"mcpServers":{"simulator-ai":{"command":"simulator-ai","args":[]}}}
```

Codex MCP configuration equivalent:

```toml
[mcp_servers.simulator-ai]
command = "simulator-ai"
args = []
env_vars = ["SIMULATOR_AI_ORIGIN", "SIMULATOR_AI_WORKSPACE", "SIMULATOR_AI_API_KEY", "SIMULATOR_AI_PROFILE", "SIMULATOR_AI_ACTOR"]
```

Or register it from the command line. Claude Code starts stdio servers with its own environment, so
export the `SIMULATOR_AI_*` variables in the shell that runs `claude`:

```sh
claude mcp add --scope user simulator-ai -- simulator-ai
```

Codex forwards only the variables named in `env_vars`: after `codex mcp add simulator-ai -- simulator-ai`
add the `env_vars` line above to `[mcp_servers.simulator-ai]` in `~/.codex/config.toml`
(include `SIMULATOR_AI_AUTHORIZATION` when you use it instead of the API key).

Do not store a key in either example. Use the client's environment injection to supply it.
Run `make client-smoke` to install into disposable Codex/Claude Code profiles and check real
client loading against a scripted loopback API, without a model turn. See
[client acceptance](docs/client-acceptance.md) for evidence and remaining live ACL/revoke gates.

## Tools by profile

Actor, form and graph tools mirror the legacy `simulator-ai-plugin` tools (name in brackets) on one public
method each, or on a few for the composite ones. Their input schemas are the pinned public params
(`contracts/parity-openrpc.json`, source in `parity-source.json`), and the result is the backend's own
projection, shaped by `fields`/`with` where the method has them. There are no receipt keys or contract
preflight. `read` and `graph-read` get only reads; `graph-create`, `smart-forms`, `reactions-read` and
`delegated-read` keep their own catalogs.

| Tool [legacy] | Public method | console | read | graph-read |
| --- | --- | :-: | :-: | :-: |
| `actor_get` [getActor] | `actor/get` (fields, with) | ✓ | ✓ | ✓ |
| `actor_by_ref_get` [getActorByRef] · `actor_system_get` [getSystemActor] | `actor/by-ref/get` · `actor/by-system_obj/get` | ✓ | ✓ | ✓ |
| `actors_search` [searchActors] · `actors_filter` / `actors_filter_count` [filterActors] | `actor/search/list` · `actor/filter/list` / `count` | ✓ | ✓ | ✓ |
| `actor_create` / `actors_create` [createActor] · `actor_update` [updateActor] | `actor/create` (batch) · `actor/update` | ✓ | | |
| `actor_status_update` [setActorStatus] · `actor_delete` [deleteActor] | `actor/status/update` · `actor/delete` | ✓ | | |
| `form_get` [getForm] (body, editToken with `editor`) | `form/get` | ✓ | ✓ | ✓ |
| `forms_list` / `forms_count` [getForms, searchForms] | `form/list` / `form/count` | ✓ | ✓ | ✓ |
| `form_create` · `form_update` · `form_status_update` · `form_delete` [createForm, updateForm, setFormStatus, deleteForm] | `form/create` · `form/update` · `form/status/update` · `form/delete` | ✓ | | |
| `form_accounts_list` [getFormAccounts] | `account_form/list` | ✓ | ✓ | ✓ |
| `form_account_create` · `form_account_delete` [createFormAccount, removeFormAccount] | `account_form/create` · `account_form/delete` | ✓ | | |
| `edge_create` / `edges_create` [createLink, massLink] | `edge/create` (batch) | ✓ | | |
| `edge_get` [getEdge] | `edge/get` | ✓ | | ✓ |
| `edge_update` · `edge_endpoints_update` · `edge_delete` [updateEdge, deleteEdge] | `edge/update` · `edge/endpoints/update` · `edge/delete` | ✓ | | |
| `edges_by_nodes_delete` [deleteEdgesByNodes] | `batch` of `edge/by-node/delete` | ✓ | | |
| `edge_types_list` [getEdgeTypes] | `edge_type/list` | ✓ | | `graph_edge_types_list` |
| `layer_placements_list` · `layer_edges_list` [getLayerActorsPaginated] | `graph/layer/actors/list` · `graph/layer/edges/list` | ✓ | | ✓ |
| `layer_placements_all` [getAllLayerPlacements] | `graph/layer/actors/list`, every page | ✓ | | ✓ |
| `layer_skeleton_get` [getLayerActors] | `graph/layer/skeleton/get` | ✓ | | ✓ |
| `layer_placements_manage` [manageLayerActors] (`layer_placements_create` for flowchart blocks) | `graph/layer/actors/manage` | ✓ | | |
| `layer_positions_update` [updateLayerPositions] · `layer_actors_move` [moveActors] · `layer_clean` [cleanGraphLayer] | `…/positions/update` · `…/actors/move` · `graph/layer/clean` | ✓ | | |
| `layer_element_exist` [existLayerElement] · `layer_actors_search` [searchLayerActors] | `graph/layer/element/exist` · `graph/layer/actors/search/list` | ✓ | | ✓ |
| `layer_stats_get` [layerStats] | `graph/layer/stats/get` | ✓ | | `graph_layer_stats_get` |
| `graph_list` [getRelatedActors, getLinkedActors] (hierarchy by default) | `graph/list` | ✓ | | ✓ |
| `graph_get` [getActorLinks] | `graph/get` | ✓ | | ✓ |
| `layer_compact` [compactGraphLayout] · `layer_prune_long_edges` [pruneLongEdges] | layer reads + `…/positions/update` · `edge/delete` | ✓ | | |

Not covered yet, and why:
- No public method: getCorezoidProcesses, searchAll, existLink (all ❌); getFormsTree (`form/tree/get`) and
  getLinkedForms (`graph/forms/list`) are internal only (🔒). The gap list has been sent to the backend.
- pullGraphFile / pushGraphFile: a local file ↔ layer sync engine on public methods, too large for this slice.

## AI Console profile

`SIMULATOR_AI_PROFILE=console` is what Hermes runs per AI Console request under the initiator's own
`SIMULATOR_AI_AUTHORIZATION`. It covers two Console answers: a flowchart drawn on a layer and a released
Smart Form opened inline in the reply. Writes are plain public calls: no receipt keys and no contract
preflight, because sim-api deduplicates Console retries. Each tool returns only ids and what the next
step needs.

| MCP tool | Public RPC | Result |
| --- | --- | --- |
| `actor_get` | `actor/get` | id, title, description, formId, status, ref |
| `actor_create` / `actor_update` | `actor/create` / `actor/update` | same actor subset |
| `actors_create` / `edges_create` | one atomic `batch` of `actor/create` / `edge/create` (edge types by name) | actors / edges in input order; one failed item fails the call with its index and creates nothing |
| `flowchart_blocks_list` | `form/flowchart-block/search` | formId, title, blockId, `hasMore`; with `blockIds` also ready `data` and `layerSettings` |
| `flowchart_layout` | `form/flowchart-block/search` (read-only) | per node formId, data, grid position and layerSettings; `returnEdges` |
| `layer_create` | `form/system/get` (`Layers`) + `actor/create` | layer actor |
| `edge_create` | `edge_type/get` (default `hierarchy`) + `edge/create` | id, source, target, edgeTypeId, name |
| `layer_placements_create` | `graph/layer/actors/manage` (create only); a flowchart node is border-snapped and gets the block's `areaPicture` | nodes `{laId, actorId, position}`, edges `{laId, edgeId, laIdSource, laIdTarget}` |
| `layer_positions_update` | `graph/layer/actors/positions/update` | updated count |
| `layer_get` | `graph/layer/actors/list` + `graph/layer/edges/list` | compact nodes/edges, `hasMore` |
| `smart_form_create` | `application/create` (`cduVersion: 2`) | appId, ref, developEnvId, productionEnvId |
| `smart_form_components` | none (pinned sim-api CDU 2.0 catalog, `contracts/cdu-components.json`) | component types with category; with `types` their props, events and slots |
| `smart_form_files_get` | `application/env/list` + `application/struct` (develop) | folders/files by path, optional sources |
| `smart_form_files_update` / `smart_form_files_create` | `app_file/update` / `app_file/create` | id, folderId, title, sourceHash |
| `smart_form_release` | `application/env/list` + `release/create` (develop → production) | `{appId, releaseId, releaseNumber, status, ready, marker}` |
| `smart_form_release_get` | `application/env/list` + `release/get` | same release result |

Flowchart: `flowchart_blocks_list` (kinds) → `flowchart_layout(nodes, edges)` → `actor_create(formId, title,
color, data)` per node → `layer_placements_create` nodes with the layout's `position`, `layerSettings` and the same
`color` → one `edges_create` (`name` is the label) → `layer_placements_create` edges between the node `laId`s →
`layer_get` to read it back. Geometry follows Graph Editor: a cell is 50 units and grid lines lie on cell
boundaries (25 + 50k); a position is the block centre and a block's top-left corner sits on a grid line, so a
block of whole cells fills whole cells. The layout ranks nodes top to bottom (loop-back edges excluded from
ranking) with three free cells between blocks and ranks. Like the editor, the placement carries the block
silhouette as `areaPicture`, which makes it graphic, so sim-api keeps that position instead of snapping the centre
to a multiple of 50 (half a cell off for even-sized blocks). Smart Form: `smart_form_create` →
`smart_form_files_get` → `smart_form_files_update` of `pages/index/config` → `smart_form_release`. Its
`ready: true` (status `active`, snapshot kept, production env) comes with `marker: "appId: <appId>"`. The
agent ends its final answer with that line, and sim-api reads it from the Markdown (the Hermes run stream carries
no tool results) to put `appId` into the reply reaction with `appSettings.autorun`.

### Live smoke on a local stand

`make smoke SIM_API_DIR=<sim-api checkout> WORKSPACE=<workspace id>` builds the binary, then runs sim-api's
`make ai-smoke-env` (sim-api `cmd/ai-dev-credential/README.md`), which issues a short `Simulator` JWT of the stand's seed
user by default (`SMOKE_KIND=session` for a Bearer session, `SMOKE_KIND=apikey` for a workspace API user without
workspace roles, `SMOKE_TTL`, `ROTATE=1`) and writes `~/.config/simulator-ai/smoke.env` (`SIMULATOR_AI_ORIGIN`, `SIMULATOR_AI_WORKSPACE`,
`SIMULATOR_AI_API_KEY` or `SIMULATOR_AI_AUTHORIZATION`, mode 0600). It then runs `cmd/console-smoke` over stdio with only that file's credential (five-minute bound): a layer with Start / Process / End blocks, two labelled edges and their
placements read back with `layer_get`, then a Smart Form with a field and a submit button, released and
read back as `ready`. Each run creates new objects in that workspace; nothing secret is printed.

The same file connects the desktop agents with the read profile. Start them from a shell that loaded it:

```sh
set -a; . ~/.config/simulator-ai/smoke.env; set +a; export SIMULATOR_AI_PROFILE=read
claude mcp add --scope user simulator-ai -- simulator-ai   # once; then run `claude`
codex mcp add simulator-ai -- simulator-ai                 # once; add env_vars (above) incl. SIMULATOR_AI_AUTHORIZATION, then run `codex`
```

`claude mcp list` shows `simulator-ai ... ✔ Connected`; ask either agent for `actor_get` of a known actor.

## Development and contracts

[contracts/source.json](contracts/source.json) pins the source commit and complete public
OpenRPC digest. [public-openrpc.json](contracts/public-openrpc.json) is a projection containing
only the three public methods, copied from a locally generated public document. The Go
client's request types and returned metadata types are generated from that pinned source.
`make generate` updates code; `make gen-check` detects drift. Never import private/full RPC
snapshots or widen the catalog as a side effect of regeneration.

To refresh after a reviewed backend contract change, export `cmd/api-contract -public` from
the intended sim-api revision, select these same three methods, record its SHA/revision,
regenerate, inspect schemas and run tests. `make gen-check` verifies generated code locally;
hosted CI activation is pending (see [verification](docs/verification.md)). Local generation
checks cannot prove the deployed server matches the pinned source. [Architecture](docs/architecture.md)
records transport/errors/limits and remaining gates.
