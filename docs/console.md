# Console tools and workflows

[Tool guide](tool-reference.md) · [Generated profile availability](tool-catalog.md)

`console` provides a focused catalog for interactive tasks; `console-full` exposes the
complete Console catalog. Hosts may restrict it further using tool groups. Inspect the
actual `tools/list` schema before use; installed skills cannot enable unavailable tools.

Parameter descriptions are shortened for context size. List rows and graph nodes use
`id`, `title` and `formId` unless the tool accepts and receives another `fields` projection.

## Graph results and tool groups

Graph reads return compact results unless the call sends `full: true`: `layer_get` as in the table below;
`layer_placements_list` and `layer_actors_search` drop each row's `areaPicture` (a block's base64 SVG),
`cellPosition`, `createdAt`, `layerSettings.shape`/`textFrame` and the actor's copy of the row id, and round
positions; `layer_edges_list` keeps of each edge its type, ends, label and hole; `layer_skeleton_get` answers at
most 150 nodes and the edges between them with `total`, the `box` every node centre lies in and `truncated`; `layer_placements_all`
answers `count`, `summary {forms [{formId, count}], box}` and the first `limit` (150) rows by `cell`. Tool groups: Console entries in `tools/list` carry `_meta: {"simulator-ai/groups": [...]}`.
Hosts can select `core` plus the domains needed for a task. A tool can belong to multiple groups.
Groups include knowledge (the knowledge-base reads below, with `graph_get`, `actors_search` and `search_all`),
graph, graph_extra (holes, pencil drawings, opening a node, layer settings, the compact and prune
engines), smart_forms, forms, finance, finance_setup, people, files, meetings,
jobs, charts, simulation and corezoid. The plain `console` catalog is a curated subset,
not the union of all these groups. Use `tools/list` for the current inventory. The knowledge-base write tools:
`ontology_apply` is in graph (the plain catalog's budget has no room for it), saved views in forms (and charts),
the person's memory (`memory_get`, `memory_save`) in core. The knowledge-base reads are in knowledge, and
`ontology_get` and `entity_context_get` also in core; none is in the plain catalog (its 108 KiB budget has 1.2 KiB left, `ontology_get` alone is 2.3 KiB).

## Knowledge-base reads, long periods and AI Console dialogs

`ontology_get`, `entity_context_get`, `graph_traverse`, `graph_path_find`, `actor_backlinks_get`, `history_list` and
`graph_tree_get` call the public methods of the same name (`ontology/get`, `actor/context/get`, `graph/traverse`,
`graph/path/find`, `actor/backlinks/get`, `history/list`, `graph/tree/get`), pinned in `contracts/parity-openrpc.json`
from sim-api `feature/ai-knowledge-base` (see `parity-source.json`). They are console-only for now. `entity_context_get`
is compact unless `full: true`: data values and comment texts are cut at 300 characters, history values at 200, and
the neighbor groups of money edges (`transfersAmount_<nameId>_<currencyId>`) become `transfers [{direction, actorId,
title, amount, account, currency, transferIds}]`, the weights summed per counterparty, account and direction
(the type's last `_` segment is the currency, as sim-api reads it); a group with more edges than `perType` marks its
lines `partial` with their `edgeTypeId` (and `transfersHasMore`). `graph_get` adds `transfers [{from, to, amount,
edgeTypeId, transferId}]` for edges with a weight and a linked actor. A money edge's `linkedActorId` is the
transfer's Transfers actor (control-tasks makes one per transfer, ref = the transfer id): both tools read those
actors' `ref` in one `batch` of `actor/get` and name the transfer; one the caller may not read stays as
`transferActorIds` / `transferActorId`.

`history_list` maps `field` to the key history rows store: a template field named by its id or title (any case) is
sent as `data.<id>` (one `actor/get` + `form/get`; `fieldKey` in the answer), anything else as given; a `to` below
10^11 is read as Unix seconds (both bounds ×1000, `note`). `graph_traverse`, `graph_path_find` and `graph_get` add
`edgeTypes [{id, name, role}]` for the types of their edges from `ontology/get {fields: ["edgeTypes"]}`; a type that
list lacks and whose edge has a weight is `role: transactions`; a failed read leaves the answer as it is.

`layer_digest {layerId, full?}` reads the layer actor (`data.accounts`), `graph/layer/stats/get`, every node placement
and one non-atomic `batch` of `actor/context/get` (`actor`, `data`, `accounts`, `neighbors` over every edge type, 50 per
type and direction) per 100 actors (at most 500). Levels come from each actor's hierarchy parent on the layer.
`balances` are each actor's fact credit − debit of the pairs the layer shows (every non-zero pair without such
settings, or with `full`); `balanceSums` add them per template and account (`sumOfBalances`), never across templates.
`transfers` sum the outgoing money edges (`transfersAmount_*`, every account) between two actors of the layer per payer,
receiver and account, with `count`; the 40 largest in a compact answer. A card lists at most 200 neighbors and spends
them on the hierarchy and other links first, so an outgoing money group it cut (`hasMore`, under 50 listed) is read
again alone (`edgeTypeIds: [that type]`, one more non-atomic `batch` per 100, at most 300 such reads). A full answer
stays within 256 KiB: past it the deepest actors, then the smallest transfers, then the deepest balances are left out,
each with a note.

Money edges are refused by every edge write (`edge_create`, `edges_create`, `graph_build` by type name;
`edge_endpoints_update`, and `edge_update` with `linkedActorId` or `hole`, by the edge's type through `ontology/get`).
`ontology_apply` transactions without `side` follow the pair's posting side on other actors (`transaction/filter`, last
366 days, transfer legs left out); `account_create` answers it as `postingSide`.

`transfers_filter` and `transfers_filter_count` pass `dateBy` to sim-api (`transfer/filter` re-pinned for it, see
`contracts/parity-source.json` `ledgerDates`): `originalDate` bounds the window by the legs' business date (half-open, as
`createdAt`) and answers it as `originalDate`; with `formFields` or `kind: formFields` sim-api refuses it (-32602). Long
periods are split as for `createdAt`, every window carrying `dateBy`. `entity_context_get` adds `payments` from one `batch` of `transfer/filter`
(last 366 days, 10 each): `{actorIds: [actor]}` and `{query: <token>}` for up to two words of its title that hold a digit.
`graph_traverse` adds `countsByForm`.

