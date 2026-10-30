# R1.1 boundary

Flow: host agent → stdio MCP → typed public client → POST /api/2.0/rpc.
The client has three exported reads and a closed internal method allowlist. There is no
server orchestration, model SDK, database, shell tool or legacy fallback. Skills are packaged
separately from MCP and do not manufacture permissions.

The public schema is pinned at source commit d1b467c97835a408d34386e199102d01b7c1b243.
Generated types are a deliberate metadata projection. Unknown backend fields are discarded;
this prevents form editor tokens or unrequested related data reaching the host. Missing
optional fields remain absent; layer accessDenied strips any actor details defensively.
The API still owns ACL: these local checks cannot certify deployed server policy.

Origin/workspace/key come only from process config. HTTP is allowed only for literal
loopback development; redirects are never followed. A key is sent as Authorization ApiKey,
never as a tool argument. 30-second request timeout, caller cancellation, 4 MiB response
limit, one stdio tool worker and bounded queue (32) keep local resource use bounded.
Layer page maximum is 50, default 50. Offset pagination is not a consistent snapshot under
concurrent edits, and no total/hasMore is invented. The backend filters deleted actors after
pagination; even an empty page does not certify completion. Full enumeration is outside
this profile. ID mismatches and malformed JSON-RPC
responses fail rather than returning partial metadata as success.

Errors expose stable code/allowlisted reason/status, not raw backend messages, bodies,
credentials or URL errors. Transport/read errors may be retried by the host with the same
arguments; the client performs no retries. Invalid/unknown tool arguments fail before any
request. Returned descriptions remain untrusted source data, never executable instructions.

Remaining gates: installed plugin/skill/MCP smoke in both desktop clients; live API-user ACL,
wrong workspace/object, integration public-only enforcement and revoke; backend/schema drift
against target installation. These gates require a designated test identity/workspace. Do not
use a developer's broad human session as substitute. Writes require domain-owned receipts,
CAS and response-loss tests from A01 before any new tool is exposed. Console, OAuth, HTTP MCP,
Corezoid connector, migration and paid billing remain separately planned slices.
