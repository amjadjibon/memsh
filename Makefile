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
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG parameter is required. Usage: make release TAG=v1.0.0"; \
		exit 1; \
	fi
	@echo "Creating release tag: $(TAG)"
	@if git rev-parse $(TAG) >/dev/null 2>&1; then \
		echo "Error: Tag $(TAG) already exists. Delete it first with: git tag -d $(TAG) && git push --delete origin $(TAG)"; \
		exit 1; \
	fi
	@echo "Checking for uncommitted changes..."
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Found uncommitted changes. Committing and pushing..."; \
		git add -A; \
		git commit -m "chore: prepare for release $(TAG)"; \
		echo "Pushing changes to origin..."; \
		git push origin main; \
		echo "✅ Changes pushed to repository"; \
	else \
		echo "Working directory is clean"; \
	fi
	@echo "Cleaning dist directory..."
	$(MAKE) clean
	rm -rf dist/
	@echo "Creating tag $(TAG)..."
	git tag -a $(TAG) -m "Release $(TAG)"
	@echo "Pushing tag to origin..."
	git push origin $(TAG)
	@echo "Running goreleaser..."
	goreleaser release --clean
	@echo "Updating homebrew-memsh repository..."
	@mkdir -p homebrew-memsh/Casks
	@cp dist/homebrew/Casks/memsh.rb homebrew-memsh/Casks/
	@cd homebrew-memsh && \
		if [ -n "$$(git status --porcelain Casks/memsh.rb)" ]; then \
			git add Casks/memsh.rb; \
			git commit -m "Update memsh cask to $(TAG)"; \
			git push origin main; \
			echo "✅ Homebrew cask pushed to repository"; \
		else \
			echo "ℹ️  Homebrew cask already up to date"; \
		fi
	@echo ""
	@echo "✅ Release $(TAG) created successfully!"
	@echo "📦 GitHub Release: https://github.com/amjadjibon/memsh/releases/tag/$(TAG)"
	@echo "🍺 Homebrew cask updated in: homebrew-memsh/Casks/memsh.rb"

release-dry-run:
	@if [ -z "$(TAG)" ]; then \
		echo "Error: TAG parameter is required. Usage: make release-dry-run TAG=v1.0.0"; \
		exit 1; \
	fi
	@echo "🧪 Dry-run release for tag: $(TAG)"
	@echo "Cleaning dist directory..."
	rm -rf dist/
	@echo "Skipping git tag creation and push..."
	@echo "Running goreleaser in test mode..."
	goreleaser release --skip=publish,validate --clean
	@echo ""
	@echo "✅ Dry-run complete! Check dist/ directory for generated artifacts."
	@echo "📄 Generated cask: dist/homebrew/Casks/memsh.rb"
	@echo ""
	@echo "📋 What would happen on full release:"
	@echo "   • Uncommitted changes would be committed and pushed"
	@echo "   • Git tag $(TAG) would be created and pushed"
	@echo "   • GitHub release would be published"
	@echo "   • Homebrew cask would be copied to homebrew-memsh/Casks/memsh.rb"
	@echo "   • Cask would be committed and pushed to homebrew-memsh repository"

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
	@echo "  make release TAG=v1.0.0      - Create release (commits, pushes, updates tap)"
	@echo "  make release-dry-run TAG=v1.0.0  - Test goreleaser without pushing"
	@echo "  make help             - Show this help message"
	@echo ""
	@echo "Release workflow:"
	@echo "  1. make release-dry-run TAG=v1.0.0   # Test first"
	@echo "  2. make release TAG=v1.0.0            # Create release"
	@echo ""
	@echo "What make release does:"
	@echo "  • Commits and pushes any uncommitted changes"
	@echo "  • Cleans dist/ and build artifacts"
	@echo "  • Creates and pushes git tag"
	@echo "  • Runs goreleaser (builds + GitHub release)"
	@echo "  • Copies cask to homebrew-memsh/Casks/"
	@echo "  • Commits and pushes cask to tap repository"
	@echo ""
	@echo "Homebrew tap: https://github.com/amjadjibon/homebrew-memsh"