**Long periods.** sim-api reads transactions and transfers in a window of at most 366 days. `transfers_filter`,
`transactions_filter`, `transactions_list`, `account_transactions_list` and the two `*_filter_count` tools split a
longer `from`/`to` into 366-day windows, newest first, at most 5 (the filters read `from <= t < to`, the lists
`from <= t <= to`: the windows meet without a gap or an overlap on either) (about five years back from `to`; `note` names what was
left unread). Counts are summed (`windows` says how many); pages are joined up to `limit` (default 50), keeping the first
window's other keys, and `next {from, to, cursor?}` is the call that reads on. `offset` is refused for such a period.

**AI Console dialogs** are Events actors whose `metaInfo.aiDialogParentId` names the layer or Smart Form they belong
to; the console's messages are comments under them. sim-api leaves them (and the comments under them) out of
`actor/search/list` (`actors_search`, `search_all`), `actor/context/get`, `actor/backlinks/get`, `graph/traverse` and
`graph/path/find` for an AI agent's token (a person's token keeps them); the tools pass `includeAiDialogs` through as a
contract param (re-projected from sim-api `feature/ai-knowledge-base` `a5d500b6`, `parity-source.json` `aiDialogs` and
`knowledgeBase`). What sim-api does not filter, the plugin does after the page is read: `layer_actors_search` adds
`metaInfo` to the projection and drops those rows (and the `metaInfo` it did not ask for), counting them in
`aiDialogsHidden` (a page can come back shorter than `limit`); `reactions_list` under a dialog answers
`{list: [], aiDialog: true, note}` after one `actor/get`. `includeAiDialogs: true` turns either off.

