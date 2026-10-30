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

CI activation is pending: the available GitHub token lacks `workflow` scope and SSH has no
authorized key. The prepared workflow is in `docs/ci/check.yml.pending`; a repository admin
can move it to `.github/workflows/check.yml`. Local checks above are executed; no hosted CI
pass is claimed.
