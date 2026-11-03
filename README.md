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
| `flowchart_blocks_list` | `form/flowchart-block/search` | formId, title, blockId, `hasMore`; with `blockIds` also ready `data` and `layerSettings` |
| `layer_create` | `form/system/get` (`Layers`) + `actor/create` | layer actor |
| `edge_create` | `edge_type/get` (default `hierarchy`) + `edge/create` | id, source, target, edgeTypeId, name |
| `layer_placements_create` | `graph/layer/actors/manage` (create only) | nodes `{laId, actorId, position}`, edges `{laId, edgeId, laIdSource, laIdTarget}` |
| `layer_positions_update` | `graph/layer/actors/positions/update` | updated count |
| `layer_get` | `graph/layer/actors/list` + `graph/layer/edges/list` | compact nodes/edges, `hasMore` |
| `smart_form_create` | `application/create` (`cduVersion: 2`) | appId, ref, developEnvId, productionEnvId |
| `smart_form_files_get` | `application/env/list` + `application/struct` (develop) | folders/files by path, optional sources |
| `smart_form_files_update` / `smart_form_files_create` | `app_file/update` / `app_file/create` | id, folderId, title, sourceHash |
| `smart_form_release` | `application/env/list` + `release/create` (develop → production) | `{appId, releaseId, releaseNumber, status, ready, marker}` |
| `smart_form_release_get` | `release/get` | same release result |

Flowchart: `flowchart_blocks_list` → `actor_create(formId, title, data)` per block → `layer_placements_create`
nodes with the block's `layerSettings` → `edge_create` (`name` is the label) → `layer_placements_create`
edges between the node `laId`s → `layer_get` to read it back. Smart Form: `smart_form_create` →
`smart_form_files_get` → `smart_form_files_update` of `pages/index/config` → `smart_form_release`. Its
`ready: true` (status `active`, snapshot kept, production env) comes with `marker: "smart-form: <appId>"`. The
agent ends its final answer with that line, and sim-api reads it from the Markdown (the Hermes run stream carries
no tool results) to put `appId` into the reply reaction with `appSettings.autorun`.

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
