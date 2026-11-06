# Client installation checks

The smoke verifies plugin loading and protocol behavior in disposable client profiles.
Live authorization and model-driven workflows require separate checks.

## Repeatable smoke

On macOS or Linux, install Go and the `codex` and `claude` CLIs, then run:

```sh
make test vet
make client-smoke
```

`cmd/client-smoke` creates fresh, private temporary client configuration directories and a
scratch project, installs this checkout as a local marketplace, and deletes the profiles
on exit. It does not modify the user's installed plugins or MCP configuration. One client
runs at a time. Each short client command has a 60-second timeout; the complete smoke has
a four-minute deadline. Process-group cleanup only stops children created by the smoke.
The already-built executable is made available through the test process PATH.

The child environment is allowlisted; production/provider credentials are not inherited.
Only synthetic API-user credentials and a loopback scripted API are supplied. Codex's model
provider and Claude's model endpoint point at a rejecting loopback route as an additional
check against accidental model invocation. No `turn/start`, `codex exec`, Claude prompt or
paid evaluation is part of the test. Claude inventory may probe `count_tokens`; those
non-generative requests are explicitly rejected locally and counted in evidence, so inventory
token estimates are not validated. Every other unexpected request fails the fixture.

The JSON evidence records client versions and these separate checks:

| Check | What it establishes |
| --- | --- |
| Codex plugin install | Real marketplace install into its disposable cache |
| Codex skills/list | The installed, enabled `simulator-ai:simulator-ai-read` skill loads |
| Codex MCP inventory | The installed server exposes the exact selected-profile catalog from this checkout |
| Codex MCP direct call | app-server mcpServer/tool/call returns one projected fixture actor |
| Claude plugin install/details | Real installation and loaded skill inventory |
| Claude mcp list | The installed plugin's server completes the client connection check |

Claude's connection check is not evidence of a Claude model choosing or executing a tool.
The server's stdio call path is independently tested, and Codex calls the tool directly
without a model. Model-driven acceptance in either client requires a separate approved
budget and must never be inferred from this smoke.

## Environment forwarding

Codex uses `.codex-mcp.json` with an explicit `env_vars` allowlist. Claude uses `.mcp.json`
and inherits the configured process environment. Neither file contains credentials.
Unsupported client protocols fail the smoke instead of silently skipping assertions.

See [live backend checks](live-backend-acceptance.md) for ACL, public-audience and issuer
revocation requirements. Installed-client checks do not establish these guarantees.
Kiro setup and its current verification boundary are documented [separately](kiro.md).

## Reactions profile checks

The same disposable-client workflow also accepts `-profile reactions-read` and
`-profile reactions-actor`. After building the current binary:

```sh
GOMAXPROCS=2 go run -p=1 ./cmd/client-smoke -profile reactions-read
GOMAXPROCS=2 go run -p=1 ./cmd/client-smoke -profile reactions-actor
```

Both require the installed `simulator-ai-reactions` skill in Codex and Claude.
Workspace mode checks exactly four tools and directly reads metadata plus reaction
counts via Codex app-server. Actor mode sets a synthetic host actor, checks exactly
one tool and calls it with `{}`. Counts preserve `comment:2` and `view:0` from the
loopback fixture. The emitted `codexReactionStats` flag records that assertion.
Claude installation/skill/MCP connection is still distinct from model-driven tool
execution. No paid provider calls, user configuration changes or issuer revoke
acceptance are added by these commands. See [reaction contract](reactions-read.md)
for real-backend functional checks and the host-binding boundary.

## Layer-statistics profile checks

`go run ./cmd/client-smoke -profile graph-read` checks the current workspace
catalog and directly reads metadata plus all six layer counters through Codex.
`-profile graph-actor` supplies a host layer and checks the one-tool catalog and
`{}` call. Both require the installed graph skill in Codex and Claude; the fixture
includes a zero `edgeHolesClosed` value. `codexLayerStats` records the assertion.
Claude still checks installation and connection only. These commands use the same
disposable synthetic loopback setup and never start a model turn. See
[layer semantics](layer-stats.md) for backend and remaining acceptance boundaries.

Graph-read also calls `graph_edge_types_list` with explicit isSystem=false and a
CSV projection, then checks an exhausted offset page. The fixture preserves a
selected null creator and verifies exact projected fields; `codexEdgeTypes` records
this check. Graph-read now expects four fixture reads (actor, layer counts and two
edge-type pages). The actor-bound graph mode still exposes only layer counts and
expects one read, matching the hidden legacy edge-type catalog in actor mode.
