.PHONY: dev build build-opaque opaque-setup verify test test-integration lint deadcode vulncheck gosec

build-opaque:
	@cargo build --release --locked --manifest-path rust/opaque-ffi/Cargo.toml

opaque-setup: build-opaque
	@mkdir -p ./bin
	@CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/opaque-setup ./cmd/opaque-setup
	@cp rust/opaque-ffi/target/release/libarkive_opaque.so ./bin/
	@./bin/opaque-setup

build: build-opaque
	@mkdir -p ./bin
	@CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o ./bin/arkive .
	@cp rust/opaque-ffi/target/release/libarkive_opaque.so ./bin/

dev:
	@command -v CompileDaemon >/dev/null 2>&1 || (echo "CompileDaemon not found. Run: go install github.com/githubnemo/CompileDaemon@latest" && exit 1)
	@CompileDaemon -build="make build" -command="./bin/arkive" -pattern=".+\\.(go|rs|toml|lock|css|js|svg)$$" -exclude-dir=".git" -exclude-dir="target" -exclude-dir="node_modules" -exclude-dir=".svelte-kit"

verify: test lint deadcode vulncheck gosec

test:
	@cargo test --locked --manifest-path rust/opaque-ffi/Cargo.toml
	@$(MAKE) build-opaque
	@CGO_ENABLED=0 go test -short ./...
	@cd web && npm run test:unit

test-integration: build-opaque
	@test -n "$(TEST_DATABASE_URL)" || (echo "Set TEST_DATABASE_URL to a disposable PostgreSQL database" && exit 1)
	@command -v node >/dev/null 2>&1 || (echo "Install Node.js to run OPAQUE auth integration tests" && exit 1)
	@test -d web/node_modules/@serenity-kit/opaque || (echo "Run npm install in web before OPAQUE auth integration tests" && exit 1)
	@TEST_DATABASE_URL="$(TEST_DATABASE_URL)" CGO_ENABLED=0 go test -v -tags=integration ./...

lint:
	@golangci-lint run ./...

deadcode:
	@deadcode -test -tags=integration ./...

vulncheck:
	@govulncheck ./...

gosec:
	@gosec -exclude-dir=web ./...
