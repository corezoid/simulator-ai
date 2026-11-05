.PHONY: generate gen-check test vet build

# gen-cdu-reference reads sim-api and simulator-ui-kit (SIM_API_DIR / UI_KIT_DIR, default: the
# checkouts next to this one) at origin/develop; its -check regenerates at the commits recorded in
# the documents and is skipped where those checkouts are not available.
generate:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-cdu-reference

gen-check:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client -check
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-cdu-reference -check

test: check-source gen-check
	GOMAXPROCS=2 go test -p=1 -timeout=60s ./...

vet:
	GOMAXPROCS=2 go vet -p=1 ./...

build:
	GOMAXPROCS=2 go build -p=1 -o bin/simulator-ai ./cmd/simulator-ai

.PHONY: check-source
check-source:
	GOMAXPROCS=2 go run -p=1 ./cmd/check-source

.PHONY: client-smoke
client-smoke: build
	GOMAXPROCS=2 go run -p=1 ./cmd/client-smoke

# Live console smoke against a local sim-api stand. sim-api's ai-smoke-env target writes
# SMOKE_ENV (origin, workspace, API key or Authorization); it replaces any SIMULATOR_AI_*
# credential of the calling shell and is loaded only into this run.
SMOKE_ENV ?= $(HOME)/.config/simulator-ai/smoke.env

.PHONY: smoke
smoke: build
	@test -n "$(SIM_API_DIR)" || { echo "SIM_API_DIR=<sim-api checkout> required"; exit 2; }
	$(MAKE) -C "$(SIM_API_DIR)" ai-smoke-env $(if $(WORKSPACE),WORKSPACE=$(WORKSPACE))
	@unset SIMULATOR_AI_API_KEY SIMULATOR_AI_AUTHORIZATION SIMULATOR_AI_DELEGATED_TOKEN SIMULATOR_AI_ACTOR SIMULATOR_AI_PROFILE && \
		set -a && . "$(SMOKE_ENV)" && set +a && GOMAXPROCS=2 go run -p=1 ./cmd/console-smoke -bin bin/simulator-ai graph form registry core && \
		GOMAXPROCS=2 go run -p=1 ./cmd/console-smoke -bin bin/simulator-ai -profile console-full parity people finance agents simulation charts formops extras jobs meetings
