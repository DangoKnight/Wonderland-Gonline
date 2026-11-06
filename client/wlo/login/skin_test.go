package login

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/localization"
)

func skinFixture(t *testing.T, path string, ink color.RGBA) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	m := image.NewRGBA(image.Rect(0, 0, 3, 9))
	for y := 0; y < 9; y++ {
		for x := 0; x < 3; x++ {
			m.Set(x, y, ink)
		}
	}
	if err := png.Encode(file, m); err != nil {
		t.Fatal(err)
	}
}

func TestSingleUISkinWorksForEitherTextLanguage(t *testing.T) {
	previous := localization.CurrentLanguage()
	t.Cleanup(func() { catalog, _ := localization.Load("", previous); localization.Install(catalog) })
	for _, installed := range []string{localization.English, "en-localized", localization.Chinese} {
		t.Run(installed, func(t *testing.T) {
			a := NewAssets(t.TempDir())
			root := a.MediaPath("menu", "Skins") // Legacy/common English also remains supported.
			if installed == localization.Chinese {
				root = a.MediaPath("locales", installed, "menu", "Skins")
			} else if installed == "en-localized" {
				root = a.MediaPath("locales", localization.English, "menu", "Skins")
			}
			skinFixture(t, filepath.Join(root, "default", "Btn_Test.bmp.png"), color.RGBA{R: 200, A: 255})
			skinFixture(t, filepath.Join(root, "white", "Form_Test.jpg.png"), color.RGBA{G: 200, A: 255})
			for _, selected := range []string{localization.English, localization.Chinese} {
				catalog, _ := localization.Load(a.Root, selected)
				localization.Install(catalog)
				db := picdb.New()
				for _, directory := range a.UISkinRoots(selected) {
					for _, skin := range []string{"default", "white"} {
						if err := db.LoadDir(Path(directory, skin), false, 0); err != nil {
							t.Fatal(err)
						}
					}
				}
				index := db.Find("Btn_Test")
				if index < 0 || db.Entries[index].Path != Path(root, "default") {
					t.Fatal("missing or incorrect fallback UI", installed, selected, db.Entries)
				}
				if path := a.SkinPicture("Menu", "Skins", "White", "Form_Test.jpg"); !strings.EqualFold(path, Path(root, "white", "Form_Test.jpg.png")) {
					t.Fatal("background did not use available UI", installed, selected, path)
				}
			}
		})
	}
}

func TestCachedSkinBackgroundChangesWithoutReplacingInput(t *testing.T) {
	a := NewAssets(t.TempDir())
	for _, code := range []string{localization.English, localization.Chinese} {
		ink := color.RGBA{R: 200, A: 255}
		if code == localization.Chinese {
			ink = color.RGBA{G: 200, A: 255}
		}
		skinFixture(t, a.MediaPath("locales", code, "menu", "Skins", "white", "Form_Test.jpg.png"), ink)
	}
	previous := localization.CurrentLanguage()
	t.Cleanup(func() { catalog, _ := localization.Load("", previous); localization.Install(catalog) })
	en, _ := localization.Load(a.Root, localization.English)
	localization.Install(en)
	env := &seui.Env{Pics: picdb.New()}
	form := seui.NewForm(env)
	input := seui.NewEditor(env, form)
	input.SetText([]byte("Keep input"))
	var background *surface.Surface
	bindSkinBackground(form, "background", a, "Form_Test.jpg", &background)
	if background == nil {
		t.Fatal("English background missing")
	}
	first := background.Pix[0]
	zh, _ := localization.Load(a.Root, localization.Chinese)
	localization.Install(zh)
	form.Base().RefreshLanguage()
	if background == nil || background.Pix[0] == first || string(input.Text) != "Keep input" {
		t.Fatal("background refresh lost artwork or input")
	}
	if !strings.Contains(a.SkinPicture("menu", "skins", "white", "Form_Test.jpg"), "zh-TW") {
		t.Fatal("preferred language did not win")
	}
	background.Close()
}
