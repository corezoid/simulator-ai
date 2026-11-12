# Security policy

Report suspected vulnerabilities privately through [GitHub's vulnerability reporting form](https://github.com/corezoid/simulator-ai/security/advisories/new).
Do not disclose credentials or an unpatched exploit in a public issue. Include the affected
version, profile and a credential-free reproduction. Maintainers triage reports privately
and coordinate fixes before public disclosure.

CI blocks on reachable known Go vulnerabilities and detected secrets. Module-only findings are
triaged separately; record reachability, owner and remediation rather than treating them as safe
forever. Tool/database/network failure is not a successful scan. CI rescans develop daily.

Gosec runs inside golangci-lint. Generator filesystem/process warnings have narrowly documented
exceptions for operator-owned source paths. The checked-in PDF fork is excluded from lint because
it preserves upstream source layout; its upstream revision/license/local patches are in
internal/third_party/pdf/README.md. It is included in Go compilation/vulnerability analysis and
bounded-input attachment tests. Upstream security fixes for copied code require explicit review;
module scanning alone cannot identify that fork's upstream advisories.

Credentials come only from process configuration or the person's own browser login, never tool
arguments. The login (OAuth2 PKCE through SuperAdmin) stores its token per API origin in the user's
config directory (`credentials.json`, 0600, never a project file); a host credential in the
environment always wins over it, and a host `SIMULATOR_AI_AUTHORIZATION` process exposes no login.
Tokens never appear in tool results, errors or logs. Read-only profiles,
workspace binding, no-redirect transport, response bounds and per-profile recovery rules remain
security invariants. Skills and MCP annotations are not authorization. Never weaken tests or
write preflight to obtain green CI. No model calls or production credentials belong in PR jobs.
