# Security policy

Report suspected vulnerabilities privately using this repository's GitHub Security advisory
reporting when enabled, or contact the repository owners privately. Do not disclose credentials
or an unpatched exploit in a public issue. The owner must configure the preferred reporting route
before broad public distribution.

CI blocks on reachable known Go vulnerabilities and detected secrets. Module-only findings are
triaged separately; record reachability, owner and remediation rather than treating them as safe
forever. Tool/database/network failure is not a successful scan. CI rescans develop daily.

Gosec runs inside golangci-lint. Generator filesystem/process warnings have narrowly documented
exceptions for operator-owned source paths. The checked-in PDF fork is excluded from lint because
it preserves upstream source layout; its upstream revision/license/local patches are in
internal/third_party/pdf/README.md. It is included in Go compilation/vulnerability analysis and
bounded-input attachment tests. Upstream security fixes for copied code require explicit review;
module scanning alone cannot identify that fork's upstream advisories.

Credentials come only from process configuration, never tool arguments. Read-only profiles,
workspace binding, no-redirect transport, response bounds and per-profile recovery rules remain
security invariants. Skills and MCP annotations are not authorization. Never weaken tests or
write preflight to obtain green CI. No model calls or production credentials belong in PR jobs.
