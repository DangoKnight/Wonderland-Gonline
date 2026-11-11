package main

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAssetPackNames(t *testing.T) {
	for _, test := range []struct {
		name, manifest string
		fail           bool
	}{
		{"standalone", "", false},
		{"manifest", `{"version":1,"packs":["world.zip"]}`, false},
		{"traversal", `{"version":1,"packs":["../private.zip"]}`, true},
		{"duplicate", `{"version":1,"packs":["world.zip","world.zip"]}`, true},
		{"core duplicate", `{"version":1,"packs":["client-assets.zip"]}`, true},
		{"missing", `{"version":1,"packs":["missing.zip"]}`, true},
		{"version", `{"version":2,"packs":[]}`, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			f, err := os.Create(filepath.Join(dir, coreAssetPack))
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			if test.manifest != "" {
				w, err := z.Create("_packs.json")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := w.Write([]byte(test.manifest)); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()
			for _, name := range []string{"world.zip", "old-world.zip", "private.txt"} {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			names, err := assetPackNames(dir)
			if test.fail {
				if err == nil {
					t.Fatal("accepted invalid manifest")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := []string{coreAssetPack}
			if test.manifest != "" {
				want = append(want, "world.zip")
			}
			if !reflect.DeepEqual(names, want) {
				t.Fatalf("got %v want %v", names, want)
			}
		})
	}
}

func TestAssetHTTPRangesAndAllowlist(t *testing.T) {
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, coreAssetPack))
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	w, err := z.Create("_packs.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(`{"version":1,"packs":["world.zip"]}`)); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	for _, name := range []string{"world.zip", "stale.zip", "private.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("0123456789"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	handler, err := assetHandler(dir, "127.0.0.1:6414")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method, path, rangeHeader string
		status                    int
		body                      string
	}{
		{http.MethodHead, "/assets/world.zip", "", http.StatusOK, ""},
		{http.MethodGet, "/assets/world.zip", "bytes=2-5", http.StatusPartialContent, "2345"},
		{http.MethodGet, "/assets/world.zip", "bytes=99-100", http.StatusRequestedRangeNotSatisfiable, ""},
		{http.MethodGet, "/assets/stale.zip", "", http.StatusNotFound, ""},
		{http.MethodGet, "/assets/private.txt", "", http.StatusNotFound, ""},
		{http.MethodGet, "/assets/", "", http.StatusNotFound, ""},
	} {
		r := httptest.NewRequest(test.method, test.path, nil)
		if test.rangeHeader != "" {
			r.Header.Set("Range", test.rangeHeader)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != test.status {
			t.Fatalf("%s %s: status %d", test.method, test.path, response.Code)
		}
		if test.status < http.StatusBadRequest && response.Body.String() != test.body {
			t.Fatalf("%s %s: unexpected body %q", test.method, test.path, response.Body.String())
		}
		if test.method == http.MethodHead && response.Header().Get("Content-Length") != "10" {
			t.Fatal("HEAD did not expose the pack size")
		}
		if test.status == http.StatusPartialContent && response.Header().Get("Content-Range") != "bytes 2-5/10" {
			t.Fatal("range boundaries are not verifiable")
		}
	}
}
