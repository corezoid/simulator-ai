# Public API mappings

[Tool guide](tool-reference.md) · [Generated profile availability](tool-catalog.md)

These mappings explain public API calls and operational limits. Exact tool availability
comes from the generated catalog, not from this table. Parameter schemas are exposed by
`tools/list`; a compatible backend must provide the mapped public methods.

Legacy names in brackets help migration from the older Simulator plugin. They do not promise
identical result envelopes or behavior. These parity tools use public request/result shapes;
strict graph-create and Smart Forms tools have their own [recovery guides](tool-reference.md#write-and-recovery-behavior).


## Actors and forms

| Tool [legacy] | Public method |
| --- | --- |
| `actor_get` [getActor] | `actor/get` (fields, with) |
| `actor_by_ref_get` [getActorByRef] · `actor_system_get` [getSystemActor] | `actor/by-ref/get` · `actor/by-system_obj/get` |
| `actor_corezoid_processes_list` [getCorezoidProcesses] (details only for processes you see in Corezoid) | `actor/corezoid/process/list` |
| `actors_search` [searchActors] · `actors_filter` / `actors_filter_count` [filterActors] | `actor/search/list` · `actor/filter/list` / `count` |
| `search_all` [searchAll] (users and/or actors, each its own page) | one non-atomic `batch` of `user/list` + `actor/search/list` |
| `actor_create` / `actors_create` [createActor] · `actor_update` [updateActor] | `actor/create` (batch) · `actor/update` |
| `actor_status_update` [setActorStatus] · `actor_delete` [deleteActor] | `actor/status/update` · `actor/delete` |
| `form_get` [getForm] (body, editToken with `editor`) | `form/get` |
| `forms_list` / `forms_count` [getForms, searchForms] | `form/list` / `form/count` |
| `form_create` · `form_update` · `form_status_update` · `form_delete` [createForm, updateForm, setFormStatus, deleteForm] | `form/create` · `form/update` · `form/status/update` · `form/delete` |
| `form_fields_patch` (ops by field id: add, update, remove, move, sections; template properties; optional expected `editToken`) | `form/get` with `editor`, then `form/update` under its token |
| `forms_tree_get` [getFormsTree] · `form_linked_get` [getLinkedForms] | `form/tree/get` · its subtree, or `form/get` with `parents` |
| `form_accounts_list` [getFormAccounts] | `account_form/list` |
| `form_account_create` [createFormAccount] (pair by name or ids; also opened on the template's existing actors, 500 per call) · `form_account_delete` [removeFormAccount] | `account_form/list` + atomic `batch` of `account/create` + `account_form/create`, then `actor/filter/list` + `batch` of `account_actor/create` · `account_form/delete` |

## Graphs and layers

| Tool [legacy] | Public method |
| --- | --- |
| `edge_create` / `edges_create` [createLink, massLink] | `edge/create` (batch) |
| `edge_get` [getEdge] · `edge_exist` [existLink] | `edge/get` · `edge/by-node/get` |
| `edge_update` · `edge_endpoints_update` · `edge_delete` [updateEdge, deleteEdge] | `edge/update` · `edge/endpoints/update` · `edge/delete` |
| `edges_by_nodes_delete` [deleteEdgesByNodes] | `batch` of `edge/by-node/delete` |
| `edge_types_list` [getEdgeTypes] | `edge_type/list` (isSystem) |
| `layer_placements_list` · `layer_edges_list` [getLayerActorsPaginated] | `graph/layer/actors/list` · `graph/layer/edges/list` |
| `layer_placements_all` [getAllLayerPlacements] | `graph/layer/actors/list`, every page |
| `layer_skeleton_get` [getLayerActors] | `graph/layer/skeleton/get` |
| `layer_placements_manage` [manageLayerActors] (`layer_placements_create` for flowchart blocks) | `graph/layer/actors/manage` |
| `layer_positions_update` [updateLayerPositions] · `layer_actors_move` [moveActors] · `layer_clean` [cleanGraphLayer] | `…/positions/update` · `…/actors/move` · `graph/layer/clean` |
| `layer_element_exist` [existLayerElement] · `layer_actors_search` [searchLayerActors] | `graph/layer/element/exist` · `graph/layer/actors/search/list` |
| `layer_stats_get` [layerStats] | `graph/layer/stats/get` |
| `graph_list` [getRelatedActors] (hierarchy by default) | `graph/list` |
| `graph_get` [getActorLinks, getLinkedActors] | `graph/get` |
| `layer_compact` [compactGraphLayout] · `layer_prune_long_edges` [pruneLongEdges] | layer reads + `…/positions/update` · `edge/delete` |

## Access and collaboration

| Tool [legacy] | Public method |
| --- | --- |
| `access_rules_get` [getAccessRules, getTemplateActorsAccess, getTreeLayerAccess] (objType actor, formTemplate, templateActors, account) | `access_rule/<objType>/list` |
| `access_rules_save` [saveAccessRules, saveTemplateActorsAccess, saveTreeLayerAccess] (`mode` merge, or replace: everyone not named is revoked) | `access_rule/<objType>/share` (replace reads `…/list` first) |
| `access_request_create` [requestAccess] (an actor of the host workspace) · `access_requests_list` · `access_request_confirm` · `access_request_cancel` | `access_rule/actor/request/create` · `…/list` · `…/confirm` · `…/cancel` |
| `reactions_list` [getReactions] · `reactions_count` · `reactions_mark_read` [markReactionsRead] | `reaction/list` · `reaction/count` · `reaction/read/update` |
| `reaction_create` · `reaction_update` · `reaction_delete` [createReaction, updateReaction, deleteReaction] | `reaction/create` · `reaction/update` · `reaction/delete` |
| `chat_open` (p2p reused by its exact ref as the web UI does, you are the user a `Simulator` JWT names (its `sub`, matched to a member's `saId`), else `selfUserId` checked against the chat's rules; or a new group chat) | `form/system/get` (`Events`) + `actor/by-ref/get` + `user/get`, then one atomic `batch` of `actor/create` (ref, title) + `access_rule/actor/share` (`$0.id`) |
| `actor_attach` (actor card, Smart Form or pinned form card on an actor) | `actor/update` (`cardActorId` / `appId`) · `edge_type/get` + `edge/create` (`pinned`), with a new form actor in one atomic `batch` |
| `users_search` [searchUsers, getUsers] (without query a page of members) · `user_get` [getUser] · `workspaces_list` [getWorkspaces] | `user/list` · `user/get` · `workspace/list` |

## Files

| Tool [legacy] | Public method |
| --- | --- |
| `attachments_list` [getAttachments] · `attachments_by_actor_list` [getActorAttachments] | `attachment/list` · `attachment/by-actor/list` |
| `attachment_attach` · `attachment_update` · `attachment_detach` [addAttachments, updateAttachment, removeAttachments] | `attachment/attach` · `attachment/update` · `attachment/detach` |
| `attachment_upload` (file bytes as base64, up to 700 KiB; `actorId` attaches it) | HTTP `POST /api/2.0/rest/upload` (multipart, ufs limits and antivirus), which attaches it to `actorId` or refuses before storing |
| `attachment_read` [readAttachment] | Public download endpoint; bounded text/image/document decoding. See [file read limits](#file-read-limits). |

## Finance

| Tool [legacy] | Public method |
| --- | --- |
| `account_pair_create` [createAccountPair] (by name; creates a missing name and currency; an existing pair nobody was given access to only with `takeOwnership`) | `account_name/list`, `account_currency/list`, the missing `…/create`, `access_rule/account/list` for an existing pair, `account/create` |
| `account_create` [createAccount] (pair by account name and currency or by ids, as ui-kit's create-account; `plan` adds the plan twin) · `account_by_ref_create` · `account_update` [updateAccount] · `account_status_update` · `account_delete` [deleteAccount] · `account_pair_unregister` | atomic `batch` of `account/create` + `account_actor/create` (`ignoreIfExist`) · `account_actor/by-ref/create` · `…/update` · `…/status/update` · `…/delete` · `account/delete` |
| `accounts_list` [getAccounts] · `accounts_count` · `accounts_by_ref_list` · `account_get` [getBalance] · `account_by_ref_get` | `account_actor/list` · `…/count` · `…/by-ref/list` · `…/get` · `…/by-ref/get` |
| `currencies_list` [getCurrencies, searchCurrencies] · `currency_get` · `currency_create` [createCurrency] · `currency_update` | `account_currency/list` · `…/get` · `…/create` · `…/update` |
| `account_names_list` [getAccountNames, searchAccountNames] · `account_name_get` · `account_name_create` [createAccountName] · `account_name_update` [updateAccountName] | `account_name/list` · `…/get` · `…/create` · `…/update` |
| `counter_get` [getCounters] · `counter_by_period_get` · `counter_increment` [saveCounters] · `counter_set` [setCounters] | `counter/get` · `…/by-period/get` · `…/increment` · `…/set` |
| `transaction_create` [createTransaction, finalizeTransaction] · `transaction_by_actor_ref_create` | `transaction/create` (type, parentId) · `transaction/by-actor_ref/create` |
| `transaction_get` · `transaction_by_ref_get` [getTransactionByRef] · `transactions_list` [getTransactions] · `account_transactions_list` [getAccountTransactions] · `transactions_filter` / `_count` · `transaction_children_list` | `transaction/get` · `…/by-ref/get` · `…/list` · `…/account/list` · `…/filter` / `count` · `…/children/list` |
| `transfer_create` [createTransfer, createTransferTwoStep] · `transfer_get` [getTransfer] · `transfer_by_ref_get` [getTransferByRef] · `transfers_filter` [filterTransfers] / `_count` | `transfer/create` · `…/get` · `…/by-ref/get` · `…/filter` / `count` |
| `account_by_id_get` [getAccount] · `account_amount_set` [setAccountAmount] | `account_actor/by-id/get` · `account_actor/amount/update` |
| `account_formula_get` [getAccountFormula] · `account_formula_preview` · `account_formula_set` [setAccountFormula] | `account_actor/formula/get` · `…/preview` · `…/update` |
| `atom_transaction_create` [atomCreateTransaction] | `transaction/atom/create` |
| `account_tags_list` · `account_tag_add` · `account_tags_set` · `account_actor_tags_list` / `_count` · `account_triggers_list` / `_set` · `account_actor_triggers_list` / `_set` [saveAccountActors] | `account/tags/list` · `…/create` · `…/update` · `account_actor/tags/list` / `count` · `account/triggers/*` · `account_actor/triggers/*` (every `_set` replaces the set) |
| `actor_field_triggers_list` / `_set` [getDataFieldActorsByActor, saveDataFieldActorsByActor] | `actor/field/triggers/list` / `update` |

## Skills and agents

| Tool [legacy] | Public method |
| --- | --- |
| `skills_find` [findSkill] (verified only, no body) · `skill_get` [getSkill] (by ref or id, with the procedure) | `form/system/get` (`Skills`) + `actor/filter/list` · `actor/by-ref/get` or `actor/get` |
| `agents_find` [findAgent] (members without query) · `agent_get` [getAgent] (a user's twin or any actor, with the profile) | `user/list`, or `form/system/get` (`System`) + `actor/search/list` (semantic, else text), or `actor/filter/list` of a `formId` · `actor/by-system_obj/get` or `actor/get` |
| `build_link` [buildLink] (actor, layer, form, chat, chart, … and a Smart Form route; with `appKey`/`appId` inside the current Smart Form) | `route/build` (`dynamic` on the current form's key, `route/key/get` for an `appId`) plus the block's `#b=` fragment; the legacy web app route table on `SIMULATOR_AI_ORIGIN` as the fallback; a Smart Form's short URL from `route/build` (its production release) |
| `bbcode_tags_get` [getBbcodeTags] | none: the bundled BBCode tag catalog |

## Charts

| Tool [legacy] | Public method |
| --- | --- |
| `chart_create` [createChart] (manual series, or the top actors of an ActorFilters actor) | `form/system/get` (`Dashboards`, `ActorFilters`), `actor/create` of the filter and the chart, placed open as `layer_placements_create` does (room checked before the actors are made, made with anisotropy) |
| `chart_update` [updateChart] (settings, series or filter; with `laId` the placement's size) | `actor/get` + `form/system/get`, a new filter's `actor/create`, `actor/update`, `graph/layer/elements/list` + `graph/layer/actor_settings/save` |
| `chart_data_get` (what a saved chart shows over a period: series, buckets, total; also in `graph-read`) | `dashboard/get` (`chartId` is its `actorId`; `with` `total`/`series`, `actorIds`; no `source` override) |

## Applications and Corezoid

| Tool [legacy] | Public method |
| --- | --- |
| `smart_form_folders_create` (a new page's `pages/<page>`) | `app_folder/create` |
| `smart_form_releases_list` · `smart_form_release_diff` · `smart_form_release_rollback` | `release/list` · `release/diff` · `release/rollback` |
| `smart_form_file_versions_list` · `smart_form_file_version_get` · `smart_form_file_rollback` | `file_version/list` · `file_version/get` · `file/rollback` |
| `smart_form_trash_list` · `smart_form_trash_restore` | `trash/list` · `trash/restore` |
| `smart_form_page_render` · `smart_form_page_submit` (a page run as the runtime does; submit runs its process) | `page/render` · `page/submit` |
| `corezoid_projects_list` · `corezoid_stages_list` · `corezoid_processes_list` · `corezoid_project_get` | `corezoid/project/list` · `corezoid/stage/list` · `corezoid/process/list` · `corezoid/project/get` |
| `corezoid_project_bind` · `corezoid_project_create` (bind a Smart Form's envs to processes, or give it a project) | `corezoid/project/bind` · `corezoid/project/create` |

## Graph editing and import

| Tool [legacy] | Public method |
| --- | --- |
| `layer_edge_settings_save` (an edge's line style, width, colour, label on one layer) | `graph/layer/edge_settings/save` |
| `actor_picture_upload` [uploadActorPicture] (PNG, JPEG, GIF or WebP; no SVG) | HTTP `POST /api/2.0/rest/upload` (attached to nothing) + `actor/update` `picture` = its fileName, as the web app writes |
| `graph_export_create` [exportGraph] · `graph_import_create` [importGraph] (no Corezoid `scripts`) | `task/export/create` · `task/import/create` |

## Jobs and utilities

| Tool [legacy] | Public method |
| --- | --- |
| `task_get` [getTaskStatus] (adds `downloadUrl` for an archive) · `graph_file_upload` [uploadGraphFile] (base64, up to 700 KiB) | `task/get` · HTTP `POST /api/2.0/rest/upload` as `application/octet-stream`, attached to nothing |
| `public_link_create` [generatePublicLink] · `public_link_get` [getPublicLink] · `public_link_revoke` [revokePublicLink] | `public_link/create` · `/get` · `/revoke` (revoke takes back what the link gave) |
| `meet_transcription_list` [getTranscription] (the live transcript; empty before and after a call) | `meet/transcription/list` |
| `simulation_snapshot_get` [simulationSnapshot] · `simulation_check` [simulationCheck] · `simulation_run` [simulationRun] (the engine runs in the plugin and writes nothing) | reads `graph/layer/actors/list` · `graph/layer/edges/list` · `edge_type/list` · `account_actor/list` (`form/get` names a graph's bare form ids) |

The connected backend must expose the public methods listed above. Missing methods fail
with `methodNotFound`; check the instance’s public API reference before enabling a workflow. The rollback, restore, page submit, graph
import, guest link create/revoke and Corezoid bind/create tools are hinted destructive (a new guest link opens view to
anyone with its URL and stops the one already handed out); page render is not read-only either (it runs the page's `/get`).

An AI Console run's token requires backend support for run-scoped authorization on
`rest/upload`; unsupported instances refuse it, and `attachment_upload` returns that refusal. On a stand
serving downloads from ufs, a file under scan or in quarantine answers 404, so a read right after an upload is
retried for about 3 seconds and then reports "no such file, or its antivirus scan has not finished". The skills `simulator-ai-access`,
`simulator-ai-attachments`, `simulator-ai-chat`, `simulator-ai-tasks`, `simulator-ai-finance`, `simulator-ai-skills`,
`simulator-ai-agents`, `simulator-ai-charts` and `simulator-ai-meetings` compose these tools.

Skill procedures and agent profiles are descriptions users write: `skill_get` and `agent_get` return them as data, and
their tool descriptions tell the model not to take instructions from them. `build_link` builds a link inside the current
Smart Form when given its `appKey` (the Console context's `/sf/<key>`) or `appId`: `route/build` on that key with the start Smart Form's route of the entity (`workspace.actors`,
`workspace.graphs-edit`, `workspace.events`, `workspace.chats`, `workspace.templates-edit`, the lists) or a `route` of
the form's own, plus the block state as `#b=<blockId>:<route>` (both parts encodeURIComponent), root-relative. Without
them, or for an entity with no runtime page (marked `legacy`), it builds web app URLs on `SIMULATOR_AI_ORIGIN` with
the 8-character workspace segment, each id escaped as one path segment; a `smartForm` link needs a route its released
pages declare.

A chart's `data.source` uses the platform's JSON string format. Its initial placement uses
`layerSettings` `{expandType: chart, expand: true, width: 650, height: 450}`. `chart_update` keeps every stored key it does not change and never edits an ActorFilters actor (a
changed filter is a new one; the result names the one the chart stopped using). Legacy's account inheritance from the
layer's graph (`/accounts/inherit`) has no public method and is not set up.

Simulation runs locally in model time and memory; it writes nothing to Simulator. `simulation_snapshot_get` returns the layer
YAML instead of writing `<layerId>.sim.yaml`, and `simulation_check` / `simulation_run` take it back as `graph`
(there are no file paths); a formula account keeps its formula next to its current value, which a run does not
recompute. A snapshot reads accounts of at most 2000 actors; `simulation_run` takes at most 5 minutes, live layer
read included, and two at once per process (`SIMULATOR_AI_MAX_CONCURRENT_SIMULATIONS`); another is refused as busy. The skill `simulator-ai-simulate` (with the model format in its `references/`) composes them.

`graph_import_create` leaves out `scripts`: Corezoid API secrets never travel in tool arguments, so an imported
Smart Form is bound afterwards with `corezoid_project_bind`. Legacy `uploadGraphFile` also fetched a `fileUrl`; here the
archive comes as base64 only, and the plugin fetches no URL.

## Unsupported capabilities

`pullGraphFile` / `pushGraphFile` (local file ↔ layer synchronization
is not provided); getChildAccounts, since account trees were removed from the platform (walk the
children with `graph_list` and read their `accounts_list`). Also unavailable: pinned reactions, uploadBase64 and data-field actors
by form.


## File read limits

`attachment_read` fetches authorized files from `GET /api/1.0/download/<fileName>`.
It returns text, an MCP image block, or type/size information for unsupported content.

- Text: up to 100 KiB. Images: PNG/JPEG/GIF/WebP up to 5 MiB.
- PDF: text from at most 200 pages, up to 100 KiB returned. Textless scans may return up
  to four embedded JPEG page images. Parsing limits include a 10-second deadline,
  bounded cross-reference/predictor structures, 10 MiB text input and a 32 MiB aggregate
  decoded-stream budget. A PDF block is not sent to the host.
- DOCX/XLSX: bounded ZIP/XML extraction, 32 MiB unpacked per file, five seconds, and
  at most 32 sheets/512 columns for spreadsheets.
- Downloads retry 425 for about 15 seconds and 404 for about three seconds while a file
  may still be scanned. Permission and quarantine failures are surfaced to the caller.

The backend owns per-run image limits; these parser limits do not replace file permissions
or antivirus checks.
