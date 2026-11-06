package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"wonderland-gonline/internal/localization"
)

func TestChineseClientAssetsAndNativeArtwork(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(filepath.Join(root, "runtime", "locales", "zh-TW.json")); err != nil {
		t.Skip("compile client language catalogs first")
	}
	en, err := localization.Load(root, localization.English)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { localization.Install(en) })
	r := &Resources{}
	t.Cleanup(func() { r.Close() })
	userRoot := t.TempDir()
	prefs := defaultDisplaySettings()
	prefs.Language = localization.Chinese
	if err := prefs.save(filepath.Join(userRoot, "display.json")); err != nil {
		t.Fatal(err)
	}
	c, err := New(Options{Root: root, Shared: r, UserRoot: userRoot, ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString((&Session{ID: 1}).DisplayTitle()) {
		t.Fatal("session title was encoded before its rendering boundary")
	}
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Tester"))
	c.loadTalks()
	if c.npcTemplates[10000].Name != "布蕾雅" || c.items[10034].Definition.Name != "赤鐵刀" || c.talks[57166] != "布蕾雅" {
		t.Fatal("Chinese source was not resolved by stable IDs", c.npcTemplates[10000].Name, c.items[10034].Definition.Name, c.talks[57166])
	}
	if len(c.Pics.Missing) > 0 {
		t.Fatal("missing Chinese interface artwork", c.Pics.Missing)
	}
	localized := 0
	for _, entry := range c.Pics.Entries {
		if strings.Contains(entry.Path, "zh-TW") {
			localized++
		}
	}
	if localized < 100 {
		t.Fatal("native Chinese interface labels not registered", localized)
	}
	if path := os.Getenv("CHINESE_CLIENT_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
		g := NewGame(c)
		g.display = newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
		g.display.openMenu()
		g.display.openSelect(displayLanguageRow, image.Pt(-1, -1))
		frame := g.Workspace.Compose()
		g.drawDisplay(frame)
		output, err := os.Create(strings.TrimSuffix(path, ".png") + "-language.png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(output, frame.RGBA()); err != nil {
			t.Fatal(err)
		}
		output.Close()
	}
	if _, err := New(Options{Root: root, Language: localization.English, Shared: r}); err == nil {
		t.Fatal("mixed shared-cache languages accepted")
	}
}
