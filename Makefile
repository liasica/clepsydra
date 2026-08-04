.PHONY: build run test lint generate

build:
	go build -o bin/clepsydra ./cmd/clepsydra

run:
	go run ./cmd/clepsydra -c configs/config.yaml

test:
	go test ./... -count=1

lint:
	gclint run --config .golangci.yml --timeout=10m

generate:
	go generate ./internal/ent/...
