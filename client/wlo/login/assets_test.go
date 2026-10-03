package login

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectDataAssets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset_manifest.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	a := NewAssets(root)
	if a.Data != root {
		t.Fatalf("data root %s", a.Data)
	}
	if a.SkinPicture("menu", "Skins", "white", "Form_ServerList_1.jpg") != filepath.Join(root, "media", "menu", "Skins", "white", "Form_ServerList_1.jpg.png") {
		t.Fatal("wrong standard background path")
	}
	if a.UserPath("save.dat") != filepath.Join(filepath.Dir(root), "var", "client", "user", "save.dat") {
		t.Fatal("state written into static data")
	}
}
