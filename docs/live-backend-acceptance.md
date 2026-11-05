# R1.2b — real sim-api wire and authorization acceptance

The local sim-api functional suite now exercises this executable with a real readonly
API-user from non-production Single Account and real local test databases. Its test is
`TestSimulatorAIReadProfile`, with optional subtest `MCP binary uses real API-user reads`
(enabled by `SIMULATOR_AI_MCP_BINARY`). The pairing is recorded by exact commits in the PR/MR.

The first live run found that this client sent numeric JSON-RPC IDs. sim-api documents and
requires **string** IDs (`internal/server/rpc_handler.Request.ID`). Version `0.1.0-dev.3`
emits a monotonic string ID and requires an exact string echo. Numeric, missing, null and
mismatched response IDs fail. Scripted HTTP fixtures now model that actual server boundary;
MCP's own stdio and Codex app-server IDs remain independent of the public HTTP transport.

The backend also maps an explicit SA 401/403 on scope verification to code 1,
`reason=notAuthorized`, instead of treating it as an internal error. This client preserves
that bounded reason without exposing issuer messages. Transport, rate-limit and server
failures are not relabeled credential errors.

Executed assertions cover actor/form/layer reads, no metadata leakage from a hidden layer
actor, no extra tools, and all three reads failing after ACL revocation while the MCP process
remains alive. Regrant restores all three reads. Backend-only assertions also cover readonly
scope despite Modify ACL, wrong workspace, internal RPC refusal in direct/atomic/non-atomic
calls, the per-scope cache TTL and immediate local token blocking over warmed caches.

No paid model or Hermes runtime was called. The user-supplied Hermes dev origin belongs in
the backend's A04 environment notes, not in this desktop MCP configuration.

This is not complete issuer-revoke acceptance: sim-api's current SA client has no verified
API-user revoke/rotation operation, and available credentials could not read the referenced
issuer source repository. Local token blocking and ACL revocation are separate mechanisms.
The fixture blocks its test credential locally and deletes actor/form fixtures; it does not
claim permanent cleanup or rotation at SA. Full R1 rollout still requires that issuer gate,
hosted CI/release checks and any separately budgeted model-driven acceptance.

The existing [client smoke](client-acceptance.md) remains the repeatable installed-client
check. The new backend test is the complementary live wire/auth boundary, not a substitute
for either host packaging checks or production rollout approval.