## Actor defaults and placeholders

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

`SIMULATOR_AI_PROFILE=console` can run per AI Console request under the initiator's own
`SIMULATOR_AI_AUTHORIZATION`. Hosts supply the current layer, actor or application context. Writes are plain public calls: no receipt keys and no contract
preflight, because sim-api deduplicates Console retries. Results follow each tool's declared projection; actor reads can include the full data document. The run's prompt carries sim-api's "Current context" (the layer, actor and Smart Form the user has
open): `layer_create` and `layer_placements_create` tell the agent to build on that layer unless the user names
another, never on a new one.

## Tool behavior

Expand an entry for its calls and result semantics. Profile membership is in the
[generated catalog](tool-catalog.md).

<details>
<summary>actor_get</summary>

**Public calls:** `actor/get`

**Result:** Backend actor shape, including `data`; `fields` narrows the result and `with` requests related data.

</details>

<details>
<summary>actor_create / actor_update</summary>

**Public calls:** `actor/create` / `actor/update`

**Result:** Defaults to id, title, description, formId, status, ref and color; caller-provided `fields` changes that projection.

</details>

<details>
<summary>actors_create / edges_create</summary>

**Public calls:** one atomic `batch` of `actor/create` / `edge/create` (edge types by name)

**Result:** actors / edges in input order; one failed item fails the call with its index and creates nothing.
`actors_create` keeps its actors out of the caller's recent list (`addToRecent: false`); with `operationId` they carry
refs `gb:<operationId>:<index>` and a repeated call answers the same actors (`actor/by-ref/get`) instead of new ones.

</details>

<details>
<summary>ontology_apply</summary>

