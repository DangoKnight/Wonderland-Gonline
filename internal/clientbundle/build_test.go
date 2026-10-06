package clientbundle

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/internal/clientfs"
)

func fixture(t *testing.T) (Options, string) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "data")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	return Options{Source: source, Output: filepath.Join(dir, "assets.zip"), Contract: filepath.Join(dir, "contract.json")}, source
}
func put(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
func picture(t *testing.T, path string, w, h int, ink color.NRGBA) {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	m.SetNRGBA(0, 0, ink)
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	put(t, path, b.Bytes())
}
func TestBuildEditableChangesReuseAndAtomicValidation(t *testing.T) {
	o, source := fixture(t)
	imagePath := filepath.Join(source, "page.png")
	picture(t, imagePath, 4, 4, color.NRGBA{R: 255, A: 255})
	put(t, filepath.Join(source, "values.json"), []byte("{\n\"rate\": 1\n}"))
	first, err := Build(o)
	if err != nil || first.Files != 2 || first.Reused != 0 {
		t.Fatal(first, err)
	}
	originalInfo, _ := os.Stat(o.Output)
	second, err := Build(o)
	if err != nil || second.Reused != 2 {
		t.Fatal(second, err)
	}
	afterInfo, _ := os.Stat(o.Output)
	if !os.SameFile(originalInfo, afterInfo) {
		t.Fatal("unchanged build rewrote bundle")
	}
	picture(t, imagePath, 4, 4, color.NRGBA{B: 255, A: 128})
	put(t, filepath.Join(source, "values.json"), []byte("{\n\"rate\": 2\n}"))
	third, err := Build(o)
	if err != nil || third.Reused != 0 {
		t.Fatal(third, err)
	}
	root, closeBundle, err := clientfs.Mount(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := clientfs.ReadFile(filepath.Join(root, "values.json"))
	if err != nil || string(raw) != "{\"rate\":2}" {
		t.Fatal(string(raw), err)
	}
	f, err := clientfs.Open(filepath.Join(root, "page.png"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(f)
	if err != nil || cfg.Width != 4 {
		t.Fatal(cfg, err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	m, err := png.Decode(f)
	f.Close()
	if err != nil || color.NRGBAModel.Convert(m.At(0, 0)).(color.NRGBA).A != 128 {
		t.Fatal("changed alpha not preserved", err)
	}
	closeBundle()
	before, _ := os.ReadFile(o.Output)
	picture(t, imagePath, 5, 4, color.NRGBA{})
	if _, err = Build(o); err == nil {
		t.Fatal("dimension contract ignored")
	}
	after, _ := os.ReadFile(o.Output)
	if !bytes.Equal(before, after) {
		t.Fatal("failed build replaced usable bundle")
	}
	picture(t, imagePath, 4, 4, color.NRGBA{})
	put(t, filepath.Join(source, "values.json"), []byte("{broken"))
	if _, err = Build(o); err == nil {
		t.Fatal("malformed JSON accepted")
	}
	after, _ = os.ReadFile(o.Output)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid JSON replaced bundle")
	}
}
func TestBuildManifestRectanglesAndSymlinks(t *testing.T) {
	o, source := fixture(t)
	picture(t, filepath.Join(source, "page.png"), 4, 4, color.NRGBA{})
	manifest := filepath.Join(source, "manifest.json")
	put(t, manifest, []byte(`{"png":"page.png","rect":{"x":3,"y":0,"width":2,"height":1}}`))
	if _, err := Build(o); err == nil {
		t.Fatal("out-of-bounds atlas accepted")
	}
	put(t, manifest, []byte(`{"png":"../page.png"}`))
	if _, err := Build(o); err == nil {
		t.Fatal("traversal accepted")
	}
	put(t, manifest, []byte(`{"png":"missing.png"}`))
	if _, err := Build(o); err == nil {
		t.Fatal("missing page accepted")
	}
	put(t, manifest, []byte(`{"png":"page.png","rect":{"x":0,"y":0,"width":4,"height":4}}`))
	if _, err := Build(o); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(manifest, filepath.Join(source, "link.json")); err != nil {
		t.Skip(err)
	}
	if _, err := Build(o); err == nil {
		t.Fatal("symlink accepted")
	}
}
func TestBundleCompressionMethods(t *testing.T) {
	o, source := fixture(t)
	picture(t, filepath.Join(source, "page.png"), 4, 4, color.NRGBA{})
	put(t, filepath.Join(source, "values.json"), []byte(`{"n":1}`))
	if _, err := Build(o); err != nil {
		t.Fatal(err)
	}
	z, err := zip.OpenReader(o.Output)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for _, f := range z.File {
		switch f.Name {
		case "page.png":
			if f.Method != zip.Store {
				t.Fatal("PNG recompressed")
			}
		case "values.json":
			if f.Method != zip.Deflate {
				t.Fatal("JSON uncompressed")
			}
		}
	}
}

func TestRawCopyDropsObsoleteZIP64Offset(t *testing.T) {
	// The old offset is 4 GiB + 7. Preserve unrelated extended timestamps.
	extra := []byte{1, 0, 8, 0, 7, 0, 0, 0, 1, 0, 0, 0, 0x55, 0x54, 1, 0, 9}
	got := withoutZIP64(extra)
	if !bytes.Equal(got, []byte{0x55, 0x54, 1, 0, 9}) {
		t.Fatalf("obsolete offset retained: %x", got)
	}
}
