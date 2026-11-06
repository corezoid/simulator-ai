# R1.1 verification

Development profile only. All API responses in local tests come from scripted HTTP servers;
no model calls, server agent, real workspace writes or production credentials are used.
Checks and independent xhigh review are recorded on the PR with the exact reviewed commit.

R1.2a adds real isolated client-install smoke; see [client acceptance](client-acceptance.md).
Live API-user ACL/revoke acceptance remains open: transport mocks cannot prove backend
rights or full legacy parity.

Local 2026-10-30 checks:

- Public schema projection and generated-client drift gate; typed HTTP happy/error,
  malformed response, identity mismatch, timeout/cancellation, bounded body/page and redirect
  rejection tests; actual subprocess stdio initialize/list/call and invalid/write-tool denial.
- The initial cancellation test fixture blocked its HTTP handler without draining the request;
  corrected fixture cleanup and added a 60-second test-suite timeout. This was a mock teardown
  issue, not a claimed API/runtime conformance pass.
- `make test vet build` uses GOMAXPROCS=2 and Go -p=1; no full sim-api/UI Kit builds.

Review corrections: do not infer layer completion from short/empty pages (backend deletion
filter runs after pagination); enforce MCP input JSON Schema in addition to Go decoding.
Regression tests cover a later live page after an empty one, case-sensitive property names,
null pagination and fractional limits, with zero backend calls for invalid arguments.
Malformed/null placements, missing identities and nested actor identity mismatches now fail;
denied actor metadata remains stripped before checking any unreadable nested actor.

The original CI authorization blocker is superseded by the active
[GitHub workflow](../.github/workflows/check.yml). The historical local results above remain
local evidence; hosted verdicts belong to the exact PR/commit runs. See [release checks](release.md).

R1.2b adds [real backend acceptance](live-backend-acceptance.md), including the actual public
wire ID contract and ACL revoke/regrant through a persistent MCP process. Exact paired commits
and review/check results are recorded in its PR/MR; issuer revoke is still not claimed.

2026-11-03 graph-read: `make test vet build` passed with GOMAXPROCS=2 / -p=1.
The source-size and generated-client guards passed. Tests cover the pinned three-method
public projection, input/output schemas, HTTP response validation and actual subprocess
stdio initialization/catalog/calls for read, delegated-read and graph-read. Graph reads
use only scripted loopback responses; no live Graph Editor or model acceptance is claimed.
The exact reviewed commit and xhigh verdict are recorded in the slice PR.

2026-11-03 graph-create: `make test vet build` passed with bounded concurrency.
The new opt-in inventory has 12 tools. Scripted HTTP/MCP tests exercise all six public
create/recovery operations, per-write public-schema checks, response-loss lookup,
wrong result bindings, repeated actor placements and invalid/delete argument denial.
Actual subprocess stdio loads graph-create with the expected tool names/annotations.
Native schema cases cover full explicit positions, anchored edges, shape objects,
text-frame/dimension bounds and rejected unknown settings. No live graph write or
Console acceptance is claimed; reviewed commit/verdict are recorded in the PR.
