#!/usr/bin/env bash
# install-python.sh — download the Python 3.12 WASI binary into ~/.memsh/plugins/

set -e

PLUGIN_DIR="$HOME/.memsh/plugins"
DEST="$PLUGIN_DIR/python.wasm"
URL="https://github.com/vmware-labs/webassembly-language-runtimes/releases/download/python%2F3.12.0%2B20231211-040d5a6/python-3.12.0.wasm"

mkdir -p "$PLUGIN_DIR"

if [ -f "$DEST" ]; then
  echo "python.wasm already installed at $DEST"
  exit 0
fi

echo "Downloading Python 3.12 WASI → $DEST ..."
curl -fSL "$URL" -o "$DEST"
echo "Done. Start memsh and run: python -c \"print('hello')\""
