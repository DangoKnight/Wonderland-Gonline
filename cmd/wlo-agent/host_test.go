package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Source builds must embed a complete distributable without npm or a CDN.
func TestEmbeddedVueHostBundle(t *testing.T) {
	index, err := hostFS.ReadFile("host/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range []string{"/ui/host.js", "/ui/host.css", "/wasm_exec.js"} {
		if !bytes.Contains(index, []byte(reference)) {
			t.Fatalf("missing embedded entry %s; run npm run agent-host:build", reference)
		}
		asset, err := hostFS.ReadFile("host" + reference)
		if err != nil || len(asset) == 0 {
			t.Fatalf("embedded asset %s: %v", reference, err)
		}
	}
	if bytes.Contains(index, []byte("/src/")) {
		t.Fatal("host embeds development source instead of the production bundle")
	}
	for _, match := range regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllSubmatch(index, -1) {
		if !strings.HasPrefix(string(match[1]), "/") || strings.HasPrefix(string(match[1]), "//") {
			t.Fatalf("nonlocal frontend dependency: %s", match[1])
		}
	}
	runtime, err := hostFS.ReadFile("host/wasm_exec.js")
	if err != nil {
		t.Fatal(err)
	}
	upstream, err := os.ReadFile("frontend/public/wasm_exec.js")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runtime, upstream) {
		t.Fatal("bundled Go runtime differs from its upstream source; rebuild agent host")
	}
}
