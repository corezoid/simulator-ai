# Architecture and API boundaries

Flow: host agent → stdio MCP → public API client → POST /api/2.0/rpc.
File upload/download tools also use the public file endpoints listed in the tool reference.
The default `read` profile exposes read-only actor/form tools and paginated layer reads.
Each optional profile has its own catalog; see the [tool reference](tool-reference.md).
[Graph metadata](graph-read.md) adds three metadata reads, exact placement readback and six layer counters with projected input/output schemas.
[Graph creation](graph-create.md) adds six create/recovery operations with mandatory
receipts and a public-contract check before each write; no update/delete tools. There is no
server orchestration, model SDK, database, shell tool or legacy fallback. Skills are packaged
separately from MCP and do not manufacture permissions.

The contracts directory records source revisions and public schema projections. Typed metadata
reads discard unknown backend fields and strip denied actor details. The parity tools in `read`,
`graph-read` and Console profiles instead expose their declared public request/result shapes,
including documented `fields`, `with` and editor options. Do not assume every profile returns
metadata only. The backend enforces object permissions for every request.

Origin/workspace/key come only from process config. HTTP is allowed only for literal
loopback development; redirects are never followed. A key is sent as Authorization ApiKey,
never as a tool argument. 30-second request timeout, caller cancellation, 4 MiB response
limit, one stdio tool worker and bounded queue (32) keep local resource use bounded.
Layer page maximum is 50, default 50. Offset pagination is not a consistent snapshot under
concurrent edits, and no total/hasMore is invented. The backend filters deleted actors after
pagination; even an empty page does not certify completion. This paginated tool alone cannot
certify full enumeration; explicit graph enumeration tools have their own bounds in the [tool reference](tool-reference.md). ID mismatches and malformed JSON-RPC
responses fail rather than returning partial metadata as success.

Errors expose stable code/allowlisted reason/status, not raw backend messages, bodies,
credentials or URL errors. The console profile alone appends the server's error message and
reasonParams, bounded to 2000 bytes, so the Console agent can correct a refused page config or route. Transport/read errors may be retried by the host with the same
arguments; the JSON-RPC transport performs no retries. File downloads have the bounded
status-specific retry behavior documented in the tool reference. Invalid/unknown tool arguments fail before any
request. Returned descriptions remain untrusted source data, never executable instructions.

## Validation boundaries

[Client acceptance](client-acceptance.md) describes isolated installation and MCP checks.
Live acceptance must also cover API-user permissions, wrong workspace/object denial, public-API
enforcement, revocation and compatibility with the target backend. Use a designated test
identity and workspace; a broad human session is not a substitute for an API-user check.

Smart Forms and graph-create writes have domain-specific recovery keys, contract checks and
response-loss tests. Console profiles have different backend-owned retry semantics and may
use a host-provided per-request user credential. See the [tool reference](tool-reference.md)
for each domain. Local checks and CI do not certify deployment, permissions across a running
fleet or successful model-driven execution.
