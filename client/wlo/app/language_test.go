package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/localization"
)

func TestFirstLaunchDefaultLanguagePriority(t *testing.T) {
	for _, tc := range []struct{ name, saved, explicit, want string }{
		{"first choice", "", "", localization.Chinese},
		{"later settings", localization.English, "", localization.English},
		{"command line", localization.Chinese, localization.English, localization.English},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newClientFixture(t)
			o := Options{Root: f.root, Shared: f.resources, UserRoot: t.TempDir(), DefaultLanguage: localization.Chinese, Language: tc.explicit, ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")}
			if tc.saved != "" {
				s := defaultDisplaySettings()
				s.Language = tc.saved
				if err := s.save(filepath.Join(o.UserRoot, "display.json")); err != nil {
					t.Fatal(err)
				}
			}
			c, err := New(o)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Net.Shutdown()
			if c.resources.language != tc.want {
				t.Fatal(c.resources.language, tc.want)
			}
		})
	}
}

func TestLiveLanguagePreservesSharedSessionsAndConversation(t *testing.T) {
	c, _, _ := enteredClient(t)
	w := NewWorkspace(c)
	w.options.UserRoot = t.TempDir()
	t.Cleanup(func() {
		if commit, err := w.prepareLanguage(localization.English); err == nil {
			commit()
		}
	})
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	other := w.Current()
	t.Cleanup(func() { other.Net.Shutdown() })
	other.Login.Account.SetText([]byte("KeepMyAccount"))
	sent := wire(t, c)
	player := c.World.Player
	state, net, scene, input := c.InventoryState, c.Net, c.World.Scene, other.Login.Account
	item := c.items[10034]
	skinIndex := c.Pics.Find("Btn_OK_1")
	hintBefore := append([]byte(nil), c.MainButtons.Buttons[1].HintText...)
	c.World.Questions[65000] = world.Question{Entries: []world.QuestionEntry{
		{Kind: world.QuestionText, Type: world.QuestionPrompt, Value: 57166},
		{Kind: world.QuestionText, Type: world.QuestionOption, Value: 57171},
		{Kind: world.QuestionText, Type: world.QuestionOption, Value: 57174},
	}}
	c.ask(eventSpeakerSelf, 0, 65000, 0)
	answers := append([]byte(nil), c.event.answers...)
	var sounds int
	c.Talk.Sound = func(string) { sounds++ }
	commit, err := w.prepareLanguage(localization.Chinese)
	if err != nil {
		t.Fatal(err)
	}
	if c.resources.language != localization.English {
		t.Fatal("preparation mutated live language")
	}
	commit()
	if c.npcTemplates[10000].Name != "布蕾雅" || c.items[10034].Definition.Name != "赤鐵刀" || other.items[10034].Definition.Name != "赤鐵刀" {
		t.Fatal("shared source definitions not refreshed")
	}
	if !bytes.Equal(bytes.Join(c.Talk.Lines(), nil), clientassets.Big5Text("布蕾雅")) || !bytes.Equal(c.Talk.Options()[0], clientassets.Big5Text("神父")) {
		t.Fatal("open dialogue/question was not relocalized")
	}
	if sounds != 0 || !bytes.Equal(c.event.answers, answers) || len(sent()) != 0 {
		t.Fatal("language change replayed a sound or sent a gameplay request", sounds, sent())
	}
	if c.InventoryState != state || c.Net != net || c.World.Scene != scene || !reflect.DeepEqual(player, c.World.Player) || other.Login.Account != input || string(input.Text) != "KeepMyAccount" {
		t.Fatal("language change replaced gameplay or input state")
	}
	after := c.items[10034]
	after.Definition.Name, after.Description = item.Definition.Name, item.Description
	if !reflect.DeepEqual(after, item) {
		t.Fatal("language change modified item mechanics")
	}
	if c.Pics.Find("Btn_OK_1") != skinIndex || !bytes.Contains([]byte(c.Pics.Entries[skinIndex].Path), []byte("zh-TW")) {
		t.Fatal("skin indices changed or locale artwork did not update")
	}
	if bytes.Equal(hintBefore, c.MainButtons.Buttons[1].HintText) || !bytes.Equal(c.MainButtons.Buttons[1].HintText, other.MainButtons.Buttons[1].HintText) {
		t.Fatal("existing session toolbar hints did not update")
	}
	if path := os.Getenv("LIVE_LANGUAGE_SNAPSHOT"); path != "" {
		c.Sprites.WaitNative()
		c.Frame()
		savePNG(t, path, c)
	}
	if err := w.Add(); err != nil {
		t.Fatal("new session did not inherit live language", err)
	}
	t.Cleanup(func() { w.Current().Net.Shutdown() })
	if w.Current().resources.language != localization.Chinese {
		t.Fatal("new session reverted language")
	}
	back, err := w.prepareLanguage(localization.English)
	if err != nil {
		t.Fatal(err)
	}
	back()
	if c.items[10034].Definition.Name != "Pure Iron Knife" || !bytes.Equal(hintBefore, c.MainButtons.Buttons[1].HintText) || bytes.Contains([]byte(c.Pics.Entries[skinIndex].Path), []byte("zh-TW")) {
		t.Fatal("switching back retained Chinese caches")
	}
}

func TestLiveLanguagePreparationFailureDoesNotSavePreferences(t *testing.T) {
	d := newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	d.openMenu()
	d.draft.Language = localization.Chinese
	d.prepareLanguage = func(string) (func(), error) { return nil, errors.New("bad catalog") }
	d.applyDraft(time.Unix(0, 0))
	if d.settings.Language == localization.Chinese || d.error == "" {
		t.Fatal("failed language preparation changed preferences")
	}
	got, err := loadDisplaySettings(d.path)
	if err != nil || got.Language == localization.Chinese {
		t.Fatal("failed language preparation persisted", got, err)
	}
}

func TestLiveLanguageFromCompiledBundle(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE for packaged language switching")
	}
	root, closeBundle, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	r := &Resources{}
	defer r.Close()
	c, err := New(Options{Root: root, Language: localization.English, Shared: r, UserRoot: t.TempDir(), ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorkspace(c)
	for _, code := range []string{localization.Chinese, localization.English} {
		commit, err := w.prepareLanguage(code)
		if err != nil {
			t.Fatal(err)
		}
		commit()
		c.Frame()
		want := "Pure Iron Knife"
		if code == localization.Chinese {
			want = "赤鐵刀"
		}
		if c.items[10034].Definition.Name != want || len(c.Pics.Missing) > 0 {
			t.Fatal("packaged locale text or skin failed", code, c.items[10034].Definition.Name, c.Pics.Missing)
		}
	}
}
