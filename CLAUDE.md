# Claude Code instructions — Simulator AI

Read and follow [AGENTS.md](AGENTS.md), the shared operating contract.

Work in a separate worktree on `codex/*`; preserve unrelated changes. Keep source modules
at most 700 lines and split cohesive responsibilities before they become large. Use targeted
checks with low concurrency. After each completed slice, obtain independent xhigh review,
fix confirmed findings and re-review the exact final commit. Do not claim unavailable review
or unexecuted checks. No paid model calls without an approved budget.

The desktop plugin uses only public sim-api and the host's own agent. It must not start
server Hermes, access databases or silently fall back to legacy. See AGENTS.md for all
architecture, credential, generation and release rules.
