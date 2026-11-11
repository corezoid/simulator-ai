# Claude Code instructions — Simulator AI

Read [AGENTS.md](AGENTS.md) first; it is the shared operating contract.

Use an isolated worktree on feature/*, bugfix/* or hotfix/*. Feature/fix PRs target develop;
main is the frozen pre candidate. sim-deploy coordinates weekly promotion and production tags.
Keep source modules below 700 lines; prefer small fixes and existing helpers over frameworks.

Use Go 1.27.4. Run targeted checks first, then `make check` sequentially with GOMAXPROCS=2
and Go -p=1. Heavy race/cross-platform jobs belong in CI. Never run multiple test stacks,
local Docker builds or paid model calls without their explicit scope/budget.

`make gen-check` is self-contained. When refreshing source contracts, also run strict
`make gen-source-check` with SIM_API_DIR/UI_KIT_DIR. Do not hand-edit generated artifacts.

After implementation, request one independent xhigh review, fix confirmed findings and
re-review the exact final diff/commit. Report actual checks and unresolved gates. A green
pipeline is not evidence of a running environment or live backend authorization behavior.

Desktop uses the host agent and public sim-api only. No hidden Hermes/model runtime,
private API, databases, credential logs or legacy fallback. See docs/release.md and SECURITY.md.

Main accepts only same-repository develop PRs; hotfixes and release metadata go through develop.
