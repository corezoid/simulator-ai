[← Overview and installation](../README.md)

# Tool reference

Actor, form and graph tools mirror the legacy `simulator-ai-plugin` tools (name in brackets) on one public
method each, or on a few for the composite ones. Their input schemas are the pinned public params
(`contracts/parity-openrpc.json`, exported from one sim-api revision recorded in `parity-source.json`), and the result is the backend's own
projection, shaped by `fields`/`with` where the method has them. There are no receipt keys or contract
preflight. `read` and `graph-read` get only reads; `graph-create`, `smart-forms`, `reactions-read` and
`delegated-read` keep their own catalogs.

The `console` column below denotes the complete `console-full` catalog. The plain `console`
profile exposes a focused subset for interactive tasks (see "AI Console profile").

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
| `form_fields_patch` (ops by field id: add, update, remove, move, sections; template properties; optional expected `editToken`) | `form/get` with `editor`, then `form/update` under its token | ✓ | | |
| `forms_tree_get` [getFormsTree] · `form_linked_get` [getLinkedForms] | `form/tree/get` · its subtree, or `form/get` with `parents` | ✓ | ✓ | ✓ |
| `form_accounts_list` [getFormAccounts] | `account_form/list` | ✓ | ✓ | ✓ |
| `form_account_create` [createFormAccount] (pair by name or ids; also opened on the template's existing actors, 500 per call) · `form_account_delete` [removeFormAccount] | `account_form/list` + atomic `batch` of `account/create` + `account_form/create`, then `actor/filter/list` + `batch` of `account_actor/create` · `account_form/delete` | ✓ | | |
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
| `chat_open` (p2p reused by its exact ref as the web UI does, you are the user a `Simulator` JWT names (its `sub`, matched to a member's `saId`), else `selfUserId` checked against the chat's rules; or a new group chat) | `form/system/get` (`Events`) + `actor/by-ref/get` + `user/get`, then one atomic `batch` of `actor/create` (ref, title) + `access_rule/actor/share` (`$0.id`) | ✓ | | |
| `actor_attach` (actor card, Smart Form or pinned form card on an actor) | `actor/update` (`cardActorId` / `appId`) · `edge_type/get` + `edge/create` (`pinned`), with a new form actor in one atomic `batch` | ✓ | | |
| `users_search` [searchUsers, getUsers] (without query a page of members) · `user_get` [getUser] · `workspaces_list` [getWorkspaces] | `user/list` · `user/get` · `workspace/list` | ✓ | | |
| `attachments_list` [getAttachments] · `attachments_by_actor_list` [getActorAttachments] | `attachment/list` · `attachment/by-actor/list` | ✓ | | |
| `attachment_attach` · `attachment_update` · `attachment_detach` [addAttachments, updateAttachment, removeAttachments] | `attachment/attach` · `attachment/update` · `attachment/detach` | ✓ | | |
| `attachment_upload` (file bytes as base64, up to 700 KiB; `actorId` attaches it) | HTTP `POST /api/2.0/rest/upload` (multipart, ufs limits and antivirus), which attaches it to `actorId` or refuses before storing | ✓ | | |
| `attachment_read` [readAttachment] (by `kind`: text up to 100 KiB; png/jpeg/gif/webp up to 5 MiB as an MCP image block; a PDF as its text layer (the pure-Go `github.com/ledongthuc/pdf`, forked), a scan without one as up to 4 of its JPEG (`/DCTDecode`) page images; .docx and .xlsx as text from their zip/XML parts; anything else type and size. Hermes passes image blocks to the model but parks an embedded PDF in a file the agent cannot read, so no PDF block is sent. Bounds: a PDF's text is read only up to 10 MiB, by the parser forked into `internal/third_party/pdf` with a read budget in its stream reader (32 MiB produced by all filter layers of the file, a deadline of 10 s, bounded xref and predictor sizes), page by page (200 pages, 100 KiB); docx/xlsx within 32 MiB unpacked per file, 5 s, 32 sheets and 512 columns. A per-run image limit belongs to the backend: Hermes starts the plugin per tool call) | HTTP `GET /api/1.0/download/<fileName>` (425 retried for about 15 s; 404, from a stand serving files from ufs while it scans, for about 3 s; 403 quarantine) | ✓ | | |
| `account_pair_create` [createAccountPair] (by name; creates a missing name and currency; an existing pair nobody was given access to only with `takeOwnership`) | `account_name/list`, `account_currency/list`, the missing `…/create`, `access_rule/account/list` for an existing pair, `account/create` | ✓ | | |
| `account_create` [createAccount] (pair by account name and currency or by ids, as ui-kit's create-account; `plan` adds the plan twin) · `account_by_ref_create` · `account_update` [updateAccount] · `account_status_update` · `account_delete` [deleteAccount] · `account_pair_unregister` | atomic `batch` of `account/create` + `account_actor/create` (`ignoreIfExist`) · `account_actor/by-ref/create` · `…/update` · `…/status/update` · `…/delete` · `account/delete` | ✓ | | |
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
| `build_link` [buildLink] (actor, layer, form, chat, chart, … and a Smart Form route; with `appKey`/`appId` inside the current Smart Form) | `route/build` (`dynamic` on the current form's key, `route/key/get` for an `appId`) plus the block's `#b=` fragment; the legacy web app route table on `SIMULATOR_AI_ORIGIN` as the fallback; a Smart Form's short URL from `route/build` (its production release) | ✓ | | |
| `bbcode_tags_get` [getBbcodeTags] | none: the BBCode catalog of simulator-ui-kit `packages/utils/src/bbcode/bbcode-tags.json`, built in | ✓ | | |
| `chart_create` [createChart] (manual series, or the top actors of an ActorFilters actor) | `form/system/get` (`Dashboards`, `ActorFilters`), `actor/create` of the filter and the chart, placed open as `layer_placements_create` does (room checked before the actors are made, made with anisotropy) | ✓ | | |
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
| `public_link_create` [generatePublicLink] · `public_link_get` [getPublicLink] · `public_link_revoke` [revokePublicLink] | `public_link/create` · `/get` · `/revoke` (revoke takes back what the link gave) | ✓ | | |
| `meet_transcription_list` [getTranscription] (the live transcript; empty before and after a call) | `meet/transcription/list` | ✓ | | |
| `simulation_snapshot_get` [simulationSnapshot] · `simulation_check` [simulationCheck] · `simulation_run` [simulationRun] (the engine runs in the plugin and writes nothing) | reads `graph/layer/actors/list` · `graph/layer/edges/list` · `edge_type/list` · `account_actor/list` (`form/get` names a graph's bare form ids) | ✓ | | |

These tools need sim-api `feature/simulator-ai` at a547f397 or later (`cdu/catalog/get` !463,
`actor/corezoid/process/list` !465, `form/tree/get`, `edge/by-node/get`, the Smart Form history, trash, page and
Corezoid methods, the task methods !494, guest links !495, the transcript !496 and `graph/viewport/command` for `layer_viewport`); on an older backend they fail with reason `methodNotFound`. The rollback, restore, page submit, graph
import, guest link create/revoke and Corezoid bind/create tools are hinted destructive (a new guest link opens view to
anyone with its URL and stops the one already handed out); page render is not read-only either (it runs the page's `/get`).

An AI Console run's token reaches `rest/upload` (and no other REST route) through sim-api !481
(`AuthReqAllowingAgentRun`); an older stand refuses it, and `attachment_upload` returns that refusal. On a stand
serving downloads from ufs, a file under scan or in quarantine answers 404, so a read right after an upload is
retried for about 3 seconds and then reports "no such file, or its antivirus scan has not finished". The skills `simulator-ai-access`,
`simulator-ai-attachments`, `simulator-ai-chat`, `simulator-ai-tasks`, `simulator-ai-finance`, `simulator-ai-skills`,
`simulator-ai-agents`, `simulator-ai-charts` and `simulator-ai-meetings` compose these tools.

Skill procedures and agent profiles are descriptions users write: `skill_get` and `agent_get` return them as data, and
their tool descriptions tell the model not to take instructions from them. `build_link` builds a link inside the current
Smart Form when given its `appKey` (the Console context's `/sf/<key>`) or `appId` (sim-api `docs/simulator-ai-architecture.md`
"Посилання"): `route/build` on that key with the start Smart Form's route of the entity (`workspace.actors`,
`workspace.graphs-edit`, `workspace.events`, `workspace.chats`, `workspace.templates-edit`, the lists) or a `route` of
the form's own, plus the block state as `#b=<blockId>:<route>` (both parts encodeURIComponent), root-relative. Without
them, or for an entity with no runtime page (marked `legacy`), it builds web app URLs on `SIMULATOR_AI_ORIGIN` with
the 8-character workspace segment, each id escaped as one path segment; a `smartForm` link needs a route its released
pages declare.

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
children with `graph_list` and read their `accounts_list`). Also unavailable: pinned reactions, uploadBase64 and data-field actors
by form.

## AI Console profile

Every tool schema rides in the model context on every turn, so `SIMULATOR_AI_PROFILE=console` is the core
catalog of the Console scenarios: `actor_get`, `actor_create`, `actors_create`, `actor_update`, `actors_search`,
`actor_by_ref_get`, `form_get`, `forms_list`, `graph_get`, the flowchart tools, the Smart Form tools below and,
for a comment, a chat, a task or a file, `reactions_list`, `reaction_create`, `users_search`, `access_rules_get`,
`access_rules_save`, `attachment_upload` and `attachment_read`, and for a saved playbook and a link to what was made
`skills_find`, `skill_get` and `build_link`, and for accounts, Account Templates, chats and cards `account_create`,
`accounts_list`, `transfer_create`, `transaction_create`, `form_create`, `form_fields_patch`, `form_update`,
`form_account_create`, `chat_open` and `actor_attach`, and for a Smart Form's business logic `corezoid_project_create` and
`corezoid_project_get`, `layer_viewport` to show the user what the answer is about, `hole_close` to close a placeholder
`actor_delete` for a delete named outside a layer, `smart_form_page_patch` to change part of a page and
`smart_form_reference_grep` / `smart_form_reference_read` to look the CDU format up: 54 in all (`smart_form_folders_create` is there because a form of more
than one page needs a folder per page; `internal/mcpserver/console.go` says why each later one is). `console-full` adds every other actor, form, graph, access, reaction, user,
attachment, finance, registry, simulation, chart, Smart Form history, Corezoid, graph export/import, guest link and
transcript tool, hole reopen, Pencil drawing, layer settings and nodes opened in place (the complete catalog). Parameter descriptions are cut to their first sentence and nested fields keep only
their types. List rows and graph nodes are `id`, `title`, `formId` unless the call sends `fields`.

Graph reads answer compact unless the call sends `full: true` (a large layer's reads were the biggest outputs of a
dialog; every one rides in the context of each later step): `layer_get` as in the table below;
`layer_placements_list` and `layer_actors_search` drop each row's `areaPicture` (a block's base64 SVG),
`cellPosition`, `createdAt`, `layerSettings.shape`/`textFrame` and the actor's copy of the row id, and round
positions; `layer_edges_list` keeps of each edge its type, ends, label and hole; `layer_skeleton_get` answers at
most 150 nodes and the edges between them with `total`, the `box` every node centre lies in and `truncated`; `layer_placements_all`
answers `count`, `summary {forms [{formId, count}], box}` and the first `limit` (150) rows by `cell`. Tool groups: Console entries in `tools/list` carry `_meta: {"simulator-ai/groups": [...]}`.
Hosts can select `core` plus the domains needed for a task. A tool can belong to multiple groups.
Groups include graph, smart_forms, forms, finance, finance_setup, people, files, meetings,
jobs, charts, simulation and corezoid. The plain `console` catalog is a curated subset,
not the union of all these groups. Use `tools/list` for the current inventory.

Actors get the editor's colours: without `color`, `actor_create` and `actors_create` pick a random hue of
simulator-ui-kit `themes/actorHues.ts` (the eight chromatic ones; red and slate are never random there either),
`flowchart_layout` gives each block its form's own colour or such a hue, and `layer_placements_create` paints a
flowchart block with its actor's colour when the call has none. Holes: `actors_create`/`actor_create` with
`hole` make a placeholder actor (its data fields are the conditions that close it), `edge_create`/`edges_create`
with `hole` a placeholder edge, and `layer_get` marks `hole` and `closedHole` on nodes and edges. `hole_close`
closes an actor hole with an existing actor or a new one (atomic `actor/create` + `actor/hole/merge`; the new closer
starts from the hole's template and data, its conditions), `edge_hole_close` a placeholder link with an existing edge
or a new one from the hole's endpoints and type (`edge/create` + `edge/hole/merge`); `hole_conditions_get`,
`hole_reopen` and `edge_hole_reopen` are the dry run and the reverts (`console-full`).

`SIMULATOR_AI_PROFILE=console` is what Hermes runs per AI Console request under the initiator's own
`SIMULATOR_AI_AUTHORIZATION`. It covers two Console answers: a flowchart drawn on a layer and a released
Smart Form opened inline in the reply. Writes are plain public calls: no receipt keys and no contract
preflight, because sim-api deduplicates Console retries. Each tool returns only ids and what the next
step needs. The run's prompt carries sim-api's "Current context" (the layer, actor and Smart Form the user has
open): `layer_create` and `layer_placements_create` tell the agent to build on that layer unless the user names
another, never on a new one.

| MCP tool | Public RPC | Result |
| --- | --- | --- |
| `actor_get` | `actor/get` | id, title, description, formId, status, ref |
| `actor_create` / `actor_update` | `actor/create` / `actor/update` | same actor subset |
| `actors_create` / `edges_create` | one atomic `batch` of `actor/create` / `edge/create` (edge types by name) | actors / edges in input order; one failed item fails the call with its index and creates nothing |
| `flowchart_blocks_list` | `form/flowchart-block/search` | formId, title, blockId, `hasMore`; with `blockIds` also ready `data` and `layerSettings` |
| `flowchart_layout` | `form/flowchart-block/search` (read-only) | per node formId, data, grid position and layerSettings; `returnEdges` |
| `layer_create` | `form/system/get` (`Layers`) + `actor/create` | layer actor |
| `edge_create` | `edge_type/get` (default `hierarchy`) + `edge/create` | id, source, target, edgeTypeId, name |
| `layer_placements_create` | `actor/get` (the layer's `data.anisotropy`) + `graph/layer/actors/list` (what each placement holds: a cell, an open box, a block's box) + `graph/layer/actors/manage` (create only); a flowchart node is border-snapped and gets the block's `areaPicture`. A taken area is refused, or with anisotropy made room for: atomic `batch` of `graph/layer/actors/offset` (a seam through the area) + manage. `expand` on the only node places it open (the `layer_node_expand` bag, its push first) | nodes `{laId, actorId, position, cell}`, edges `{laId, edgeId, laIdSource, laIdTarget}`, `moved` (each node once), `opened {expandType, area}`, `covers` |
| `layer_positions_update` | `graph/layer/actors/positions/update` | updated count |
| `layer_get` | `actor/get` (`data.anisotropy`) + `graph/layer/edges/list` + `graph/layer/actors/list` (+ `graph/layer/stats/get` when `hasMore`) | compact nodes `{laId, actorId, title, formId, cell, blockId, hole, open}` and edges `{laId, edgeId, laIdSource, laIdTarget, name, hole}` (+ `source`/`target` when a node is off the page), 100 per kind, `anisotropy`, `hasMore` with `total` (best effort: 2 s, none on an error); `full` adds `position` and edge `source`/`target` (limit 200) |
| `layer_node_expand` (`console-full`) | `graph/layer/elements/list` (the node) + the layer read above + `form/system/get` (`Scripts`, `Dashboards`) → atomic `batch` of `graph/layer/actor_settings/save` (`expand`, `expandType`, `flaskView`, `nestedFrame`, `expandPushed`, legacy `width`/`height`) + `graph/layer/actors/offset` (anisotropy; the close mirrors `expandPushed`), as simulator-ui-kit `expandOps` / `collapseOps` | `{laId, actorId, expanded, changed, expandType, area, pushed, moved, covers}` |
| `layer_viewport` | `graph/viewport/command` (a ruler `cell` sent as its index: `"C, 7"` → `{x: 2, y: 6}`) | `commandId`, `openTabs` (0: the layer is not open, nothing moved) |
| `hole_close` | `actor/hole/merge`, or `actor/get` (the hole) + atomic `actor/create` + `actor/hole/merge` | `{holeActorId, actorId, created, changes}` (+ `closingProgress`, `conditions` with `forceMerge`) |
| `edge_hole_close` | `edge/hole/merge`, or `edge/get` (the hole) + atomic `edge/create` + `edge/hole/merge` | `{holeEdgeId, edgeId, created, changes}` |
| `layer_draw` (`console-full`) | `internal/drawing` PNG (2 px per unit, as the editor's Pencil confirm) → `rest/upload` → atomic `actor/create` (layer's `defForm`, `picture`, `pictureObject`) + `graph/layer/actors/manage` (`layerSettings.image`) + `attachment/attach` | `{actorId, laId, picture, position, cell, width, height}` |
| `layer_settings_update` (`console-full`) | names → `account_name/list` / `account_currency/list`; `form/get` for the `defForm` title; one `actor/update` of the layer's `data` | `{layerId, keys}` |
| `smart_form_create` | `application/create` (`cduVersion: 2`) | appId, ref, developEnvId, productionEnvId |
| `smart_form_components` | `cdu/catalog/get` | with `brief` every type's kind, category and label; with `types` or `kind` one line per prop, event, state key and api method (TypeScript-like, `$ref`s resolved, enums inlined, inherited HTML/aria/on* attributes left out; Stack+Form 60 KB of schema → 4.7 KB); `schema: true` the validator's JSON Schemas |
| `smart_form_reference_grep` / `smart_form_reference_read` | `cdu/catalog/get` (all types, once per 10 minutes per process) for `components`; `protocol` and `examples` are embedded | plain text, grep-like: case-insensitive RE2 hits `doc:line: text` with context lines and the section heading (≈ 6 000 chars at most); reads by line or by section `doc/<section>` (8 000 chars a page); see "CDU reference" |
| `smart_form_data_form_create` | `form/create` (data template) | formId and a ready page whose button submits via `form/submit` into it |
| `smart_form_files_get` | `application/env/list` + `application/struct` (develop); with `paths` + `app_file/get` per named file | folders/files by path, optional sources (`paths`: only the named files and folders, each file with its source; `missing`), `developEnvId`, `productionEnvId` |
| `smart_form_page_patch` | `application/env/list` + `application/struct` (by `path`) + `app_file/get` + `app_file/update` (`ifMatchHash` = the hash read) | RFC 6902 ops (add, replace, remove, move, copy, test; with `nodeId` relative to that tree node) on one develop JSON file (a page config or a definition `definitions/<name>`; a `$ref` instance takes no type, props, children or actions), all or none: `{appId, fileId, path, sourceHash, applied}`, never the source |
| `smart_form_files_update` / `smart_form_files_create` | `app_file/update` / `app_file/create` | id, folderId, title, sourceHash |
| `smart_form_folders_create` | `app_folder/create` | folders with ids (a page is `pages/<page>`) |
| `smart_form_release` | `application/env/list` + `release/create` (develop → production) | `{appId, releaseId, releaseNumber, status, ready, marker}` |
| `smart_form_release_get` | `application/env/list` + `release/get` (without `releaseId`: `route/key/get` names the active one) | same release result |

Flowchart: `flowchart_blocks_list` (kinds) → `flowchart_layout(nodes, edges)` → `actor_create(formId, title,
color, data)` per node → `layer_placements_create` nodes with the layout's `position`, `layerSettings` and the same
`color` → one `edges_create` (`name` is the label) → `layer_placements_create` edges between the node `laId`s →
`layer_get` to read it back. Cells follow the Graph Editor rulers (simulator-ui-kit `utils/src/graphCoordinates.ts`,
ported as `publicapi.CellOf` / `CellCenter`): `"C, 7"` is column C, row 7; columns run A…Z, A1…Z1, A2…, `-A`… left
of the origin, rows 1… down and `-1`… up, no 0; the cell `A, 1` is centred on the origin. `layer_get`,
`layer_placements_create` and `flowchart_layout` answer each node's `cell`, and `layer_placements_create` /
`layer_positions_update` take `cell` instead of a position (the cell's centre). Geometry follows Graph Editor: a cell is 50 units and grid lines lie on cell
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
set it) submit into a template its released page names. Page writes refuse a `form/submit` without a template and a `formData`
filter reading another Form, and set its `accId` to the host workspace and its `appId` to that Smart Form; other submits (via Corezoid, other
methods) are left as they are. A page that sends nothing yet (inputs with an action-less button but no Form, a Form that neither a form.submit names
nor any action passes on: `vm.<valueKey>` in a `simapi.call`, `widget.call`, `bus.publish`, `navigate.local`/`navigate.route` or `copy`, their `onError`/`notify` follow-ups not counting)
is saved with `pending` naming what is missing, so a large page can be built section by section; `smart_form_release` reads the develop
page configs (`application/struct` with sources) and refuses to release one still in that state. Then
`smart_form_files_get` → `smart_form_files_update` of `pages/index/config` → `smart_form_release`; a later change to a page is
`smart_form_files_get {paths}` → `smart_form_reference_grep` / `smart_form_reference_read` (or
`smart_form_components {types}`) → `smart_form_page_patch` by `nodeId`. Its
`ready: true` (status `active`, snapshot kept, production env) comes with `marker: "appId: <appId>"`. The
agent ends its final answer with that line, and sim-api reads it from the Markdown (the Hermes run stream carries
no tool results) to put `appId` into the reply reaction with `appSettings.autorun`.

