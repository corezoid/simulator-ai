# Changelog

## [0.1.0]

### Changed
- record verified release quality evidence without review gates
- use a repository link in copied PR descriptions

## [Unreleased]

- `chart_data_get` says what an answer without series means: sim-api's public `dashboard/get` now reads a chart
  nobody configured (a `Dashboards` actor without a saved source) as an empty chart instead of refusing it with
  `invalidParams`. The tool description and the charts skill state that an answer without series
  is a chart with none to show — never configured, without accounts, with its series hidden or gone, or `actorIds`
  that matched none — and not a period without movement; the skill no longer lists a missing source among the
  `invalidParams` causes. Params and result schema are unchanged, so the pinned `dashboard/get` contract stays as is.

- The person's memory is a graph (sim-api `service_ai_memory`): `memory_save` sends relations by actor id or by title
  and `forgetAll`; sim-api draws each relation as an edge from the person's actor (`hierarchy` unless another type
  fits) marked `aiSource: memory`, resolves a `[[Title]]` to the one actor of that title (else a text fact), keeps the
  relation lines in step with the graph and deletes only memory edges. The plugin no longer draws or deletes memory
  edges itself. `ontology_apply` refuses a person's actor as a hierarchy parent; `layer_digest` levels ignore memory
  edges. `aiSource` (`console` for an AI Console run's edges) is named in the ontology skill. `actor/memory/save`,
  `edge/create` and `graph/layer/elements/list` re-pinned.

- `memory_save` no longer writes the person's description itself: it sends the facts, relations and `forget` to
  sim-api's new public `actor/memory/save`, which merges them into the `[h3]AI memory[/h3]` block as it is at the
  write, under the same locks as the automatic memory after a dialog. The known risk — a fact sim-api remembered
  between the save's read and its `actor/update` was lost — is closed; the rest of the description stays byte for
  byte. The block's text rules move to `internal/aimemory` (shared with the test fake, the same vectors as sim-api's
  `e_ai_memory`). One save takes at most 50 facts of up to 1000 characters; the relations' edges are drawn and deleted
  after the write (a refused save touches none); a legacy `data.aiMemory` moves in its own save and its key keeps only
  the relations no actor answers to. `actor/memory/save` is pinned
  in `contracts/parity-openrpc.json`.

- sim-api review follow-ups of the knowledge base (sim-api develop, see its `docs/api/audience.md`): `ontology/get` no
  longer lists a template with part of its accounts — a page ends before such a template, and one with over 2000
  accounts alone on its page says `accountsHasMore` (the knowledge skill points to `form_accounts_list` for the rest);
  `attachment/search` leaves AI Console dialog files out unless `includeAiDialogs` (no tool here calls it yet). The
  knowledge-base record of `contracts/parity-source.json` is re-pinned to that sim-api revision; the projected params
  and summaries of its methods are unchanged (the projection carries no results).

- `layer_digest` answers money a person can repeat (eval r11251105 layer-01, layer-07): `balances` lists what each
  actor holds now (credit − debit, as `entity_context_get`), `balanceSums` add them per template and account as
  `sumOfBalances` — the old `totals` added every actor's balance into one figure (2 000 000 over company, departments
  and counterparties), which read as the departments' money. `transfers` cover every money account (the cards are read
  over every edge type, no longer 19 types; `ontology/get` is not read), per payer → receiver × account with `count`.
  Notes are written for the person, without edge-type details.

- Skill descriptions cut to at most 200 characters each (4 496 characters across the 24 skills; the first 22 were
  11 586): what the skill does, its first ~57 characters naming the task (the Hermes index shows that much), named
  as Simulator's, plus a few EN/UK/RU trigger phrases and the words that tell look-alike skills apart (chat / meetings / tasks, agents / tasks, the Smart Form skills). Hosts load every description into every
  session that has the plugin, Simulator task or not, and once Claude Code's skill listing overflows it shows skills
  by name only. Skill bodies are unchanged. A `cmd/client-smoke` test fails `make test` when a description is over 200
  characters or a skill's `name` differs from its directory; it parses the frontmatter as YAML, so a folded or
  multi-line description is measured whole.
