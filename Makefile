.PHONY: build run test lint generate dashboard

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

dashboard:
	cd dashboard && pnpm install --frozen-lockfile && pnpm build
	rm -rf internal/api/static/dist
	mkdir -p internal/api/static/dist
	cp -R dashboard/dist/. internal/api/static/dist/
	touch internal/api/static/dist/.gitkeep
