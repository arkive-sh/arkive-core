.PHONY: dev build build-opaque opaque-setup verify test lint deadcode vulncheck gosec

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
	@CGO_ENABLED=0 go test ./...

lint:
	@golangci-lint run ./...

deadcode:
	@deadcode .

vulncheck:
	@govulncheck ./...

gosec:
	@gosec ./...
