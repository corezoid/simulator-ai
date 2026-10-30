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
| Codex MCP inventory | The installed server exposes exactly actor_get/form_get/layer_actors_list |
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
- Hosted CI activation: available GitHub token lacks `workflow` scope; prepared workflow
  remains `docs/ci/check.yml.pending`. Local checks are not a hosted CI verdict.
- Public directory submission: Claude validator reports a missing license. The legacy repo
  is MIT, but this new repository has no explicit license grant yet; obtain owner choice
  before directory submission. Local installation succeeds. No license was invented here.

The local functional sim-api harness was inspected: it uses reusable test PostgreSQL/Redis/
Scylla and external Single Account, including real API-user creation. This smoke deliberately
uses no such infrastructure and does not claim its auth behavior has been exercised.
