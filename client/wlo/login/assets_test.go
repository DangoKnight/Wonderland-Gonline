package login

import (
	"archive/zip"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/clientfs"
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

// Login panels must use the same asset filesystem as the other bundled artwork.
func TestLoginBackgroundFromBundle(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "assets.zip")
	f, err := os.Create(bundle)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	path := "media/menu/Skins/white/Form_IdPassword_1.jpg.png"
	entry, err := z.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	if err := png.Encode(entry, img); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	root, closeBundle, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	a := NewAssets(root)
	background, err := loadJPEG(a.SkinPicture("Menu", "Skins", "White", "Form_IdPassword_1.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if background.W != 3 || background.H != 2 || background.Pix[0] != 0xf800 {
		t.Fatal("bundled login panel did not decode correctly")
	}
}
