package clientfs

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func archive(t *testing.T, names ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pack.zip")
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	z := zip.NewWriter(f)
	for _, name := range names {
		w, e := z.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Write([]byte("asset")); e != nil {
			t.Fatal(e)
		}
	}
	if e = z.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	return path
}
func TestMountedDirectoriesAndMissingFiles(t *testing.T) {
	bundle := archive(t, "media/Menu/page.png")
	root, closeBundle, err := Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	entries, err := ReadDir(filepath.Join(root, "media"))
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		t.Fatal(entries, err)
	}
	info, err := Stat(filepath.Join(root, "media", "Menu", "page.png"))
	if err != nil || info.Size() != 5 {
		t.Fatal(info, err)
	}
	if _, err = ReadFile(filepath.Join(root, "missing.json")); !os.IsNotExist(err) {
		t.Fatal("missing bundled file did not fail", err)
	}
	if _, _, err = Mount(bundle); err == nil {
		t.Fatal("duplicate mount accepted")
	}
}
func TestUnsafeBundleNames(t *testing.T) {
	for _, names := range [][]string{{"../escape"}, {"/absolute"}, {"a\\b"}, {"same", "same"}} {
		if _, closeBundle, err := Mount(archive(t, names...)); err == nil {
			closeBundle()
			t.Fatal("unsafe bundle accepted", names)
		}
	}
}
