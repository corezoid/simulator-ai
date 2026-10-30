.PHONY: generate gen-check test vet build

generate:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client

gen-check:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client -check

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
