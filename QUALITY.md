# Repository quality standard

A release must pass the checks below for its exact source commit. A skipped, missing,
cancelled or failed required job is not a pass. Do not lower thresholds, exclude new code
or disable a check simply to publish a release. Changes to the standard must be explicit
in the PR and justified by evidence.

| Area | Required evidence | What automation does not establish |
| --- | --- | --- |
| Code and tests | Quality and Race jobs: formatting, module consistency, lint, tests, coverage baselines and race detection. | Complete absence of bugs. |
| Security | Security job: reachable Go vulnerability scan and redacted git-history secret scan; pinned CI tooling. | A complete security audit or live authorization behavior. |
| Maintainability | Quality job: source-size limit, static analysis and generated-source consistency. | Architectural judgment, useful abstractions or absence of unnecessary complexity. |
| Documentation | Quality job: generated contracts, CDU reference and actual tool/profile catalog agree with source. PR authors explain behavior and documentation changes. | Accuracy of every hand-written sentence or example. |
| Release integrity | Same-commit binary/manifests/skills, version checks, packaged license notices, native Windows verification and all five archive SHA256 checks. | A live deployment or compatibility across an already-running fleet. |

## Review policy

Automated AI review and mandatory review approval are currently **not enabled**. There is no
model API key, paid review job or new manual approval step in the release path.
The release report records `review: not_performed` and does not assign a numerical score.
Passing automated checks is not an automatically certified "5/5" expert assessment.
A review may still be performed during development; its evidence belongs to the PR and
must identify the reviewed commit. It is not inferred from a checked template box.

## Before merging

Use the [PR template](.github/pull_request_template.md) to record the observable change,
validation and affected documentation. Update `CHANGELOG.md` under Unreleased. The template
is guidance, not evidence that a test ran. Required CI results remain authoritative.

Both permanent branches require passing checks and PRs. Only this repository's `develop`
may target `main`. Release metadata and version preparation follow that same path. The
internal deployment coordinator can continue automatically when required checks pass;
this policy adds no reviewer wait or extra confirmation to that flow.

## Release evidence

The Release workflow consumes the actual results of its reusable CI jobs, verifies the
five packaged archives against `checksums.txt`, and emits `quality-report.json` and
`quality-report.md` into the release artifact. Tagged releases attach the same reports to
the draft before publication. The reports contain the exact commit, plugin version,
workflow run URL, job results and archive hashes. The Markdown report also appears in the
workflow summary. A manual rehearsal produces evidence without publishing a release.

Failure to validate evidence stops artifact publication. Reports are CI evidence, not signed
attestations. They do not attest to code review or include live environment acceptance.
See [verification](docs/verification.md) and [release procedures](docs/release.md).
