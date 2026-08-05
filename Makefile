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
	rm -rf assets/dashboard
	mkdir -p assets/dashboard
	cp -R dashboard/dist/. assets/dashboard/
	touch assets/dashboard/.gitkeep
