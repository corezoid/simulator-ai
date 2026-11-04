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
workspace, arbitrary RPC, URL or credentials; `fields`/`with` exist only on the parity tools whose public
method documents them. Read requests are never
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

Skills ported from the legacy plugin onto this catalog: `simulator-ai` (platform model, rules and
which skill does what), `simulator-ai-init` (environment and profile setup), `simulator-ai-actors`,
`simulator-ai-forms`, and for Smart Forms `simulator-ai-smart-forms-logic` (Corezoid `/get`/`/send`
briefs for the Corezoid plugin), `simulator-ai-smart-forms-runtime`, `simulator-ai-styles` (the v2
`theme` file) and `simulator-ai-app-generator`. Where the catalog has no tool (binding a Corezoid
process, rendering or submitting a page), they say so and leave that step to the user.
`simulator-ai-graph` and `simulator-ai-smart-forms` lead with the console tools (flowcharts,
batch actors and edges, holes, layer edits; Smart Form pages, data templates, releases, the Console
reply) and keep the `graph-read`/`graph-create` and `smart-forms` profiles as their own sections.

The `plugins/simulator-ai` directory includes Claude Code and Codex manifests, MCP config
and the portable skills in `plugins/simulator-ai/skills/` (one folder each). Its MCP command is the separately installed
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
(`contracts/parity-openrpc.json`, exported from one sim-api revision recorded in `parity-source.json`), and the result is the backend's own
projection, shaped by `fields`/`with` where the method has them. There are no receipt keys or contract
preflight. `read` and `graph-read` get only reads; `graph-create`, `smart-forms`, `reactions-read` and
`delegated-read` keep their own catalogs.

`console` below means `console-full`; plain `console`, which Hermes runs, keeps only the 36 tools the Console
scenarios use (see "AI Console profile").