**Public calls (dry run, reads only):** `form/list` (a template by title), `form/get` (its fields), `actor/get`
(existing actors and sources), `edge_type/get`, non-atomic `batch`es of `actor/by-ref/get` (the operation's own
actors), `actor/search/list` (same title) and `actor/filter/list` (`queryData {"$or": [{"aliases": {"$all":
[name]}}]}`), `account_name/list`, `account_currency/list`, `account_actor/get`, `account_form/list`, and — when the
patch posts — `ontology/get {fields: ["ledgerWrite"]}` for the ledger mode (a stand without it answers `unknown`);
with a layer and transfers, `actor/get` of the layer's `data.accounts`. **Apply:** with a layer, `graph/layer/actors/list` once; one atomic
`batch` of `actor/create` (ref `gb:<operationId>:<key>`), `graph/layer/actors/manage` (nodes, then edges by
`$<item>.nodes.<n>.laId`), `edge/create` (`forceDirection`) and `account_actor/create` (`ignoreIfExist`), at most 100
items; edges between actors already on the layer are drawn after it only when the layer does not show them. Then
`transaction/by-ref/get` + `transaction/atom/create`, and `transfer/by-ref/get` + `transfer/create` per transfer, refs
`<operationId>:<key>` (legs `…:f<i>` / `…:t<i>`; closing a hold names `parentRef` of the hold's legs). With a layer
and transfers (not on a legacy ledger), one `actor/update` of the layer adds the transfers' account pairs to its
`data.accounts` after the ones it has (as `layer_settings_update` writes them, the old `accountName`/`currency` keys
cleared, at most 32): the Graph Editor draws transfers only for the pairs a layer names.

**Result:** `done`, `actors [{key, status create|created|exists|existing, actorId, candidates}]`, `edges`, `accounts`,
`postings [{key, kind, status plan|posted|exists|skipped|failed, ref, id}]`, `ledger simapi|legacy|unknown`,
`layerAccounts` (the pairs added to the layer, or on a dry run to add),
`bulk`, `warnings`, `summary`. Problems (all of them) come back as one error before any write. The data key
`aliases` (a list of strings) holds an actor's other names; it is not a template field and not in full-text search.

</details>

<details>
<summary>saved_view_create / saved_views_find / saved_view_run, memory_get / memory_save</summary>

A saved view is an `ActorFilters` actor (`form/system/get`, `actor/create`): `data.filter` is the JSON string the filter
editor writes, with `formId`, `selectedForms`, `q` (`field=value&other>=10||…`, values without `&` or `|`), `status`
and the order; `saved_view_run` is `actor/filter/list` (and `actor/filter/count`) with `filterActorId`.
Personal memory is a section of the description of the person's twin (`actor/by-system_obj/get {objType: user}`; the
user is the one the Simulator JWT names, found via `user/list`; without viewUsers `selfUserId` finds the twin and its
access rule must carry the JWT's SuperAdmin id; a credential that names no user is refused). The memory block is the
line `[h3]AI memory[/h3]` and the memory lines right after it with no other line between them; the first line that is
not a memory line (a blank line, a paragraph, a next `[h3]`) ends it, and everything after it is the person's text,
never read as memory even when it looks like a memory line (a `- [x] done` checklist). Everything outside the block
(the agent profile `# Agent`, the person's text) is kept byte for byte. Its lines are `- [category] fact #tag` (category in
English, the fact in the person's language) and `- relation_type [actor=<id>]Title[/actor]` (`[[Title]]` and
`[[actor:<id>]]` from the model become the chip); a fact's brackets
are written as parentheses and its category as sim-api writes it (`preference`, `role`, `responsibility`, `works_with`,
`rule`, `note`); at most 8 KiB with the heading, as sim-api counts. `memory_save`
resolves every relation and edge type first, reads the twin again (`actor/get`) right before writing and merges its
change into what the section holds then (a section sim-api wrote after a dialog is kept), then writes the
`description` with `actor/update`; a relation is also an `edge/create` from the twin (atomic `batch`), and `forget`
deletes a forgotten relation's edge (`edge/by-node/delete`). A legacy `data.aiMemory` text is shown by `memory_get`
(`legacy: true`) and moved into the section, and the key cleared, by the next `memory_save`. Known risk: memory_save has no conditional write: a fact sim-api adds after the twin is read again and before the write can be lost (a window of milliseconds); sim-api uses the same block rules and vectors (`e_ai_memory`).

</details>

<details>
<summary>graph_build / graph_build_delete</summary>

**Public calls:** `graph/layer/actors/list` (the layer, once), then atomic `batch`es of up to 99 `actor/create` plus the
`graph/layer/actors/manage` that places them by result reference, and of up to 99 `edge/create` plus the `manage` that
draws them; `edge_type/get` per edge type; `actor/by-ref/get`, `actor/search/list` (Layers, the title) and `actor/create`
for `newLayer`: a new operation whose title a visible layer already has (spaces and case aside) builds nothing and answers
`done: false`, `existingLayerId` (the newest of them) and a `warning` saying to build on it, unless `allowDuplicateTitle`.
Delete: the layer read,
then `batch`es of `actor/delete`.

**Input:** nodes and edges are short tuples (`[key, title]`, `[from, to, name?]`) or objects; call-level `formId`,
`kind` and `edgeType` are defaults. **Result:** `done`, `layerId`, `nodes`/`edges` `{total, done, existing}`, `area`,
`ids` only for the keys asked for; `chunks` (runs of batches by outcome), `error` and `resume`
only when something is left to build. Never the input back. Up to 2000 nodes and 4000 edges a call. Every actor carries the ref
`gb:<operationId>:<key>` (unique within its template) and stays out of the recent list; a repeated call builds only
the keys that are not on the layer. Layouts: ranks top to bottom with every node's children in a block under it (rank order by
barycentre; more than six childless children fold into a near-square block; a row wider than `rowCells` is narrowed,
then wrapped inside the parent's block), whole ranks for flowchart blocks, a grid, or given cells; never over an
occupied cell (the call is refused before any write). Without `origin` a section takes the highest free place within
80 columns, so sections fill rows.

</details>

<details>
<summary>flowchart_blocks_list</summary>

**Public calls:** `form/flowchart-block/search`

**Result:** formId, title, blockId, `hasMore`; with `blockIds` also ready `data` and `layerSettings`

</details>

<details>
<summary>flowchart_layout</summary>

**Public calls:** `form/flowchart-block/search` (read-only)

**Result:** per node formId, data, grid position and layerSettings; `returnEdges`. Up to 100 blocks and 500 edges;
a rank wider than 60 cells wraps; `origin` is the layout's top-left cell.

</details>

<details>
<summary>layer_create</summary>

**Public calls:** `form/system/get` (`Layers`) + `actor/create`

**Result:** layer actor

</details>

<details>
<summary>edge_create</summary>

**Public calls:** `edge_type/get` (default `hierarchy`) + `edge/create`

**Result:** id, source, target, edgeTypeId, name

</details>

<details>
<summary>layer_placements_create</summary>

**Public calls:** `actor/get` (the layer's `data.anisotropy`) + `graph/layer/actors/list` (what each placement holds: a cell, an open box, a block's box) + `graph/layer/actors/manage` (create only); a flowchart node is border-snapped and gets the block's `areaPicture`. A taken area is refused, or with anisotropy made room for: atomic `batch` of `graph/layer/actors/offset` (a seam through the area) + manage. `expand` on the only node places it open (the `layer_node_expand` bag, its push first)

**Result:** nodes `{laId, actorId, position, cell}`, edges `{laId, edgeId, laIdSource, laIdTarget}`, `moved` (each node once), `opened {expandType, area}`, `covers`

</details>

<details>
<summary>layer_positions_update</summary>

**Public calls:** `graph/layer/actors/positions/update`

**Result:** updated count

</details>

<details>
<summary>layer_get</summary>

**Public calls:** `actor/get` (`data.anisotropy`) + `graph/layer/edges/list` + `graph/layer/actors/list` (+ `graph/layer/stats/get` when `hasMore`)

**Result:** compact nodes `{laId, actorId, title, formId, cell, blockId, hole, open}` and edges `{laId, edgeId, laIdSource, laIdTarget, name, hole}` (+ `source`/`target` when a node is off the page), 100 per kind, `anisotropy`, `hasMore` with `total` (best effort: 2 s, none on an error); `full` adds `position` and edge `source`/`target` (limit 200)

</details>

<details>
<summary>layer_node_expand (console-full)</summary>

**Public calls:** `graph/layer/elements/list` (the node) + the layer read above + `form/system/get` (`Scripts`, `Dashboards`) → atomic `batch` of `graph/layer/actor_settings/save` (`expand`, `expandType`, `flaskView`, `nestedFrame`, `expandPushed`, legacy `width`/`height`) + `graph/layer/actors/offset` (anisotropy; the close mirrors `expandPushed`), as simulator-ui-kit `expandOps` / `collapseOps`

**Result:** `{laId, actorId, expanded, changed, expandType, area, pushed, moved, covers}`

</details>

<details>
<summary>layer_viewport</summary>

**Public calls:** `graph/viewport/command` (a ruler `cell` sent as its index: `"C, 7"` → `{x: 2, y: 6}`)

**Result:** `commandId`, `openTabs` (0: the layer is not open, nothing moved)

</details>

<details>
<summary>hole_close</summary>

**Public calls:** `actor/hole/merge`, or `actor/get` (the hole) + atomic `actor/create` + `actor/hole/merge`

**Result:** `{holeActorId, actorId, created, changes}` (+ `closingProgress`, `conditions` with `forceMerge`)

</details>

<details>
<summary>edge_hole_close</summary>

**Public calls:** `edge/hole/merge`, or `edge/get` (the hole) + atomic `edge/create` + `edge/hole/merge`

**Result:** `{holeEdgeId, edgeId, created, changes}`

</details>

<details>
<summary>layer_draw (console-full)</summary>

**Public calls:** `internal/drawing` PNG (2 px per unit, as the editor's Pencil confirm) → `rest/upload` → atomic `actor/create` (layer's `defForm`, `picture`, `pictureObject`) + `graph/layer/actors/manage` (`layerSettings.image`) + `attachment/attach`

**Result:** `{actorId, laId, picture, position, cell, width, height}`

</details>

<details>
<summary>layer_settings_update (console-full)</summary>

**Public calls:** names → `account_name/list` / `account_currency/list`; `form/get` for the `defForm` title; one `actor/update` of the layer's `data` (`maxVisibleChildren`: 0 off or 1–200, the layer's fan limit for every viewer)

