.PHONY: build clean install lint lint-lua test test-e2e test-race validate

FILE ?= internal/testdata/add.go
DRAFTSMAN_RUN := uv run --locked python
LUACHECK := $(CURDIR)/build/tools/luacheck/bin/luacheck

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X github.com/siutsin/gofactos/internal/app.version=$(VERSION) \
	-X github.com/siutsin/gofactos/internal/app.buildTime=$(BUILD_TIME)

build:
	mkdir -p build
	go build -ldflags "$(LDFLAGS)" -o build/gofactos .

install: build
	mkdir -p "$(HOME)/.local/bin"
	ln -sfn "$(CURDIR)/build/gofactos" "$(HOME)/.local/bin/gofactos"

test: lint
	GOFACTOS_FACTORIO_E2E=0 \
		go test -cover -shuffle=on ./internal/...

test-race: lint
	GOFACTOS_FACTORIO_E2E=0 \
		go test -race -shuffle=on ./internal/...

test-e2e: build
	GOFACTOS_FACTORIO_E2E=1 \
		GOFACTOS_BIN="$(CURDIR)/build/gofactos" \
		go test -count=1 -timeout=5m -v ./internal/e2e

lint: lint-lua
	go fmt . ./internal/...
	go fmt demo/loop.go
	golangci-lint run . ./internal/...
	golangci-lint run --build-tags ignore ./demo/...
	markdownlint-cli2 "**/*.md"

lint-lua:
	@test -x "$(LUACHECK)" || { \
		echo "luacheck not installed; run: mise install"; \
		exit 1; \
	}
	"$(LUACHECK)" internal/e2e/testdata

validate:
	go run . blueprint $(FILE) | $(DRAFTSMAN_RUN) -c "\
	import sys; \
	from draftsman.blueprintable import get_blueprintable_from_string; \
	bp = get_blueprintable_from_string(sys.stdin.read().strip()); \
	[print(f'{e.name} at {e.position} - {e.to_dict()}') for e in bp.entities]; \
	print('Wires:', bp.wires)"

clean:
	rm -rf build/
