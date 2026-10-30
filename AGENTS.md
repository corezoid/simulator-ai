# Simulator AI

New public plugin, separate from simulator-ai-plugin. Domain tools call only documented
public sim-api JSON-RPC at /api/2.0/rpc. Never use /papi, private RPC, SQL or internal HTTP.
Desktop uses its host agent: do not add Hermes, LiteLLM or server billing to this path.

Default profile is three read-only metadata tools, explicit ApiKey/origin/workspace.
The opt-in smart-forms profile adds thirteen typed public tools with mandatory recovery
keys/CAS and backend contract preflight. No OAuth, remote MCP or full legacy parity claim. Capability additions require
source schemas, named parity mapping, ACL/error/side-effect/recovery tests and docs.

Go code/commands, pinned dependency versions, gofmt. Never hand-edit types.gen.go;
run make generate and make gen-check. Test with GOMAXPROCS=2 and -p=1; no paid model calls
without a separately approved budget. Credentials stay in process environment and never
enter tools, output or logs. Skills remain portable and reflect the actual catalog.

Small codex/* branches and PRs to develop. Review each slice with one independent reviewer
configured xhigh, fix confirmed findings, repeat review on the exact commit/diff and record
checks. Full rollout requires separate client-install and live API-user ACL/revoke evidence.

## Worktrees, structure and verification

- Work in a separate git worktree on a small `codex/*` branch. Leave the primary checkout,
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
