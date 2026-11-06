# Weekly release through sim-deploy

Two permanent branches: develop for integration/dev; main freezes the pre candidate.
Feature/bugfix PRs target develop. Temporary release/* or hotfix/* branches are allowed.
The release tag is vX.Y.Z; no environment branches and no per-PR version bumps.

## Checks and promotion

1. CI runs on PRs and develop/main pushes: formatting, go mod tidy/verify, source limits,
   self-contained generation, golangci-lint, tests/coverage, race, govulncheck and redacted
   Gitleaks history scanning. All jobs must succeed for the required Gate. Actions are SHA-pinned.
2. sim-deploy prerelease resolves develop to a full SHA, verifies its CI, promotes that SHA
   into main, and checks the resulting SHA. No production tag or public release is created.
3. Pre must consume that exact commit for both binary and skills. Test backend compatibility
   with supported old desktop plugins, the candidate, and the previous backend for rollback.
4. sim-deploy verifies the pre candidate before backend production rollout. After backend
   success it refuses publication if the candidate moved, mints version/changelog metadata,
   waits for that final commit's CI, and creates the tag. It does not merge a later develop tip.
5. Release workflow rechecks the tag, main ancestry and version equality, then builds Linux/macOS
   amd64/arm64 archives containing bin/simulator-ai plus manifests/skills, SHA256 checksums and
   Go build dependency information. It stages a draft; sim-deploy publishes only after success.

Production is not part of the initial CI/dev/pre adoption. No license is invented: owner choice
and the documented live issuer-revoke acceptance remain prerequisites for broad public rollout.
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
not hidden inside these numbers. The full coverage profile is retained as a CI artifact.

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