| Tool [legacy] | Public method | console | read | graph-read |
| --- | --- | :-: | :-: | :-: |
| `actor_get` [getActor] | `actor/get` (fields, with) | ✓ | ✓ | ✓ |
| `actor_by_ref_get` [getActorByRef] · `actor_system_get` [getSystemActor] | `actor/by-ref/get` · `actor/by-system_obj/get` | ✓ | ✓ | ✓ |
| `actor_corezoid_processes_list` [getCorezoidProcesses] (details only for processes you see in Corezoid) | `actor/corezoid/process/list` | ✓ | ✓ | ✓ |
| `actors_search` [searchActors] · `actors_filter` / `actors_filter_count` [filterActors] | `actor/search/list` · `actor/filter/list` / `count` | ✓ | ✓ | ✓ |
| `search_all` [searchAll] (users and/or actors, each its own page) | one non-atomic `batch` of `user/list` + `actor/search/list` | ✓ | ✓ | ✓ |
| `actor_create` / `actors_create` [createActor] · `actor_update` [updateActor] | `actor/create` (batch) · `actor/update` | ✓ | | |
| `actor_status_update` [setActorStatus] · `actor_delete` [deleteActor] | `actor/status/update` · `actor/delete` | ✓ | | |
| `form_get` [getForm] (body, editToken with `editor`) | `form/get` | ✓ | ✓ | ✓ |
| `forms_list` / `forms_count` [getForms, searchForms] | `form/list` / `form/count` | ✓ | ✓ | ✓ |
| `form_create` · `form_update` · `form_status_update` · `form_delete` [createForm, updateForm, setFormStatus, deleteForm] | `form/create` · `form/update` · `form/status/update` · `form/delete` | ✓ | | |
| `forms_tree_get` [getFormsTree] · `form_linked_get` [getLinkedForms] | `form/tree/get` · its subtree, or `form/get` with `parents` | ✓ | ✓ | ✓ |
| `form_accounts_list` [getFormAccounts] | `account_form/list` | ✓ | ✓ | ✓ |
| `form_account_create` · `form_account_delete` [createFormAccount, removeFormAccount] | `account_form/create` · `account_form/delete` | ✓ | | |
| `edge_create` / `edges_create` [createLink, massLink] | `edge/create` (batch) | ✓ | | |
| `edge_get` [getEdge] · `edge_exist` [existLink] | `edge/get` · `edge/by-node/get` | ✓ | | ✓ |
| `edge_update` · `edge_endpoints_update` · `edge_delete` [updateEdge, deleteEdge] | `edge/update` · `edge/endpoints/update` · `edge/delete` | ✓ | | |
| `edges_by_nodes_delete` [deleteEdgesByNodes] | `batch` of `edge/by-node/delete` | ✓ | | |
| `edge_types_list` [getEdgeTypes] | `edge_type/list` (isSystem) | ✓ | | `graph_edge_types_list` |
| `layer_placements_list` · `layer_edges_list` [getLayerActorsPaginated] | `graph/layer/actors/list` · `graph/layer/edges/list` | ✓ | | ✓ |
| `layer_placements_all` [getAllLayerPlacements] | `graph/layer/actors/list`, every page | ✓ | | ✓ |
| `layer_skeleton_get` [getLayerActors] | `graph/layer/skeleton/get` | ✓ | | ✓ |
| `layer_placements_manage` [manageLayerActors] (`layer_placements_create` for flowchart blocks) | `graph/layer/actors/manage` | ✓ | | |
| `layer_positions_update` [updateLayerPositions] · `layer_actors_move` [moveActors] · `layer_clean` [cleanGraphLayer] | `…/positions/update` · `…/actors/move` · `graph/layer/clean` | ✓ | | |
| `layer_element_exist` [existLayerElement] · `layer_actors_search` [searchLayerActors] | `graph/layer/element/exist` · `graph/layer/actors/search/list` | ✓ | | ✓ |
| `layer_stats_get` [layerStats] | `graph/layer/stats/get` | ✓ | | `graph_layer_stats_get` |
| `graph_list` [getRelatedActors] (hierarchy by default) | `graph/list` | ✓ | | ✓ |
| `graph_get` [getActorLinks, getLinkedActors] | `graph/get` | ✓ | | ✓ |
| `layer_compact` [compactGraphLayout] · `layer_prune_long_edges` [pruneLongEdges] | layer reads + `…/positions/update` · `edge/delete` | ✓ | | |
| `access_rules_get` [getAccessRules, getTemplateActorsAccess, getTreeLayerAccess] (objType actor, formTemplate, templateActors, account) | `access_rule/<objType>/list` | ✓ | | |
| `access_rules_save` [saveAccessRules, saveTemplateActorsAccess, saveTreeLayerAccess] (`mode` merge, or replace: everyone not named is revoked) | `access_rule/<objType>/share` (replace reads `…/list` first) | ✓ | | |
| `access_request_create` [requestAccess] (an actor of the host workspace) · `access_requests_list` · `access_request_confirm` · `access_request_cancel` | `access_rule/actor/request/create` · `…/list` · `…/confirm` · `…/cancel` | ✓ | | |
| `reactions_list` [getReactions] · `reactions_count` · `reactions_mark_read` [markReactionsRead] | `reaction/list` · `reaction/count` · `reaction/read/update` | ✓ | | |
| `reaction_create` · `reaction_update` · `reaction_delete` [createReaction, updateReaction, deleteReaction] | `reaction/create` · `reaction/update` · `reaction/delete` | ✓ | | |
| `users_search` [searchUsers, getUsers] (without query a page of members) · `user_get` [getUser] · `workspaces_list` [getWorkspaces] | `user/list` · `user/get` · `workspace/list` | ✓ | | |
| `attachments_list` [getAttachments] · `attachments_by_actor_list` [getActorAttachments] | `attachment/list` · `attachment/by-actor/list` | ✓ | | |
| `attachment_attach` · `attachment_update` · `attachment_detach` [addAttachments, updateAttachment, removeAttachments] | `attachment/attach` · `attachment/update` · `attachment/detach` | ✓ | | |
| `attachment_upload` (file bytes as base64, up to 700 KiB; `actorId` attaches it) | HTTP `POST /api/2.0/rest/upload` (multipart, ufs limits and antivirus), which attaches it to `actorId` or refuses before storing | ✓ | | |
| `attachment_read` [readAttachment] (text up to 100 KiB; binary: type and size) | HTTP `GET /api/1.0/download/<fileName>` (425 retried for about 15 s; 404, from a stand serving files from ufs while it scans, for about 3 s; 403 quarantine) | ✓ | | |
| `account_pair_create` [createAccountPair] (by name; creates a missing name and currency; an existing pair nobody was given access to only with `takeOwnership`) | `account_name/list`, `account_currency/list`, the missing `…/create`, `access_rule/account/list` for an existing pair, `account/create` | ✓ | | |
| `account_create` [createAccount] · `account_by_ref_create` · `account_update` [updateAccount] · `account_status_update` · `account_delete` [deleteAccount] · `account_pair_unregister` | `account_actor/create` · `…/by-ref/create` · `…/update` · `…/status/update` · `…/delete` · `account/delete` | ✓ | | |
| `accounts_list` [getAccounts] · `accounts_count` · `accounts_by_ref_list` · `account_get` [getBalance] · `account_by_ref_get` | `account_actor/list` · `…/count` · `…/by-ref/list` · `…/get` · `…/by-ref/get` | ✓ | | |
| `currencies_list` [getCurrencies, searchCurrencies] · `currency_get` · `currency_create` [createCurrency] · `currency_update` | `account_currency/list` · `…/get` · `…/create` · `…/update` | ✓ | | |
| `account_names_list` [getAccountNames, searchAccountNames] · `account_name_get` · `account_name_create` [createAccountName] · `account_name_update` [updateAccountName] | `account_name/list` · `…/get` · `…/create` · `…/update` | ✓ | | |
| `counter_get` [getCounters] · `counter_by_period_get` · `counter_increment` [saveCounters] · `counter_set` [setCounters] | `counter/get` · `…/by-period/get` · `…/increment` · `…/set` | ✓ | | |
| `transaction_create` [createTransaction, finalizeTransaction] · `transaction_by_actor_ref_create` | `transaction/create` (type, parentId) · `transaction/by-actor_ref/create` | ✓ | | |
| `transaction_get` · `transaction_by_ref_get` [getTransactionByRef] · `transactions_list` [getTransactions] · `account_transactions_list` [getAccountTransactions] · `transactions_filter` / `_count` · `transaction_children_list` | `transaction/get` · `…/by-ref/get` · `…/list` · `…/account/list` · `…/filter` / `count` · `…/children/list` | ✓ | | |
| `transfer_create` [createTransfer, createTransferTwoStep] · `transfer_get` [getTransfer] · `transfer_by_ref_get` [getTransferByRef] · `transfers_filter` [filterTransfers] / `_count` | `transfer/create` · `…/get` · `…/by-ref/get` · `…/filter` / `count` | ✓ | | |
| `account_by_id_get` [getAccount] · `account_amount_set` [setAccountAmount] | `account_actor/by-id/get` · `account_actor/amount/update` | ✓ | | |
| `account_formula_get` [getAccountFormula] · `account_formula_preview` · `account_formula_set` [setAccountFormula] | `account_actor/formula/get` · `…/preview` · `…/update` | ✓ | | |
| `atom_transaction_create` [atomCreateTransaction] | `transaction/atom/create` | ✓ | | |
| `account_tags_list` · `account_tag_add` · `account_tags_set` · `account_actor_tags_list` / `_count` · `account_triggers_list` / `_set` · `account_actor_triggers_list` / `_set` [saveAccountActors] | `account/tags/list` · `…/create` · `…/update` · `account_actor/tags/list` / `count` · `account/triggers/*` · `account_actor/triggers/*` (every `_set` replaces the set) | ✓ | | |
| `actor_field_triggers_list` / `_set` [getDataFieldActorsByActor, saveDataFieldActorsByActor] | `actor/field/triggers/list` / `update` | ✓ | | |
| `skills_find` [findSkill] (verified only, no body) · `skill_get` [getSkill] (by ref or id, with the procedure) | `form/system/get` (`Skills`) + `actor/filter/list` · `actor/by-ref/get` or `actor/get` | ✓ | | |
| `agents_find` [findAgent] (members without query) · `agent_get` [getAgent] (a user's twin or any actor, with the profile) | `user/list`, or `form/system/get` (`System`) + `actor/search/list` (semantic, else text), or `actor/filter/list` of a `formId` · `actor/by-system_obj/get` or `actor/get` | ✓ | | |
| `build_link` [buildLink] (actor, layer, form, chat, chart, … and a Smart Form route) | none: the legacy web app route table on `SIMULATOR_AI_ORIGIN`; a Smart Form's short URL from `route/build` (its production release) | ✓ | | |
| `bbcode_tags_get` [getBbcodeTags] | none: the BBCode catalog of simulator-ui-kit `packages/utils/src/bbcode/bbcode-tags.json`, built in | ✓ | | |
| `chart_create` [createChart] (manual series, or the top actors of an ActorFilters actor) | `form/system/get` (`Dashboards`, `ActorFilters`), `actor/create` of the filter and the chart, `graph/layer/actors/manage` | ✓ | | |
| `chart_update` [updateChart] (settings, series or filter; with `laId` the placement's size) | `actor/get` + `form/system/get`, a new filter's `actor/create`, `actor/update`, `graph/layer/elements/list` + `graph/layer/actor_settings/save` | ✓ | | |
| `smart_form_folders_create` (a new page's `pages/<page>`) | `app_folder/create` | ✓ | | |
| `smart_form_releases_list` · `smart_form_release_diff` · `smart_form_release_rollback` | `release/list` · `release/diff` · `release/rollback` | ✓ | | |
| `smart_form_file_versions_list` · `smart_form_file_version_get` · `smart_form_file_rollback` | `file_version/list` · `file_version/get` · `file/rollback` | ✓ | | |
| `smart_form_trash_list` · `smart_form_trash_restore` | `trash/list` · `trash/restore` | ✓ | | |
| `smart_form_page_render` · `smart_form_page_submit` (a page run as the runtime does; submit runs its process) | `page/render` · `page/submit` | ✓ | | |
| `corezoid_projects_list` · `corezoid_stages_list` · `corezoid_processes_list` · `corezoid_project_get` | `corezoid/project/list` · `corezoid/stage/list` · `corezoid/process/list` · `corezoid/project/get` | ✓ | | |
| `corezoid_project_bind` · `corezoid_project_create` (bind a Smart Form's envs to processes, or give it a project) | `corezoid/project/bind` · `corezoid/project/create` | ✓ | | |
| `layer_edge_settings_save` (an edge's line style, width, colour, label on one layer) | `graph/layer/edge_settings/save` | ✓ | | |
| `actor_picture_upload` [uploadActorPicture] (PNG, JPEG, GIF or WebP; no SVG) | HTTP `POST /api/2.0/rest/upload` (attached to nothing) + `actor/update` `picture` = its fileName, as the web app writes | ✓ | | |
| `graph_export_create` [exportGraph] · `graph_import_create` [importGraph] (no Corezoid `scripts`) | `task/export/create` · `task/import/create` | ✓ | | |
| `task_get` [getTaskStatus] (adds `downloadUrl` for an archive) · `graph_file_upload` [uploadGraphFile] (base64, up to 700 KiB) | `task/get` · HTTP `POST /api/2.0/rest/upload` as `application/octet-stream`, attached to nothing | ✓ | | |
| `simulation_snapshot_get` [simulationSnapshot] · `simulation_check` [simulationCheck] · `simulation_run` [simulationRun] (the engine runs in the plugin and writes nothing) | reads `graph/layer/actors/list` · `graph/layer/edges/list` · `edge_type/list` · `account_actor/list` (`form/get` names a graph's bare form ids) | ✓ | | |

These tools need sim-api `feature/simulator-ai` at 84ec6ff1 or later (`cdu/catalog/get` !463,
`actor/corezoid/process/list` !465, `form/tree/get`, `edge/by-node/get`, the Smart Form history, trash, page and
Corezoid methods, the task methods !494); on an older backend they fail with reason `methodNotFound`. The rollback, restore, page submit, graph
import and Corezoid bind/create tools are hinted destructive; page render is not read-only either (it runs the page's `/get`).

An AI Console run's token reaches `rest/upload` (and no other REST route) through sim-api !481
(`AuthReqAllowingAgentRun`); an older stand refuses it, and `attachment_upload` returns that refusal. On a stand
serving downloads from ufs, a file under scan or in quarantine answers 404, so a read right after an upload is
retried for about 3 seconds and then reports "no such file, or its antivirus scan has not finished". The skills `simulator-ai-access`,
`simulator-ai-attachments`, `simulator-ai-chat`, `simulator-ai-tasks`, `simulator-ai-finance`, `simulator-ai-skills`,
`simulator-ai-agents` and `simulator-ai-charts` compose these tools.

Skill procedures and agent profiles are descriptions users write: `skill_get` and `agent_get` return them as data, and
their tool descriptions tell the model not to take instructions from them. `build_link` builds web app URLs on
`SIMULATOR_AI_ORIGIN` with the 8-character workspace segment, each id escaped as one path segment; a `smartForm`
link needs a route its released pages declare.

A chart's `data.source` is built locally as legacy createChart built it: the web app wizard's JSON string, which
sim-api's `dashboard/get` reads too. The chart is drawn as `layerSettings` `{expandType: chart, expand: true, width: 650,
height: 450}`: legacy wrote that box as `offset` `{left, right, top, bottom}`, which sim-api types as `{x, y}` and would
store as zeros. `chart_update` keeps every stored key it does not change and never edits an ActorFilters actor (a
changed filter is a new one; the result names the one the chart stopped using). Legacy's account inheritance from the
layer's graph (`/accounts/inherit`) has no public method and is not set up.

Simulation is the legacy engine moved as it was (`internal/simulation`, its conformance cases included): a model
runs in model time in memory and nothing is written to Simulator. `simulation_snapshot_get` returns the layer
YAML instead of writing `<layerId>.sim.yaml`, and `simulation_check` / `simulation_run` take it back as `graph`
(there are no file paths); a formula account keeps its formula next to its current value, which a run does not
recompute. A snapshot reads accounts of at most 2000 actors; `simulation_run` takes at most 5 minutes, live layer
read included, and two at once per process (`SIMULATOR_AI_MAX_CONCURRENT_SIMULATIONS`); another is refused as busy. The skill `simulator-ai-simulate` (with the model format in its `references/`) composes them.

`graph_import_create` leaves out `scripts`: Corezoid API secrets never travel in tool arguments, so an imported
Smart Form is bound afterwards with `corezoid_project_bind`. Legacy `uploadGraphFile` also fetched a `fileUrl`; here the
archive comes as base64 only, and the plugin fetches no URL.

Not covered yet, and why: pullGraphFile / pushGraphFile, a local file ↔ layer sync engine on public methods,
too large for this slice; getChildAccounts, since account trees were removed from the platform (walk the
children with `graph_list` and read their `accounts_list`). Out of scope by the owner's decision: pinned reactions, uploadBase64 and data-field actors
by form.

## AI Console profile

Every tool schema rides in the model context on every turn, so `SIMULATOR_AI_PROFILE=console` is the core
catalog of the Console scenarios: `actor_get`, `actor_create`, `actors_create`, `actor_update`, `actors_search`,
`actor_by_ref_get`, `form_get`, `forms_list`, `graph_get`, the flowchart tools, the Smart Form tools below and,
for a comment, a chat, a task or a file, `reactions_list`, `reaction_create`, `users_search`, `access_rules_get`,
`access_rules_save`, `attachment_upload` and `attachment_read`, and for a saved playbook and a link to what was made
`skills_find`, `skill_get` and `build_link`: 36 in all (`smart_form_folders_create` is there because a form of more
than one page needs a folder per page). `console-full` adds every other actor, form, graph, access, reaction, user,
attachment, finance, registry, simulation, chart, Smart Form history, Corezoid and graph export/import tool (173 in
all). Parameter descriptions are cut to their first sentence and nested fields keep only
their types. List rows and graph nodes are `id`, `title`, `formId` unless the call sends `fields`.

Actors get the editor's colours: without `color`, `actor_create` and `actors_create` pick a random hue of
simulator-ui-kit `themes/actorHues.ts` (the eight chromatic ones; red and slate are never random there either),
`flowchart_layout` gives each block its form's own colour or such a hue, and `layer_placements_create` paints a
flowchart block with its actor's colour when the call has none. Holes: `actors_create`/`actor_create` with
`hole` make a placeholder actor (its data fields are the conditions that close it), `edge_create`/`edges_create`
with `hole` a placeholder edge, and `layer_get` marks `hole` and `closedHole` on nodes and edges.

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
| `smart_form_components` | `cdu/catalog/get` | with `brief` every type's kind, category and label; with `types` the props the validator applies, events, state and api |
| `smart_form_data_form_create` | `form/create` (data template) | formId and a ready page whose button submits via `form/submit` into it |
| `smart_form_files_get` | `application/env/list` + `application/struct` (develop) | folders/files by path, optional sources, `developEnvId`, `productionEnvId` |
| `smart_form_files_update` / `smart_form_files_create` | `app_file/update` / `app_file/create` | id, folderId, title, sourceHash |
| `smart_form_folders_create` | `app_folder/create` | folders with ids (a page is `pages/<page>`) |
| `smart_form_release` | `application/env/list` + `release/create` (develop → production) | `{appId, releaseId, releaseNumber, status, ready, marker}` |
| `smart_form_release_get` | `application/env/list` + `release/get` (without `releaseId`: `route/key/get` names the active one) | same release result |

Flowchart: `flowchart_blocks_list` (kinds) → `flowchart_layout(nodes, edges)` → `actor_create(formId, title,
color, data)` per node → `layer_placements_create` nodes with the layout's `position`, `layerSettings` and the same
`color` → one `edges_create` (`name` is the label) → `layer_placements_create` edges between the node `laId`s →
`layer_get` to read it back. Geometry follows Graph Editor: a cell is 50 units and grid lines lie on cell
boundaries (25 + 50k); a position is the block centre and a block's top-left corner sits on a grid line, so a
block of whole cells fills whole cells. The layout ranks nodes top to bottom (loop-back edges excluded from
ranking) with three free cells between blocks and ranks. Like the editor, the placement carries the block
silhouette as `areaPicture`, which makes it graphic, so sim-api keeps that position instead of snapping the centre
to a multiple of 50 (half a cell off for even-sized blocks). Smart Form: a form the user fills in saves what is submitted as an actor of its own data template.
`smart_form_create` first, then `smart_form_data_form_create` creates that template from the fields and returns a ready page (a Form node, bound
inputs, a button that validates and calls `form/submit` via simapi with the template's formId). Its limits: field
types are only text, number (an `edit/float` field and a `float` TextInput), date (a calendar holding
`{startDate, endDate}`) and select, all in one section; empty values are left out of the submit (its `formData`
filters the Form's `vm` values); the actor is titled by the first text field, or in a template without one by the
template title (its first 255 characters), so every record of a template without a text field has the same
title; text of the agent holding `{{` is written as a JSONata string literal, so the renderer shows it as typed;
the stand must serve `form/submit`. A submit needs View on the template, which only its creator has, so the page
passes `appId`: `form/submit` then lets whoever may open that Smart Form (its `sharedWith`, as `smart_form_create`
set it) submit into a template its released page names. Page writes refuse inputs with an action-less button but no Form, a Form that neither a form.submit names
nor any action passes on (`vm.<valueKey>` in a `simapi.call`, `widget.call`, `bus.publish`, `navigate.local`/`navigate.route` or `copy`; their `onError`/`notify` follow-ups do not count), a `form/submit` without a template and a `formData`
filter reading another Form, and set its `accId` to the host workspace and its `appId` to that Smart Form; other submits (via Corezoid, other
methods) are left as they are. Then
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
placements read back with `layer_get`, then a Smart Form with a field, a submit button and a root route, released,
read back as `ready` and linked with `build_link`, then a verified skill found by `skills_find`, read by `skill_get`
and linked. With `console-full` it runs the parity, people and finance scenarios and the agents one (a member's twin
by `agent_get`, found again by `agents_find`, and `bbcode_tags_get`), the simulation one (a shop with a balance
of 100 and a supplier on a new layer; a payment model checked against `simulation_snapshot_get` and run live in two
scenarios) and the charts one (a manual chart read back with its source and chart placement, turned into an
actorFilter chart with a new filter and resized, then back to manual), the formops one (a second page in its own
folder, two releases listed and compared, a rollback read back as the active release without its id, the page's
file history and a rollback of its edit, the trash, a develop render, no Corezoid project yet) and the extras one
(an edge drawn dashed, 3 wide and red, an actor's picture, the first Corezoid project's stages and processes;
nothing is bound) and the jobs one (a layer exported, its archive downloaded by `downloadUrl`, uploaded again with
`graph_file_upload` and imported as a copy; it needs the stand's control-tasks worker). Each run creates new objects in that
workspace; nothing secret is printed.

The same file connects the desktop agents with the read profile. Start them from a shell that loaded it:

```sh
set -a; . ~/.config/simulator-ai/smoke.env; set +a; export SIMULATOR_AI_PROFILE=read
claude mcp add --scope user simulator-ai -- simulator-ai   # once; then run `claude`
codex mcp add simulator-ai -- simulator-ai                 # once; add env_vars (above) incl. SIMULATOR_AI_AUTHORIZATION, then run `codex`
```

`claude mcp list` shows `simulator-ai ... ✔ Connected`. One read call through each agent's own model, with a
workspace API key (`make -C <sim-api> ai-smoke-env SMOKE_KIND=apikey SMOKE_OUT=~/.config/simulator-ai/r1-apikey.env`)
loaded as above and `SIMULATOR_AI_PROFILE=read`; the MCP config is passed for this run only:

```sh
claude -p "Call the simulator-ai MCP tool forms_list with type system and limit 3, then reply with only the titles it returned, comma-separated." \
  --mcp-config r1-mcp.json --strict-mcp-config --allowedTools mcp__simulator-ai__forms_list --max-turns 3
# r1-mcp.json: {"mcpServers":{"simulator-ai":{"command":"/abs/path/bin/simulator-ai","args":[]}}}
codex exec -s read-only -c approval_policy='"never"' -c 'mcp_servers.simulator-ai.command="/abs/path/bin/simulator-ai"' \
  -c 'mcp_servers.simulator-ai.env_vars=["SIMULATOR_AI_ORIGIN","SIMULATOR_AI_WORKSPACE","SIMULATOR_AI_API_KEY","SIMULATOR_AI_PROFILE"]' \
  "Call the simulator-ai MCP tool forms_list with type system and limit 3, then reply with only the titles it returned, comma-separated."
```

On the local stand (2026-11-04) Claude Code 2.1.312 answered `Scripts, Widgets, CompanyCategories` in 3 turns, and
Codex 0.166 (the ChatGPT.app CLI; `mcp: simulator-ai/forms_list (completed)`) gave the same answer. An API user
without workspace roles sees only system templates.

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
