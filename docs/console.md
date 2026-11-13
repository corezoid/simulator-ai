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
Groups include graph, graph_extra (holes, pencil drawings, opening a node, layer settings, the compact and prune
engines), smart_forms, forms, finance, finance_setup, people, files, meetings,
jobs, charts, simulation and corezoid. The plain `console` catalog is a curated subset,
not the union of all these groups. Use `tools/list` for the current inventory.

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
<summary>graph_build / graph_build_delete</summary>

**Public calls:** `graph/layer/actors/list` (the layer, once), then atomic `batch`es of up to 99 `actor/create` plus the
`graph/layer/actors/manage` that places them by result reference, and of up to 99 `edge/create` plus the `manage` that
draws them; `edge_type/get` per edge type; `actor/by-ref/get` and `actor/create` for `newLayer`. Delete: the layer read,
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