**Result:** `{layerId, keys}`

</details>

<details>
<summary>layer_collapse_get (console-full, graph-read)</summary>

**Public calls:** `graph/layer/collapse/state/get` (the caller's own saved folds and fans, and the layer's `maxVisibleChildren`); with any fold or a fan limit `graph/layer/extent/get` (the zone of the fans, one cell of margin) + `graph/layer/collapse/resolve`; `graph/layer/elements/list` (titles of the folds and fan parents, at most 500). With `laId`: `graph/layer/collapse/children` for a folded fan (paged, `q` by title, the children the user showed excluded), else one page of the fold's hidden placements from the resolve + their titles

**Result:** `{layerId, owner ("you" | "apiUser"), maxVisibleChildren, folds[{laId, title, conditional, condition, summary, visible, gone, hiddenCount, childCount, matchedChildCount, sharedVisibleCount}], fans[{laId, title, childCount, hiddenCount, opened}], fanOpen, fanShown, hiddenTotal, truncated, under{laId, kind (fold | fan), items[{laId, actorId, title, accessDenied}], hasMore}}`

</details>

<details>
<summary>layer_collapse_count (console-full, graph-read)</summary>

**Public calls:** `graph/layer/collapse/count`

**Result:** `{count, childCount, capped}`

</details>