- `ontology_apply` `source: reaction:<id>` writes an actor chip of the comment (`[actor=<id>]comment: <its text
  without markup, up to 80 characters>[/actor]`) instead of `[quote=…]`: `[quote]` is not in the actor-description
  tag set (bbcode-tags.json; ui-kit's description editor shows it as literal text), the actor chip is, and backlinks
  find it.

- Personal memory moves into the description of the person's twin, so they see it in the UI (owner's decision):
  `memory_save` / `memory_get` read and write the block of the line `[h3]AI memory[/h3]` and the memory lines right
  after it: the first other line (a blank line, a paragraph, a next `[h3]`) ends it, and what follows is the person's
  text, never read as memory even when it looks like it (a `- [x] done` checklist); the rest of the description (the
  agent profile `# Agent`, the person's text) is kept byte for byte. Lines are `- [category] fact #tag` (category in English, the fact in the person's language, English
  when unclear or mixed) and `- relation_type [actor=<id>]Title[/actor]` (the model's `[[Title]]` / `[[actor:<id>]]`
  become the chip; the edge from the twin stays). At most 8 KiB with the heading; a fact is
  text (brackets as parentheses), its category as sim-api writes it. A save
  reads the twin again right before writing and merges into the section as it is then, so a section sim-api writes
  after a dialog is not lost. A legacy `data.aiMemory` is shown by `memory_get` (`legacy`) and moved into the section,
  and the key cleared, by the next `memory_save`. The ontology skill and the guide say the fact's language.
- `graph_build {newLayer}` no longer makes a second layer of the same title when a build is repeated in a new turn
  with a new `operationId` (eval build-01): before creating the layer it searches the layers the caller sees
  (`actor/search/list` on the Layers template) for the title, spaces and case aside; one found answers `done: false`,
  `existingLayerId` and a warning to build on it (`layerId` with `existing`/`use`) and nothing is written.
  `allowDuplicateTitle: true` makes the second layer on purpose. The same operation still finds its own layer by ref.
  The graph skill says a repeat builds on the existing layer.

- `actorLocked` passes through: sim-api answers it while another write holds the actor, and after sim-api !598
  (CE-16160) any call that writes an actor can — `actor/update`, status and read-only changes, Smart Form edits, the
  Corezoid bind, `transfer/form-fields/create` and `reaction/create` / `reaction/delete` of freeze, sign, done and
  reject among them. The refused write changes nothing in Simulator. The client used to report it as
  `remote_error`; it now keeps the reason, and the main skill says to retry a few times, reusing anything a
  multi-step tool already created, and then to tell the user the actor is busy.
