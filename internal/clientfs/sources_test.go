package clientfs

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"
)

func sourceArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestMemoryAndReaderMountsResolveAndClose(t *testing.T) {
	main := sourceArchive(t, map[string]string{"_packs.json": `{"version":1,"packs":["world.zip"]}`, "media/main.bin": "main"})
	world := sourceArchive(t, map[string]string{"media/world.bin": "world", "_bundle.json": "{}"})
	for _, mode := range []string{"bytes", "packs"} {
		t.Run(mode, func(t *testing.T) {
			var root string
			var closeMount func() error
			var err error
			if mode == "bytes" {
				root, closeMount, err = MountBytes("memory-bytes", main, map[string][]byte{"world.zip": world})
			} else {
				root, closeMount, err = MountPacks("memory-packs", []ZipSource{{R: bytes.NewReader(main), Size: int64(len(main))}, {R: bytes.NewReader(world), Size: int64(len(world))}})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer closeMount()
			if !filepath.IsAbs(root) {
				t.Fatal("virtual mount not normalized for platform", root)
			}
			for name, want := range map[string]string{"main": "main", "world": "world"} {
				got, err := ReadFile(filepath.Join(root, "media", name+".bin"))
				if err != nil || string(got) != want {
					t.Fatal(name, string(got), err)
				}
			}
			if err := closeMount(); err != nil {
				t.Fatal(err)
			}
			if err := closeMount(); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadFile(filepath.Join(root, "media", "world.bin")); !errors.Is(err, fs.ErrNotExist) {
				t.Fatal("closed mount still readable", err)
			}
		})
	}
}
func TestReaderMountsRejectDuplicateAndMissingPacks(t *testing.T) {
	core := sourceArchive(t, map[string]string{"_packs.json": `{"version":1,"packs":["missing.zip"]}`})
	if _, _, err := MountBytes("missing-pack", core, nil); err == nil {
		t.Fatal("missing companion accepted")
	}
	data := sourceArchive(t, map[string]string{"same": "data"})
	p := ZipSource{R: bytes.NewReader(data), Size: int64(len(data))}
	if _, _, err := MountPacks("duplicate-pack", []ZipSource{p, p}); err == nil {
		t.Fatal("duplicate entries accepted")
	}
}
