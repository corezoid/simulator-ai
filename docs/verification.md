# Verification

Use the commands below from the repository root. Local checks limit Go to two OS threads
and one package build at a time. Hosted CI runs the heavier race and cross-platform checks.

| Command | What it checks |
| --- | --- |
| `make check` | Formatting, module consistency, lint, generated-source checks, tests and build, sequentially. |
| `make test-cover` | Tests plus the measured package coverage baselines. |
| `make test-race` | Go race detection. |
| `make security` | Reachable vulnerabilities in Go code and dependencies. |
| `make gen-check` | Generated output matches the committed public input projections. |
| `make gen-source-check` | Input projections match their pinned upstream source commits; requires the source checkouts. |
| `make client-smoke` | Disposable Codex and Claude plugin installation and MCP checks, without model inference. |
| `make package PLATFORMS=darwin/arm64` | One release archive from a clean commit; choose the local platform. |

Unit and protocol tests use scripted HTTP responses. They cover input/output validation,
fixed origin/workspace binding, credential redaction, authorization errors, bounded reads,
profile catalogs, explicit write recovery and incompatible-contract refusal. They cannot
prove the connected backend's current permissions, transactionality or issuer revocation.

[CI](../.github/workflows/check.yml) also scans git history for secrets, packages Linux and
Windows artifacts, and verifies the Windows ZIP's native stdio initialization and read
catalog. Every required job must succeed; a skipped or missing job is not a passing Gate.

See [client checks](client-acceptance.md), [live backend checks](live-backend-acceptance.md)
and [release procedures](release.md) for the distinct installation, deployment and rollback
boundaries. A green workflow establishes checks for its exact commit, not a live rollout.