- `timeout` passes through as well: `graph_build` reads a server timeout as an unknown outcome ("repeat the
  call"), but the client turned it into `remote_error`, so it was reported as a refusal to fix.

- Eval fixes (transfers-14, transfers-07, multihop-14):
  - `transfer_get`, `transfers_filter` and `transfer_by_ref_get` are in the `knowledge` group too.
    `entity_context_get` answers `payments`: the transfers of the last 366 days the actor is a party to or whose
    comment holds a number token of its title (`№22`), from one batch of `transfer/filter`; payments between actors
    nobody may see are only counted (`paymentsHidden`). `payments` is a section of `fields`, never sent to sim-api.
  - `transfers_filter` / `transfers_filter_count {dateBy: "originalDate"}` filter by the legs' business date on the
    server: sim-api `transfer/filter` and `transfer/filter/count` take `dateBy` (feature/ai-knowledge-base 7a91aefa:
    042766e1, 0ed345c8, review de912af4). The two methods are re-pinned, deliberately and only they
    (`parity-source.json` `ledgerDates`); the plugin's own walk over `transaction/filter` is removed. `dateBy:
    "originalDate"` with `formFields` or `kind: formFields` is refused by sim-api (-32602): a form-field transfer has
    no business date. Long periods keep the 366-day windows, each with `dateBy`.
  - `graph_traverse` answers `countsByForm` (reached actors per template, start and denied ones left out); the
    knowledge skill reads "how many" from it.

- Eval fixes (write-10, write-18, layer-07):
  - Money edges (`transfersAmount_*`, `transfersFormField_*`) are refused by every edge write, not only
    `ontology_apply`: `edge_create`, `edges_create` (before any read) and `graph_build` (in its validation) by the
    type name; `edge_endpoints_update` and `edge_update` with `linkedActorId`/`hole` read the edge's type by id from
    `ontology/get` (per-account types included) and refuse to re-point or relink one. Labels and looks stay editable.
  - Posting side: `ontology_apply` gives a transaction without `side` the side other actors post the pair on (one
    `transaction/filter` over the last 366 days, transfer legs left out, each actor counted once), else credit; the
    dry run's detail says "convention: credit (N actors, M on the other side)" and a side against it is a warning.
    `account_create` answers `postingSide`; the finance skill puts the convention before "spending on debit".
  - New `layer_digest` (groups `knowledge` and `graph`): a layer in one call — stats, actors per template and per
    hierarchy level, each actor's filled fields and fact balances (credit − debit; the pairs the layer shows, else
    every non-zero one), totals per account, transfers between its actors; one batch of `actor/context/get` per 100
    actors. Compact (80 actors, 4 fields, values cut) unless `full`. Skills route "summarise this layer" to it.

- Eval fixes (asof-07, multihop-08): `history_list` sends a template field named by its title («Сума») or bare id
  (`amount`) as the stored key `data.<id>` (`fieldKey` says so; sim-api's `history/list` matches a bare id itself since `a5d500b6`, a title or a column in another case never) and
  reads a window given in Unix seconds as seconds (`note`); its `from`/`to`/`field` descriptions say milliseconds and
  how to ask for the whole history. `graph_traverse`, `graph_path_find` and `graph_get` answer `edgeTypes [{id, name,
  role}]` for the types their edges hold (one `ontology/get {fields: ["edgeTypes"]}`; a per-account money type by its
  weight), and `ontology_get` says it is the list of edge types; the knowledge skill explains Gantt dependencies.

- Knowledge-base reads (AI Console brief `ai-knowledge-base-architecture-2026-11-24`, phases 1–2; eval baseline 14/19):
  - New tools on public sim-api methods: `ontology_get` (`ontology/get`), `entity_context_get` (`actor/context/get`),
    `graph_traverse`, `graph_path_find`, `actor_backlinks_get`, `history_list` (`history/list`, now public) and
    `graph_tree_get` (`graph/tree/get`). Console only for now. `contracts/parity-openrpc.json` adds these eight methods
    (with `ontology/count`), deliberately and only these, from sim-api `feature/ai-knowledge-base` `b4e60674`
    (`parity-source.json` records it under `knowledgeBase`; the other 156 keep the develop `87dd69ce` pin), 164 methods.
  - New tool group `knowledge` (the name sim-api's router uses): the seven reads plus `graph_get`, `actors_search` and
    `search_all`; `core` also takes `ontology_get` and `entity_context_get`. The plain `console` catalog stays as it
    was (no room under its 108 KiB budget).
  - "Who paid whom how much": `entity_context_get` sums the money edges (`transfersAmount_*`) per counterparty into
    `transfers` (direction, amount, account, currency, transferIds — the edges' Transfers actors resolved to transfer
    ids; `partial` when the group had more edges than `perType`; `full: true` keeps the edges), and `graph_get`
    lists its money edges as `transfers`. The finance skill starts there and reads details with `transfer_get`.
  - Long periods: `transfers_filter`, `transactions_filter`, `transactions_list`, `account_transactions_list` and the
    two `*_filter_count` tools split a period over sim-api's 366-day window into windows (at most 5, newest first):
    counts summed, pages joined up to `limit` with `next` to read on; `from`/`to` say so.
  - AI Console dialogs (Events with `metaInfo.aiDialogParentId`) and their comments are left out unless
    `includeAiDialogs: true`: by sim-api itself for an AI agent's token in `actors_search`, `search_all`,
    `entity_context_get`, `actor_backlinks_get`, `graph_traverse` and `graph_path_find` (the switch is passed through;
    these methods and `history/list`'s bare-id `field` re-pinned from sim-api `a5d500b6`), by the plugin in
    `layer_actors_search` (`aiDialogsHidden` counts them) and `reactions_list` under a dialog.
  - `ontology_apply` takes the ledger mode from `ontology/get`'s `ledgerWrite` instead of a legless `transfer/create`
    probe, and with a layer and transfers adds the transfers' account pairs to the layer's `data.accounts`
    (`layerAccounts`), so the Graph Editor draws them.
  - Skills: new `simulator-ai-knowledge` (mentions, an actor's card, links, history and a value on a date); the guide
    says to read `ontology_get` first and `entity_context_get` for one actor, and routes the knowledge tasks; the
    first words of the guide, finance, ontology and graph descriptions name their tasks (Hermes shows ~57 characters).

- Knowledge-base write (AI Console brief `ai-knowledge-base-architecture-2026-11-24`, W1–W4, W7):
  - `ontology_apply` writes an "ontology patch" by the model's own keys: actors (new by `form`/`formId`, or
    `existing`), `parent` (a hierarchy edge), edges (`hierarchy`, `gantt` = the Gantt dependency type
    `directedInteraction`, or any workspace type), accounts (name × currency), transactions (one account) and
    transfers (an exchange; `authorized` holds, `hold` closes). `dryRun` checks the patch strictly — the template
    exists, `data` keys are its field ids (an unknown key is refused with the template's fields), choice values are
    its options (stored as `[{value, title}]`), the account name and currency exist, every posting's account will
    exist, transfers are balanced in one currency, `existing` is visible — and answers a diff (create / exists /
    existing, edges, accounts, postings) with the ledger mode and the bulk warning (> 50 objects). Recognition finds
    the operation's own actors (ref `gb:<operationId>:<key>`) and candidates by title and `data.aliases`; the apply
    refuses a candidate unless the patch names it `existing` or sets `new`. Phase a is one atomic `batch` with result
    references (actors, placements on an optional layer, edges and their placements, `account_actor/create`);
    phase b posts `transaction/atom/create` and `transfer/create` with ref `<operationId>:<key>`, looked up first, so
    a repeated patch writes nothing twice. A `transfersAmount_*` edge is refused (a transfer draws it). A workspace
    still on the legacy ledger (`ledgerWrongAPI`) keeps the structure and skips the postings with a sentence for the
    person; the dry run says so beforehand. `source` adds a chip (`[file=…]`, `[actor=…]`, `[quote=…]`) to the
    description.
  - Saved views on `ActorFilters`: `saved_view_create` (conditions written in the filter editor's `q`),
    `saved_views_find`, `saved_view_run` (`filterActorId`, with `count`).
  - Personal memory on the person's twin actor: `memory_save` / `memory_get` keep Basic Memory's micro-syntax in
    `data.aiMemory`; a relation `- type [[Actor]]` is also an edge from the twin, and `forget` deletes that edge.
    Whose memory is the credential's to say (the run's JWT), never the model's `selfUserId`.
  - `transfer_form_fields_create` (`transfer/form-fields/create`): `contracts/parity-openrpc.json` projects one more
    method from the same pinned document (sim-api `87dd69ce`, sha256 unchanged; develop `056c970f` serves the same
    document), 156 methods.
  - Groups: `ontology_apply` in `graph` (the plain `console` catalog has no room under its 108 KiB budget);
    saved views in `forms` (and `saved_views_find`/`saved_view_create` in `charts`); memory in `core`;
    `transfer_form_fields_create` in `forms` and `finance_setup`. `ledgerWrongAPI` is a known error reason.
  - Skills: new `simulator-ai-ontology`; the guide routes to it; `simulator-ai-finance` explains transactions vs
    transfers and the legacy ledger.

- Form-field transfers (sim-api !587 / CE-16150, `formFields` from !547 / CE-16152): `transfers_filter` /
  `transfers_filter_count` take `kind: "formFields"` (only form-field and owner transfers, any field) and
  `formFields` (narrow to `(formId, fieldId)` pairs); neither combines with `accounts` / `accountType`. `actorIds`
  now also matches an owner change that moved the actor (`value[].value`), with `kind` and without. The finance
  skill says how to search transfers. `contracts/parity-openrpc.json` is re-pinned as a whole at sim-api develop
  `87dd69ce`: besides the two transfer methods it brings `ownerId` / `extraData` on `actor/create` /
  `actor/update` (pinned only: `actor_create` / `actor_update` keep their params), the Gantt placement keys on
  `graph/layer/actors/manage` items, drops the `ds` type from `reaction/create`, and refreshes descriptions of
  seven more methods.

- Template avatar on actor reads (sim-api !585 / CE-16164): with `with: ["avatar"]` sim-api adds `formColor` and
  `formPictureObject` — the account template's colour and picture a graph draws an actor with when it has none of its
  own. The pinned contracts already carry both (re-pinned at sim-api `b2d4aeb9`); `actor_get` and the actors skill now
  say what they mean (`actor_by_ref_get` / `actor_system_get` get no avatar from sim-api, and their
  descriptions now say so), and a test holds that placement compaction keeps them.

- Gantt placement keys (CE-16071): sim-api types four placement `layerSettings` keys of the Graph Editor's Gantt
  view — `baseline {startDate, endDate}`, `neighbors`, `ganttOrder`, `ganttPhase` — on public
  `graph/layer/actor_settings/save` and `graph/layer/actors/manage` creates (`null` clears one; a broken value is
  `invalidParams` naming the key). The graph skill explains them; `layer_placements_list` / `layer_actors_search`
  already return them (a test now holds that). No tool edits them on an existing placement; `layer_placements_manage`
  passes them through on a create. `contracts/public-openrpc.json` and `contracts/graph-elements-openrpc.json` are
  re-pinned at sim-api `b2d4aeb9` (also brings additive actor fields on `actor/get` / `graph/layer/actors/list`
  and `form/get`, and the elements `timeZoneOffset` -840..720 bounds, a param the tool does not send);
  `cmd/gen-client` decodes a nullable type list (`["number","null"]`) and refuses it on a projected field.
  `parity-openrpc.json` keeps its pin until its next whole-projection re-pin (a re-pin rewrites many tool input
  schemas, e.g. `actor/create` gains `ownerId`/`extraData`); `graph-create-openrpc.json` keeps its pin because its
  `*/operation/get` receipt methods are not on sim-api develop.

- The layer's default collapse (CE-16245): `layer_collapse_get` / `layer_collapse_update` read and change what the
  user sees — the layer author's default (`data.collapseDefault`) under the user's own changes; expanding a default
  fold expands it for the user only. The collapse contract is re-pinned (`state/get|update` answer `default` and
  `expanded`; `graph/layer/collapse/children` lists a collapsed actor's hidden children with `roots`/`conditional`).

- Collapsed branches for AI (CE-16204): `layer_collapse_get` reads the user's own Graph Editor collapse of a layer
  (folds whole or by condition with their hidden counts and titles, wide fans over the layer's limit, one page of
  what hides under a fold or a fan), `layer_collapse_count` previews a fold by condition, and `layer_collapse_update`
  (console only) folds and unfolds for the user through public `graph/layer/collapse/state/update` — their open
  editor follows at once. The write is refused with an API key (it would change the API user's view, which nobody
  sees). Reads are also in `graph-read`; all three are in the `graph_extra` group, not the plain console catalog.
  `layer_settings_update` takes `maxVisibleChildren` (0 off or 1–200: the layer's fan limit for every viewer).
  The new public methods are pinned in `contracts/collapse-openrpc.json` (`cmd/gen-graph-contract -profile collapse`).

- `reactions_list` and the reactions skill say that `with: ["parentActor"]` leaves the parent out when it is a
  hidden reaction the caller cannot read (no `execute` on the actor) — sim-api now omits it for a still-visible
  reply to such a note (CE-16234 follow-up); `parentId` keeps naming it. Description-only; tool schemas unchanged.

- `reaction_create` / `reaction_update` and the reactions skill say how sim-api now treats hidden reactions
  (CE-16234): a reply to a hidden reaction is always hidden and only a user who reads that reaction may answer it
  (others get "Parent actor not found"); `reaction_update {hidden: false}` on such a reply fails with `notAllowed`;
  a hidden reaction never goes into a live meeting transcript, and hiding a transcribed one takes its line out.
  Tool input schemas are unchanged; the pinned parity projection (`contracts/parity-openrpc.json`) keeps its older
  `hidden` field descriptions until its next whole-projection re-pin.

- `chart_data_get` reads what a chart (dashboard actor) shows over a period through public `dashboard/get`
  (CE-15981): one series per account, `data[]` buckets for line/stackedBar charts or with `with: ["series"]` for any
  chart type, and with `with: ["total"]` the sum of every series as a whole and per bucket; `actorIds` keeps only
  those actors' series. The period is the caller's (a chart's saved range is not applied); an answer over 256 KiB is
  refused with a hint to narrow it. Read-only, in `graph-read` (workspace mode) and `console-full` (`charts` group,
  not the `console` core). Pinned as its own projection (`contracts/dashboard-openrpc.json`, sim-api `d13fc772`: `timezoneOffset` -840..720, JS sign) by
  `cmd/gen-graph-contract -profile dashboard`. The CDU v2 reference is regenerated from sim-api `d13fc772` and
  simulator-ui-kit `edefb4b5`.

- Sign in as yourself, without an API key: with no host credential the server starts in login mode, and `login`
  opens the SuperAdmin page in the browser (OAuth2 PKCE, the legacy plugin's public client), `workspaces_list` and
  `workspace_set` choose the workspace, `whoami` and `logout` complete the set. The session and the chosen workspace
  are kept per API origin in `~/.config/simulator-ai/credentials.json` (0600) and renewed with a refresh token when
  SuperAdmin issues one. `SIMULATOR_AI_API_KEY` / `SIMULATOR_AI_AUTHORIZATION` / `SIMULATOR_AI_DELEGATED_TOKEN` and
  `SIMULATOR_AI_WORKSPACE` keep precedence, and a host Authorization process (AI Console) gets no new tools.
  In login mode `SIMULATOR_AI_ORIGIN` defaults to `https://mw.simulator.company` (host credentials still need it, and actor-bound profiles still need a host credential); new optional `SIMULATOR_AI_ACCOUNT_URL`,
  `SIMULATOR_AI_OAUTH_CLIENT_ID` and `SIMULATOR_AI_CONFIG_DIR`. `make client-smoke` gains `-profile login`.

- Go 1.27.4 (`go.mod`) for GO-2026-6859 in `net/http`; a source build needs Go 1.27.4 or a toolchain download.
- Build and edit large multi-page Smart Forms from the AI Console. `smart_form_files_get` answers at most 40,000
  characters of sources (files past that come without `source` and a `note`; one long file is cut with `next` and read
  on with `offset`), reads a page or definition as an outline (`outline: N`, one line per node) and one node by
  `nodeId` (an id or a JSON Pointer), and no longer asks `application/struct` for sources. `smart_form_files_update`
  requires `ifMatchHash` with `source`; a stale one is refused with the file's hash now and the node ids the sent
  source lacks. `smart_form_release` reads page configs one by one for its pending check. `smart_form_page_render`
  joins the plain `console` catalog (62 tools), takes `ref`, `envTitle`, `page`, `query` and answers a summary
  (`full: true` for the whole render).

- `graph_build` groups a hierarchy: ranks are ordered by their neighbours (barycentre sweeps) and every node's
  children sit in a block under it, the parent centred, so an org chart no longer interleaves departments over wrapped
  rows. More than six childless children fold into a near-square block (25 make 5 by 5); a row wider than `rowCells`
  is narrowed first and wrapped inside the parent's block only then. Flowchart blocks keep whole ranks, now in the
  same order, as does `flowchart_layout`. Without `origin` a section takes the highest free place within 80 columns of
  the layer's content, so a layer built in sections fills rows instead of one strip; the answer's `next` is gone.

- Build large graphs from the AI Console: `graph_build` creates actors, placements, edges and edge placements in one
  call from caller-chosen keys (up to 2000 nodes and 4000 edges, atomic batches of 99, layered / grid / given layouts
  with rank wrapping and an origin), repeats safely under an `operationId` (actor refs `gb:<operationId>:<key>`) and
  answers a compact result; `graph_build_delete` removes one operation. `actors_create` takes an `operationId` too.
  Both are in the plain `console` catalog.
- New tool group `graph_extra` (holes except `hole_close`, `layer_draw`, `layer_node_expand`, `layer_settings_update`,
  `layer_stats_get`, `layer_compact`, `layer_prune_long_edges`), split from `graph`: hosts that filter by group must
  know the name.
- `layer_placements_create` and `layer_positions_update` take 500 items (the server's limit); `layer_compact` moves
  layers of any size in ordered calls of 500; `flowchart_layout` wraps wide ranks and takes an `origin`.
- Bulk creations send `addToRecent: false`. Layer occupancy is read in pages of 200 until an empty page (a short
  page is not the last).

- Run the main-promotion check (Develop only) on `pull_request`: GitHub's default block of `pull_request_target`
  for public repositories stopped it from starting (startup_failure during the 2026-11-09 brownout).

- Answer "how many templates" exactly and cheaply: `forms_count` joins the plain `console` catalog and core group,
  and `forms_list` rows are compact (id, title, type, isDefault; `full` for the rest). `reactions_list` drops the
  AI reasoning trace (`treeInfo.reasoning`) and `accounts_list` its per-row noise unless `full` is sent.

- Add `layer_stats_get`, `layer_node_expand`, `layer_settings_update` and `account_pair_create` to the plain
  `console` catalog: AI Console runs on the baseline (router fallback) asked for them and missed them.

- Define repository quality criteria and attach verified CI/archive evidence to every release.
- Add a PR template; automated AI review and mandatory review approval remain disabled.

- Generate tool/profile availability from MCP registrations and check documentation drift in CI.
- Organize public documentation by workflow and keep complete inventories out of agent instructions.

- Add a Kiro Power with shared skills and read-only MCP configuration.
- Refresh public documentation and remove obsolete development-stage notes.

- Add Windows x64 ZIP packaging, PowerShell setup and native Windows MCP verification.

- Enforce develop as the only PR source for main; prepare release metadata before pre promotion.

- Add MIT licensing, packaged dependency notices and checksum/install verification.
- Support release archive rehearsals without publishing and protected-PR weekly promotions.

### Added
- Required CI for formatting, lint, contracts, tests, race detection and vulnerability/secret scans.
- Reproducible CDU reference checks from committed public input projections.
- Version inspection without credentials: `simulator-ai --version`.
- Verified release artifacts for Linux and macOS, coordinated by sim-deploy.

### Changed
- Rewrite the public README around installation, profiles and examples; move the detailed tool catalog into its own reference.

### Fixed
- Upgrade the Go toolchain to 1.27.3 for GO-2026-6809.
- Resolve static-analysis findings and refuse redirects in the live smoke HTTP client.
