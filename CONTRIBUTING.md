# Contributing

Open an issue with a credential-free reproduction before proposing a broad change.
For a focused fix, use a feature or bugfix branch and open a pull request into `develop`.
`main` accepts promotion PRs from this repository's `develop` only.

## Local checks

Use Go 1.27.3 and golangci-lint 2.14.0. Run `make check` sequentially; Go commands limit
parallelism to protect local resources. Hosted CI also runs race, vulnerability/secret,
coverage and Windows package checks. See [verification](docs/verification.md).

Every meaningful change updates `CHANGELOG.md` under Unreleased. Ordinary PRs do not bump
versions. Keep tests focused on observable behavior, including denied/invalid inputs and
recovery for writes. Never add real credentials or production data to fixtures.

## Documentation ownership

- MCP registration code owns tool names, profile membership and annotations. Run `make docs`
  after changing it; `make gen-check` rejects a stale generated catalog.
- [Tool guide](docs/tool-reference.md) is the navigation entry point. Human-written API and
  workflow guides own permissions, side effects, bounds and recovery semantics.
- Skills teach task execution using the current host catalog. Avoid duplicating complete
  profile inventories or fixed tool counts in skills, README or agent instructions.
- Update API behavior documentation and examples in the same PR as an observable change.
  Check relative links when moving a guide. Do not edit generated contracts by hand.

[AGENTS.md](AGENTS.md) is the shared agent-development contract; [CLAUDE.md](CLAUDE.md)
points Claude Code to it. Release ownership and artifact rules are in
[release procedures](docs/release.md).
