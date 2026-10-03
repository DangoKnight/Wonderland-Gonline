package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectDataLogin(t *testing.T) {
	root := os.Getenv("WONDERLAND_TEST_DATA_ROOT")
	if root == "" {
		t.Skip("set WONDERLAND_TEST_DATA_ROOT to source-derived data")
	}
	c, err := New(Options{Root: root, ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Pics.Missing) != 0 {
		t.Fatalf("missing PNG skin files: %v", c.Pics.Missing)
	}
	if len(c.Pics.Entries) == 0 || c.background == nil || c.Servers.Background == nil {
		t.Fatal("decoded login artwork not loaded")
	}
}
