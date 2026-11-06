package clientbundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/localization"
)

func TestBundleBakesTranslationsWithoutRuntimeOverrideTables(t *testing.T) {
	for _, optimized := range []bool{false, true} {
		o, src := fixture(t)
		raw := []byte(`{"records":[{"fields":{"id":57166},"text":{"text":"Breillat*"},"decoded_hex":"00ff"}]}`)
		put(t, filepath.Join(src, "talk_data.json"), raw)
		os.MkdirAll(filepath.Join(src, "runtime", "text"), 0755)
		put(t, filepath.Join(src, "runtime", "text", "talk_data.json"), []byte(`{"stale":"must not ship"}`))
		build := Build
		if optimized {
			build = BuildRuntime
		}
		if _, err := build(o); err != nil {
			t.Fatal(err)
		}
		if _, err := build(o); err != nil {
			t.Fatal("repeat build", err)
		}
		root, closePacks, err := clientfs.Mount(o.Output)
		if err != nil {
			t.Fatal(err)
		}
		defer closePacks()
		b, err := clientfs.ReadFile(filepath.Join(root, "talk_data.json"))
		if err != nil || !bytes.Contains(b, []byte(`"text":"@{talk.57166.text}"`)) || !bytes.Contains(b, []byte(`"decoded_hex":"00ff"`)) {
			t.Fatal("bundle lacks baked text", string(b), err)
		}
		for _, lang := range []string{localization.English, localization.Chinese} {
			c, err := localization.Load(root, lang)
			if err != nil {
				t.Fatal(err)
			}
			if c.Lookup("talk.57166.text") == "talk.57166.text" {
				t.Fatal("missing packaged catalog", lang)
			}
		}
		if _, err := clientfs.Stat(filepath.Join(root, "runtime", "text", "talk_data.json")); !os.IsNotExist(err) {
			t.Fatal("stale loose text projection packaged", err)
		}
		closePacks()
		b, _ = os.ReadFile(filepath.Join(src, "talk_data.json"))
		if !bytes.Equal(b, raw) {
			t.Fatal("compiler modified original source")
		}
	}
}

func TestLegacyBundleTextCacheIsInvalidated(t *testing.T) {
	o, src := fixture(t)
	raw := []byte(`{"records":[{"fields":{"id":57166},"text":{"text":"Breillat*"}}]}`)
	put(t, filepath.Join(src, "talk_data.json"), raw)
	// An older bundle has the same extraction hash, but no compiled-text
	// fingerprint. It must not reuse the untranslated compressed entry.
	f, err := os.Create(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, _ := z.Create("talk_data.json")
	entry.Write(raw)
	old := Manifest{Version: Version, Files: map[string]Entry{"talk_data.json": {SourceSHA256: digest(raw), Bytes: int64(len(raw))}}}
	b, _ := json.Marshal(old)
	entry, _ = z.Create(ManifestName)
	entry.Write(b)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	r, err := Build(o)
	if err != nil || r.Reused != 0 {
		t.Fatal("legacy untranslated entry reused", r, err)
	}
	root, closeBundle, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	b, err = clientfs.ReadFile(filepath.Join(root, "talk_data.json"))
	if err != nil || !bytes.Contains(b, []byte(`"text":"@{talk.57166.text}"`)) {
		t.Fatal("old bundle did not acquire compiled text", string(b), err)
	}
}
