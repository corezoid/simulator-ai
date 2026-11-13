# Releases and compatibility

Two permanent branches: develop for integration/dev; main freezes the pre candidate.
Feature/bugfix PRs target develop. Temporary release/* or hotfix/* branches are allowed.
The release tag is vX.Y.Z; no environment branches and no per-PR version bumps.

## Checks and promotion

1. CI runs on PRs and develop/main pushes: formatting, go mod tidy/verify, source limits,
   self-contained generation, golangci-lint, tests/coverage, race, govulncheck and redacted
   Gitleaks history scanning. All jobs must succeed for the required Gate. Actions are SHA-pinned.
2. The release automation verifies develop and main by exact SHA. A temporary release PR
   into develop synchronizes the main baseline and prepares version/changelog metadata.
   After its checks and merge, only a develop → main PR can promote the candidate.
   The final main SHA is checked again. No production tag or public release is created.
3. Pre consumes that exact commit for both binary and skills. Test backend compatibility
   with supported old desktop plugins, the candidate, and the previous backend for rollback.
4. Production verifies the pre candidate before backend rollout. Afterwards the release automation refuses
   publication if main moved, verifies its already-prepared version, tags it and publishes the
   checked draft. It never commits metadata to main or incorporates later develop changes.
5. Release workflow rechecks the tag, main ancestry and version equality, then builds Linux/macOS amd64/arm64 archives and a Windows amd64 ZIP containing the platform executable plus manifests/skills, SHA256 checksums and
   Go build dependency information. It stages a draft whose notes are the tag's `## [X.Y.Z]` section of
   CHANGELOG.md (generated notes only if that section is missing or empty); the release automation
   publishes only after success.

The project is MIT-licensed. Archives include the project license, dependency notices and
per-platform BUILD_INFO.txt. Issuer-revoke and environment acceptance remain separate live gates.
Build-info is a dependency inventory, not a signed SBOM/provenance attestation. Add those only
when publication requirements need them.

## Deployment boundary

GitHub CI and main promotion verify code and artifacts, not a live deployment. The deployment
owner pins the candidate SHA/digest for both executable and skills, waits for all replicas,
and collects live ACL/revoke/write-recovery evidence. A single public schema response cannot
certify a mixed fleet. No automatic live writes or paid model evaluation run in public PR CI.

Keep API origin/workspace/credentials explicit and separate per environment. Updating a plugin
must not switch the API target. Installed desktop copies remain after a release; retain supported
contracts for at least four weekly cycles, and announce breaking changes before removal.
The strict Smart Forms/graph-create preflights still reject changed parameter schemas: test old
released binaries before changing those methods; do not silently loosen this safety boundary.

## Local checks and generated references

Use the commands in [verification](verification.md). `make gen-check` checks committed
contracts, CDU references and the generated tool catalog. Normal contributors need no
private checkout or deployment credentials. Source-refresh instructions are in AGENTS.md.

## Rollback and recovery

Keep previous environment SHAs/digests. Restore a known compatible backend/plugin pair only
when database migrations permit it. Do not rewrite published tags/assets. Desktop publication
cannot roll back already installed binaries or undo API writes. Follow each profile's receipts,
CAS and readback rules; Console retries have different backend-owned semantics.

A failed or missing workflow stops promotion. Fix code/configuration rather than bypassing Gate.
Release draft creation fails closed on an existing draft: inspect its exact SHA/checksums before
resuming; never overwrite public assets to disguise a failed release.

## Archive rehearsal and protected branches

`make package` creates all five release archives from a clean committed checkout; use
`make package PLATFORMS=darwin/arm64` for one local platform. CI packages Linux and Windows amd64,
verifies their checksums, extracts them and checks the executables.
The manual `Release` workflow builds all five platforms on a commit contained in main without
creating a tag or a GitHub release. Download its artifact, run `shasum -a 256 -c checksums.txt`,
extract the matching platform and run `bin/simulator-ai --version` before client acceptance.
Restore the previous archive directory to roll back binary, manifests and skills together.

Both permanent branches require PRs and strict Gate checks, including administrators.
Main receives only develop PRs; the release automation prepares metadata through temporary PRs into develop, then promotes develop.
The required Develop only check runs on `pull_request` with no PR checkout and no token: GitHub's default policy for
public repositories blocks `pull_request_target` (it failed to start during the 2026-11-09 brownout). Its workflow file
comes from the PR's merge commit, so it guards against wrong-source promotions, not a hostile PR: a PR that edits
.github/workflows needs a careful look before merging, and a PR with a merge conflict gets no run until it is resolved.
The release automation deletes successful release branches and leaves failed ones for inspection. Do not disable protection for weekly releases.

Main accepts only same-repository develop PRs; hotfixes and release metadata go through develop.

Windows amd64 ships as ZIP with bin/simulator-ai.exe. Required CI downloads that same ZIP
and verifies its checksum, extraction, version and MCP initialize/tools-list on a Windows runner.
Windows client-install acceptance is separate from this executable/protocol check.

## Quality evidence

[QUALITY.md](../QUALITY.md) defines the repository's required checks. After its reusable CI
and packaging succeed, Release verifies all five archives against checksums.txt and writes
quality-report.json and quality-report.md into dist. Reports are uploaded with the archives,
attached to tagged release drafts and shown in the workflow summary. Missing/failed CI
results, missing archives, duplicate checksum entries or mismatched hashes stop publication.

The report identifies the exact commit, version and workflow run. Review is recorded as
not performed by this workflow; no numeric rating or live-deployment claim is generated.
No new review approval, model credential or paid AI job is required by the release flow.
