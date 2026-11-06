# R1.2a — installed clients, without model calls

This slice closes repeatable local plugin loading checks. It does not approve the complete
R1 release: backend ACL/public-audience/revoke acceptance against a designated API-user
workspace is still required. No UI Kit, production config or legacy routing is changed.

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

## Configuration fix found by the smoke

Codex did install the original skill, but its MCP subprocess did not inherit the three
Simulator configuration variables. `.codex-mcp.json` now declares the exact `env_vars`
allowlist; the Codex manifest selects that file. Claude keeps its compatible `.mcp.json`
and inherits the synthetic process variables. Neither file contains credential values.
The README's direct Codex TOML example includes the same allowlist.

This matches the official [Codex MCP configuration](https://developers.openai.com/codex/mcp)
and [plugin stdio environment rules](https://developers.openai.com/api/docs/guides/agents-api/tools/plugins).
Host protocol assumptions are checked against the installed CLI's generated app-server
schema (`codex app-server generate-json-schema --experimental`). Unsupported client versions
fail the smoke; they do not silently downgrade the assertions.

## Live backend follow-up

The R1.2b [live backend acceptance](live-backend-acceptance.md) exercises real readonly keys,
object ACLs and this binary. It found and fixed the string JSON-RPC ID mismatch in version
0.1.0-dev.3. The issuer-level revoke gate below remains separate.

## Remaining release checks

- Live new sim-api, `integrationAPI=enforce`, a designated API-user test workspace and known
  allowed/denied actor, form and layer fixtures. Prove allowed reads, wrong workspace/object
  denial, public-only enforcement and revoke, including the documented API-key cache window.
  Use backend functional fixtures for private-method probes; do not add private tools to MCP.
- Claude model-driven tool execution, if required by the acceptance owner: approved budget.
- Hosted CI is now configured in [check.yml](../.github/workflows/check.yml); the original
  workflow-scope blocker is resolved. Check the exact PR/commit run before promotion.
- The repository now declares MIT, and release archives include third-party license notices.
  Directory submission and its client-specific validation remain a separate publishing step.

The local functional sim-api harness was inspected: it uses reusable test PostgreSQL/Redis/
Scylla and external Single Account, including real API-user creation. This smoke deliberately
uses no such infrastructure and does not claim its auth behavior has been exercised.

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

On 2026-11-03 both commands passed with Codex CLI 0.142.5 and Claude Code
2.1.312. Workspace mode reported four tools/two fixture reads; actor-bound mode
reported one tool/one fixture read. Both reported installed skills, direct Codex
reaction counts and Claude MCP connectivity. Four Claude token-count probes per
run were rejected locally; no model request or fixture write occurred. These are
local installation results, not hosted CI or issuer-side revocation acceptance.

## Layer-statistics profile checks

`go run ./cmd/client-smoke -profile graph-read` checks the eight-tool workspace
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
