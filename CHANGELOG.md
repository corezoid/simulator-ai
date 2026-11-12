# Changelog

## [0.1.0]

### Changed
- record verified release quality evidence without review gates
- use a repository link in copied PR descriptions

## [Unreleased]

- Sign in as yourself, without an API key: with no host credential the server starts in login mode, and `login`
  opens the SuperAdmin page in the browser (OAuth2 PKCE, the legacy plugin's public client), `workspaces_list` and
  `workspace_set` choose the workspace, `whoami` and `logout` complete the set. The session and the chosen workspace
  are kept per API origin in `~/.config/simulator-ai/credentials.json` (0600) and renewed with a refresh token when
  SuperAdmin issues one. `SIMULATOR_AI_API_KEY` / `SIMULATOR_AI_AUTHORIZATION` / `SIMULATOR_AI_DELEGATED_TOKEN` and
  `SIMULATOR_AI_WORKSPACE` keep precedence, and a host Authorization process (AI Console) gets no new tools.
  `SIMULATOR_AI_ORIGIN` defaults to `https://mw.simulator.company`; new optional `SIMULATOR_AI_ACCOUNT_URL`,
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
