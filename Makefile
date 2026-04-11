BINARY     := memsh
INSTALL    := /usr/local/bin/$(BINARY)
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-s -w -X main.version=$(VERSION)"

PLUGIN_SRCS := $(wildcard plugins/*/main.go)
PLUGIN_WASM := $(patsubst plugins/%/main.go,shell/plugins/%.wasm,$(PLUGIN_SRCS))

.PHONY: all build install plugins clean test cover lint release release-dry-run help

all: build plugins

build:
	mkdir -p ./bin
	go build $(LDFLAGS) -o ./bin/$(BINARY)

install: build
	sudo mv ./bin/${BINARY} ${INSTALL}

plugins: $(PLUGIN_WASM)

shell/plugins/%.wasm: plugins/%/main.go
	GOOS=wasip1 GOARCH=wasm go build -o $@ ./$(<D)

clean:
	rm -f ./bin/$(BINARY) shell/plugins/*.wasm

test:
	go test ./... -v -count=1

cover:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint:
	go vet -stdmethods=false ./...
	@echo "vet passed"

release:
	@./scripts/release.sh --tag $(TAG)

release-dry-run:
	@./scripts/release.sh --tag $(TAG) --dry-run

help:
	@echo "memsh - Available commands:"
	@echo ""
	@echo "  make all              - Build binaries and plugins"
	@echo "  make build            - Build binary"
	@echo "  make install          - Build and install to $(INSTALL)"
	@echo "  make plugins          - Build WASM plugins"
	@echo "  make clean            - Remove build artifacts"
	@echo "  make test             - Run tests"
	@echo "  make cover            - Generate coverage report"
	@echo "  make lint             - Run go vet"
	@echo "  make release TAG=v1.0.0      - Create release (uses scripts/release.sh)"
	@echo "  make release-dry-run TAG=v1.0.0  - Test release without pushing"
	@echo "  make help             - Show this help message"
	@echo ""
	@echo "Release workflow:"
	@echo "  1. make release-dry-run TAG=v1.0.0   # Test first"
	@echo "  2. make release TAG=v1.0.0            # Create release"
	@echo ""
	@echo "Or use the script directly:"
	@echo "  ./scripts/release.sh --tag v1.0.0"
	@echo "  ./scripts/release.sh --tag v1.0.0 --dry-run"
	@echo ""
	@echo "What the release script does:"
	@echo "  • Validates working directory state"
	@echo "  • Commits and pushes any uncommitted changes"
	@echo "  • Cleans dist/ and build artifacts"
	@echo "  • Creates and pushes git tag"
	@echo "  • Runs goreleaser (builds, signs, creates release)"
	@echo "  • Updates homebrew-memsh submodule reference"
	@echo ""
	@echo "Homebrew tap: https://github.com/amjadjibon/homebrew-memsh"
