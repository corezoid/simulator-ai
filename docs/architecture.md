# Architecture and API boundaries

Flow: host agent → stdio MCP → public API client → POST /api/2.0/rpc.
File upload/download tools also use the public file endpoints listed in the tool reference.
The default `read` profile exposes read-only actor/form tools and paginated layer reads.
Each optional profile has its own catalog; see the [tool reference](tool-reference.md).
[Graph metadata](graph-read.md) adds three metadata reads, exact placement readback, six layer counters and chart data (`dashboard/get`) with projected input/output schemas.
[Graph creation](graph-create.md) adds six create/recovery operations with mandatory
receipts and a public-contract check before each write; no update/delete tools. There is no
server orchestration, model SDK, database, shell tool or legacy fallback. Skills are packaged
separately from MCP and do not manufacture permissions.

The contracts directory records source revisions and public schema projections. Typed metadata
reads discard unknown backend fields and strip denied actor details. The parity tools in `read`,
`graph-read` and Console profiles instead expose their declared public request/result shapes,
including documented `fields`, `with` and editor options. Do not assume every profile returns
metadata only. The backend enforces object permissions for every request.

Origin, profile and any host credential come only from process config. HTTP is allowed only for
literal loopback development; redirects are never followed. A key is sent as Authorization ApiKey,
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

## Credentials and browser login

The credential is chosen per request, in this order: `SIMULATOR_AI_AUTHORIZATION` (a host such as
the Hermes relay hands over the run initiator's header; no account tools are registered),
`SIMULATOR_AI_DELEGATED_TOKEN` (`delegated-read`), `SIMULATOR_AI_API_KEY`, and only then the
person's own browser login. A process with none of them still starts; every domain tool then
refuses before any request with "call the login tool".

`login` (package `internal/auth`) runs OAuth2 authorization code + PKCE (S256) against SuperAdmin
(`https://account.corezoid.com`, `SIMULATOR_AI_ACCOUNT_URL` for other stands) with the public client
the legacy Simulator plugin registered. A random `state` guards the loopback callback
(`http://localhost:<random port>/callback`); callbacks without it are refused without ending the
sign-in, which times out after 5 minutes. The tool waits up to 45 s (under Codex's default tool
timeout) and otherwise answers `pending`; the sign-in completes in the background. SuperAdmin's
`simulator_token` is a Simulator JWT that sim-api accepts as `Authorization: Simulator <jwt>` and
resolves to the person's own session, workspaces and roles; simulator-ai still calls only the
public methods each profile allows.

The token, the refresh token SuperAdmin may issue with it and the chosen workspace are stored per
API origin in `credentials.json` under `$SIMULATOR_AI_CONFIG_DIR`, `$XDG_CONFIG_HOME/simulator-ai`
or `~/.config/simulator-ai` (file 0600 written atomically under a `credentials.json.lock` shared by
all client processes, directory 0700; a group/world-readable file is refused). Each request re-reads
the token, so a login or renewal in another client process is used at once. A token within a minute
of its `exp` is renewed with the refresh token; only a refusal of the grant (an OAuth error, 400,
401 or 403 with an OAuth `error`) drops it and asks for `login` — 408, 429, 5xx and proxy pages leave it
for the next request. Renewal writes are compare-and-set on the pair they started from, so a logout
or another person's sign-in in another process always wins over a renewal in flight (a concurrent
renewal of the same person that SuperAdmin refused does not). Tokens never enter tool results, errors or logs; `whoami` shows only the
JWT's unverified `sub`/`nick`/`exp` for display. `workspace_set` accepts only a workspace that
`workspace/list` returns for the person; `SIMULATOR_AI_WORKSPACE` overrides it, and with an API key
the workspace is host configuration only. A sign-in as a different person forgets the previous
workspace choice, the same person signing in again keeps it. A process reads the stored workspace
once and keeps it while the same person is signed in: another client process choosing a workspace affects only processes started later,
and tool calls run one at a time, so a workspace change never splits an operation.

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