<details>
<summary>layer_collapse_update (console-full)</summary>

**Public calls:** `form/get` (the default `summary` of a fold by condition: "<template>: <q>", balance bounds) + `graph/layer/collapse/state/update` (writes the caller's own `graphCollapse_` / `graphCollapseConditions_` / `graphFan_` user settings and sends `graphCollapseChanged` to their tabs). Refused before any call with an API key (`ApiKey` credential): the state would be the API user's, which no person sees

**Result:** `{layerId, roots, conditional, fanOpen, fanShown, maxVisibleChildren, changed, openTabs}`

</details>

<details>
<summary>smart_form_create</summary>

**Public calls:** `application/create` (`cduVersion: 2`)

**Result:** appId, ref, developEnvId, productionEnvId

</details>

<details>
<summary>smart_form_components</summary>

**Public calls:** `cdu/catalog/get`

**Result:** with `brief` every type's kind, category and label; with `types` or `kind` one line per prop, event, state key and api method (TypeScript-like, `$ref`s resolved, enums inlined, inherited HTML/aria/on* attributes left out; Stack+Form 60 KB of schema → 4.7 KB); `schema: true` the validator's JSON Schemas

</details>

<details>
<summary>smart_form_reference_grep / smart_form_reference_read</summary>

**Public calls:** `cdu/catalog/get` (all types, once per 10 minutes per process) for `components`; `protocol` and `examples` are embedded

**Result:** plain text, grep-like: case-insensitive RE2 hits `doc:line: text` with context lines and the section heading (≈ 6 000 chars at most); reads by line or by section `doc/<section>` (8 000 chars a page); see "CDU reference"

</details>

<details>
<summary>smart_form_data_form_create</summary>

**Public calls:** `form/create` (data template)

**Result:** formId and a ready page whose button submits via `form/submit` into it

</details>

<details>
<summary>smart_form_files_get</summary>

**Public calls:** `application/env/list` + `application/struct` (develop, never with sources); + `app_file/get` per file read (at most 20)

**Result:** folders/files by path with `sourceHash`, `developEnvId`, `productionEnvId`. Sources on request (`paths`: the named files and
folders; `withSource`: every file) within 40,000 characters per answer: files past that come without `source` and `note` says how to read
them; one file too long alone is cut with `next` (read on with `offset`). `outline: N` answers pages and definitions as one line per node
(`Type#id props on:events`, `+n` nodes below level N); with one file in `paths`, `nodeId` (a node id or a JSON Pointer) answers that part
as `node`, or its outline. `missing`: paths that name nothing

</details>

<details>
<summary>smart_form_page_patch</summary>

**Public calls:** `application/env/list` + `application/struct` (by `path`) + `app_file/get` + `app_file/update` (`ifMatchHash` = the hash read)

**Result:** RFC 6902 ops (add, replace, remove, move, copy, test; with `nodeId` relative to that tree node) on one develop JSON file (a page config or a definition `definitions/<name>`; a `$ref` instance takes no type, props, children or actions), all or none: `{appId, fileId, path, sourceHash, applied}`, never the source

</details>

<details>
<summary>smart_form_files_update / smart_form_files_create</summary>

**Public calls:** `app_file/update` / `app_file/create`

**Result:** id, folderId, title, sourceHash. An update with `source` requires `ifMatchHash` (the `sourceHash` read): a file changed since
is not written, and the error names its hash now and the node ids the sent source lacks (never the stored source)

</details>

<details>
<summary>smart_form_folders_create</summary>

**Public calls:** `app_folder/create`

**Result:** folders with ids (a page is `pages/<page>`)

</details>

<details>
<summary>smart_form_release</summary>

**Public calls:** `application/env/list` + `application/struct` + `app_file/get` per page config (the pending check) + `release/create` (develop → production)

**Result:** `{appId, releaseId, releaseNumber, status, ready, marker}`

</details>

<details>
<summary>smart_form_release_get</summary>

**Public calls:** `application/env/list` + `release/get` (without `releaseId`: `route/key/get` names the active one)

**Result:** same release result

</details>

## Flowchart workflow

`flowchart_blocks_list` (kinds) → `flowchart_layout(nodes, edges)` → `actor_create(formId, title,
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
to a multiple of 50 (half a cell off for even-sized blocks).

## Smart Form workflow

A submitted form saves values as an actor of its own data template.
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
page configs (one `app_file/get` each, so a large form stays under the response limit) and refuses to release one still in that state.
`smart_form_page_render` (in the plain `console` catalog) answers `{ok, pageId, nodes, bindings, vmKeys, locales, bytes, warnings?}`
unless `full` is sent: a written page is verified by rendering it, not by reading it back. Then
`smart_form_files_get` → `smart_form_files_update` of `pages/index/config` → `smart_form_release`; a later change to a page is
`smart_form_files_get {paths}` → `smart_form_reference_grep` / `smart_form_reference_read` (or
`smart_form_components {types}`) → `smart_form_page_patch` by `nodeId`. Its
`ready: true` (status `active`, snapshot kept, production env) comes with `marker: "appId: <appId>"`. The
agent ends its final answer with that line, and sim-api reads it from the Markdown (the Hermes run stream carries
no tool results) to put `appId` into the reply reaction with `appSettings.autorun`.

## CDU reference

Use `smart_form_reference_grep` to find relevant reference sections, then
`smart_form_reference_read` to read them. `smart_form_components` describes component
schemas and examples for the connected backend. The bundled protocol and example documents
cover page structure, expressions, forms, layouts and routes; they record their source
revisions so a maintainer can reproduce them.

Start with the [CDU v2 example](../plugins/simulator-ai/skills/simulator-ai-smart-forms/references/cdu-v2.md),
then check the actual component and page contracts before saving a change. A bundled example
is guidance, not proof that every backend version supports the same components.
