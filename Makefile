BINARY     := memsh
INSTALL    := /usr/local/bin/$(BINARY)
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -ldflags "-s -w -X main.version=$(VERSION)"

PLUGIN_SRCS := $(wildcard plugins/*/main.go)
PLUGIN_WASM := $(patsubst plugins/%/main.go,shell/plugins/%.wasm,$(PLUGIN_SRCS))

.PHONY: all build install plugins clean test cover lint

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
