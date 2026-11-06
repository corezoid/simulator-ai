# Simulator AI

New public plugin, separate from simulator-ai-plugin. Domain tools call only documented
public sim-api JSON-RPC at /api/2.0/rpc. Never use /papi, private RPC, SQL or internal HTTP.
Desktop uses its host agent: do not add Hermes, LiteLLM or server billing to this path.

## Runtime and documentation boundaries

The default profile is read-only. Profile registration code is authoritative; see the
[generated catalog](docs/tool-catalog.md) and [workflow guide](docs/tool-reference.md).
Do not maintain another tool inventory or fixed counts in agent instructions or skills.

Credentials, origin and workspace are host configuration, never model-selected arguments.
Delegated reads use only their explicit scoped token. Strict graph-create and Smart Forms
writes require contract preflight and retained recovery keys/CAS; Console writes have
backend-owned retry semantics. Preserve these separate contracts and current authorization
checks. Capability changes require public schemas, permissions/error/recovery tests and
updated human documentation. A schema match alone does not establish behavioral compatibility.

Go code/commands, pinned dependency versions, gofmt. Never hand-edit types.gen.go;
run make generate and make gen-check. Test with GOMAXPROCS=2 and -p=1; no paid model calls
without a separately approved budget. Credentials stay in process environment and never
enter tools, output or logs. Skills remain portable and reflect the actual catalog.

Small feature/*, bugfix/* or hotfix/* branches and PRs to develop. Review each slice with one independent reviewer
configured xhigh, fix confirmed findings, repeat review on the exact commit/diff and record
checks. Full rollout requires separate client-install and live API-user ACL/revoke evidence.

## Worktrees, structure and verification

- Work in a separate git worktree on a small `feature/*`, `bugfix/*` or `hotfix/*` branch. Leave the primary checkout,
  unrelated local changes and existing services untouched. Publish each slice as its own PR.
- Keep every authored source module at or below 700 lines, including tests. Start splitting
  around 600 lines. Separate cohesive responsibilities (transport, domain logic, storage,
  fixtures); do not hide oversized modules behind generated code or arbitrary file chunks.
  Generated contracts are refreshed by their generator, never edited to satisfy the limit.
- Prefer named typed contracts and small focused functions. Reuse existing modules; avoid
  unrelated abstractions, duplicated validation and compatibility paths without a requirement.
- After implementation, use one independent reviewer configured with `reasoning_effort=xhigh`.
  Fix confirmed findings and repeat review on the final commit/diff. Record the reviewed SHA,
  tests and unresolved gates in the PR; never claim xhigh when unavailable.
- Protect local resources: targeted tests first, GOMAXPROCS=2, Go `-p=1`, sequential client
  smoke and a bounded timeout. Do not run unnecessary full builds, multiple test stacks or
  paid model tests. Paid tests require an explicitly approved budget.
- `make check-source` enforces the module line limit. `CLAUDE.md` points to this contract.

Host-bound counts never replace current receipt readback or recipient authority.


## Development and weekly release

- Permanent branches: develop (integration/dev), main (frozen pre candidate). Feature/fix PRs
  target develop. Only a PR from this repository's develop branch may target main. Tags vX.Y.Z identify prod.
- sim-deploy coordinates the weekly backend/plugin train. It verifies the exact candidate SHA,
  not a moving branch or an unrelated green run. Do not independently publish a stable plugin.
- Version is minted once per release in internal/version/version.go and all three host plugin manifests.
  Every normal PR updates CHANGELOG.md under Unreleased; no per-PR version bumps.
- Binary, manifests and skills (including the Kiro Power) must come from the same commit. API origin, workspace and
  credentials are runtime inputs; never bake dev/pre/prod endpoints or credentials into an artifact.
- Green CI is not deployment or live ACL/revoke acceptance. See docs/release.md for the boundary.
- Required merge check is CI / Gate: formatting, module consistency, source/generation checks,
  lint, tests/coverage, race and vulnerability/secret scans. Missing or skipped jobs do not pass.
- Source/runtime compatibility includes permissions, side effects, CAS and retry semantics;
  a schema match alone is not proof. Do not weaken strict write preflight to make rollout pass.

## Reproducible commands and agent budget

- Go 1.27.3 (go.mod); golangci-lint 2.14.0. `make check` runs checks sequentially.
- `make test` is the default local suite. `make test-race` and `make security` are separate;
  prefer hosted CI for race and cross-platform packaging. Never run make -j, parallel full suites,
  local Docker builds or multiple client-smoke processes for this task.
- `make test-cover` enforces measured baselines for mcpserver/publicapi/simulation. Do not lower
  them to hide a regression. Third-party PDF code has separate provenance and bounded-input tests.
- `make gen-check` always reproduces output from committed public input projections, with no
  private credentials and no silent skips. `make gen-source-check` strictly checks their pinned
  upstream commits; it fails if either checkout/commit is missing. Run it when refreshing CDU
  inputs using SIM_API_DIR and UI_KIT_DIR. Never edit generated inputs or outputs by hand.
- CI tools are version-pinned. Additional global agents, background daemons, model frameworks
  and paid evals are not needed for normal development. Use the host agent plus these commands.
- Before merge: independent code review with reasoning_effort=xhigh, fix confirmed findings,
  repeat on the final diff/SHA, and record tests/review/remaining gates in the PR.

- Both permanent branches require PRs and strict Gate, including administrators. sim-deploy
  promotes and updates version metadata through checked temporary release PRs; never bypass protection.
- `make package` requires a clean commit and ships MIT/dependency notices with per-platform build
  metadata. Use one local platform; prefer the manual Release workflow for cross-platform rehearsal.

Main accepts only same-repository develop PRs; hotfixes and release metadata go through develop.

Run `make docs` after tool registration/annotation changes. `make gen-check` includes the
generated catalog drift check. See CONTRIBUTING.md for documentation ownership.

## Automated release evidence

QUALITY.md defines repository quality criteria. The Release workflow records real reusable-CI
job results and verifies every archive checksum before emitting quality-report.json/md.
It does not run an AI reviewer, require review approval or assign a numerical score.
Never claim a review from a green CI job or a filled PR template. The internal deployment
coordinator remains automatic when required checks pass. This release policy does not
replace any explicitly requested development review; follow the owner's current scope.
