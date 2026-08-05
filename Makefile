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
	cd dashboard && pnpm install --frozen-lockfile && pnpm build --filter=@vben/web-antdv-next
	mkdir -p assets/dashboard
	touch assets/dashboard/.gitkeep
	find assets/dashboard -mindepth 1 ! -name '.gitkeep' -delete
	cp -R dashboard/apps/web-antdv-next/dist/. assets/dashboard/
