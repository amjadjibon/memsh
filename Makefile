BINARY     := memsh
INSTALL    := /usr/local/bin/$(BINARY)

PLUGIN_SRCS := $(wildcard plugins/*/main.go)
PLUGIN_WASM := $(patsubst plugins/%/main.go,shell/plugins/%.wasm,$(PLUGIN_SRCS))

.PHONY: all build install plugins clean

all: build plugins

build:
	mkdir -p ./bin
	go build -o ./bin/$(BINARY)

install: build
	sudo mv ./bin/${BINARY} ${INSTALL}
	

plugins: $(PLUGIN_WASM)

shell/plugins/%.wasm: plugins/%/main.go
	GOOS=wasip1 GOARCH=wasm go build -o $@ ./$(<D)

clean:
	rm -f ./bin/$(BINARY) shell/plugins/*.wasm
