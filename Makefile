.PHONY: generate gen-check test vet build

generate:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client

gen-check:
	GOMAXPROCS=2 go run -p=1 ./cmd/gen-client -check

test: gen-check
	GOMAXPROCS=2 go test -p=1 -timeout=60s ./...

vet:
	GOMAXPROCS=2 go vet -p=1 ./...

build:
	GOMAXPROCS=2 go build -p=1 -o bin/simulator-ai ./cmd/simulator-ai
