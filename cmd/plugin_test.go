package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var testWasm = []byte("\x00asm\x01\x00\x00\x00")

func sha(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestInstallWasmDirectVerifiesChecksum(t *testing.T) {
	body := testWasm
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "x.wasm")

	if err := installWasmDirect(srv.Client(), srv.URL, dest, sha(testWasm)); err != nil {
		t.Fatalf("matching checksum: %v", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != string(testWasm) {
		t.Fatal("installed file content mismatch")
	}

	// A tampered download must not replace the good file.
	body = append([]byte(nil), testWasm...)
	body = append(body, 0xff)
	err := installWasmDirect(srv.Client(), srv.URL, dest, sha(testWasm))
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered download err = %v, want checksum mismatch", err)
	}
	if got, _ := os.ReadFile(dest); string(got) != string(testWasm) {
		t.Fatal("tampered download replaced the installed file")
	}
	if entries, _ := os.ReadDir(filepath.Dir(dest)); len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}

func TestWriteWasmAtomicallyRejectsOversizeAndNonWasm(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeWasmAtomically(strings.NewReader(string(testWasm)+"padding"), filepath.Join(dir, "big.wasm"), "", 8); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize err = %v", err)
	}
	if _, err := writeWasmAtomically(strings.NewReader("#!/bin/sh\necho hi\n"), filepath.Join(dir, "sh.wasm"), "", 1024); err == nil ||
		!strings.Contains(err.Error(), "not a WebAssembly module") {
		t.Fatalf("non-wasm err = %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("failed installs left %d files", len(entries))
	}
}

func TestRuntimeInstallersArePinned(t *testing.T) {
	for name, r := range runtimeInstallers {
		if len(r.sha256) != 64 {
			t.Errorf("%s: missing or malformed sha256 pin", name)
		}
	}
}
