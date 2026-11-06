# Weekly release through sim-deploy

Two permanent branches: develop for integration/dev; main freezes the pre candidate.
Feature/bugfix PRs target develop. Temporary release/* or hotfix/* branches are allowed.
The release tag is vX.Y.Z; no environment branches and no per-PR version bumps.

## Checks and promotion

1. CI runs on PRs and develop/main pushes: formatting, go mod tidy/verify, source limits,
   self-contained generation, golangci-lint, tests/coverage, race, govulncheck and redacted
   Gitleaks history scanning. All jobs must succeed for the required Gate. Actions are SHA-pinned.
2. sim-deploy prerelease verifies develop and main by exact SHA. A temporary release PR
   into develop synchronizes the main baseline and prepares version/changelog metadata.
   After its checks and merge, only a develop → main PR can promote the candidate.
   The final main SHA is checked again. No production tag or public release is created.
3. Pre consumes that exact commit for both binary and skills. Test backend compatibility
   with supported old desktop plugins, the candidate, and the previous backend for rollback.
4. Production verifies the pre candidate before backend rollout. Afterwards sim-deploy refuses
   publication if main moved, verifies its already-prepared version, tags it and publishes the
   checked draft. It never commits metadata to main or incorporates later develop changes.
5. Release workflow rechecks the tag, main ancestry and version equality, then builds Linux/macOS
   amd64/arm64 archives containing bin/simulator-ai plus manifests/skills, SHA256 checksums and
   Go build dependency information. It stages a draft; sim-deploy publishes only after success.

The project is MIT-licensed. Archives include the project license, dependency notices and
per-platform BUILD_INFO.txt. Issuer-revoke and environment acceptance remain separate live gates.
Build-info is a dependency inventory, not a signed SBOM/provenance attestation. Add those only
when publication requirements need them, rather than claiming they exist in this first slice.

## Deployment boundary

GitHub CI and main promotion verify code and artifacts, not a live deployment. The current
Hermes harness in sim-api builds binary and skills from HERMES_SIMULATOR_AI_REF. The environment
owner must pin the emitted candidate SHA/digest in its deployment config, wait for all replicas,
and collect live ACL/revoke/write-recovery evidence. A single public schema response cannot
certify a mixed fleet. No automatic live writes or paid model evaluation run in public PR CI.

Keep API origin/workspace/credentials explicit and separate per environment. Updating a plugin
must not switch the API target. Installed desktop copies remain after a release; retain supported
contracts for at least four weekly cycles, and announce breaking changes before removal.
The strict Smart Forms/graph-create preflights still reject changed parameter schemas: test old
released binaries before changing those methods; do not silently loosen this safety boundary.

## Local checks and generated reference

Go 1.27.3 and golangci-lint 2.14.0. `make check` runs sequentially; use GOMAXPROCS=2 and -p=1.
CI adds `make test-race`, `make test-cover`, and `make security`. Coverage baselines are 86% for
mcpserver and 73% for publicapi/simulation; manual live-smoke CLIs and third-party PDF code are
not hidden inside these numbers. The full coverage profile is retained as a CI artifact. Wall-clock budgets run in Quality;
race instrumentation retains functional/allocation checks without applying uninstrumented timings.

`make gen-check` reproduces embedded CDU documents from inputs.json, a generated public
projection of the source contracts/help/examples. It never skips absent inputs. When refreshing,
run `make generate` with SIM_API_DIR and UI_KIT_DIR, then `make gen-source-check`: this strictly
compares the input projection and output at the commits recorded in the documents and fails if
sources are missing. Fork CI needs no private checkout or personal token. inputs.json is not
embedded into the runtime binary.

## Rollback and recovery

Keep previous environment SHAs/digests. Restore a known compatible backend/plugin pair only
when database migrations permit it. Do not rewrite published tags/assets. Desktop publication
cannot roll back already installed binaries or undo API writes. Follow each profile's receipts,
CAS and readback rules; Console retries have different backend-owned semantics.

A failed or missing workflow stops promotion. Fix code/configuration rather than bypassing Gate.
Release draft creation fails closed on an existing draft: inspect its exact SHA/checksums before
resuming; never overwrite public assets to disguise a failed release.

## Review evidence

Every change ends with independent xhigh review, fixes, then review of the exact final diff/SHA.
Record local commands, hosted pipeline links, source revisions, and any remaining live gates in
its PR. Do not run the broad sim-deploy CLI as a test: it can notify people and deploy other repos.

## Archive rehearsal and protected branches

`make package` creates all four release archives from a clean committed checkout; use
`make package PLATFORMS=darwin/arm64` for one local platform. CI packages Linux amd64,
verifies its checksum, extracts it and compares its version with the built executable.
The manual `Release` workflow builds all platforms on a commit contained in main without
creating a tag or a GitHub release. Download its artifact, run `shasum -a 256 -c checksums.txt`,
extract the matching platform and run `bin/simulator-ai --version` before client acceptance.
Restore the previous archive directory to roll back binary, manifests and skills together.

Both permanent branches require PRs and strict Gate checks, including administrators.
Main receives only develop PRs; sim-deploy prepares metadata through temporary PRs into develop, then promotes develop.
The required Develop only check runs trusted default-branch code with no PR checkout. It deletes successful release
branches and leaves failed ones for inspection. Do not disable protection for weekly releases.

Main accepts only same-repository develop PRs; hotfixes and release metadata go through develop.
