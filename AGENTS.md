# Simulator AI

New public plugin, separate from simulator-ai-plugin. Domain tools call only documented
public sim-api JSON-RPC at /api/2.0/rpc. Never use /papi, private RPC, SQL or internal HTTP.
Desktop uses its host agent: do not add Hermes, LiteLLM or server billing to this path.

Current slice R1.1 is three read-only metadata tools, explicit ApiKey/origin/workspace.
No write, OAuth, remote MCP or full legacy parity claim. Capability additions require
source schemas, named parity mapping, ACL/error/side-effect/recovery tests and docs.

Go code/commands, pinned dependency versions, gofmt. Never hand-edit types.gen.go;
run make generate and make gen-check. Test with GOMAXPROCS=2 and -p=1; no paid model calls
without a separately approved budget. Credentials stay in process environment and never
enter tools, output or logs. Skills remain portable and reflect the actual catalog.

Small codex/* branches and PRs to develop. Review each slice with one independent reviewer
configured xhigh, fix confirmed findings, repeat review on the exact commit/diff and record
checks. Full rollout requires separate client-install and live API-user ACL/revoke evidence.
