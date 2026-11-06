package app

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/clientfs"
)

func TestDirectDataLogin(t *testing.T) {
	root := os.Getenv("WONDERLAND_TEST_DATA_ROOT")
	if root == "" {
		t.Skip("set WONDERLAND_TEST_DATA_ROOT to source-derived data")
	}
	c, err := New(Options{Root: root, ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Pics.Missing) != 0 {
		t.Fatalf("missing PNG skin files: %v", c.Pics.Missing)
	}
	if len(c.Pics.Entries) == 0 || c.background == nil || c.Servers.Background == nil {
		t.Fatal("decoded login artwork not loaded")
	}
}

func TestCompiledBundleLoginAndSharedWorld(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE to a compiled asset bundle")
	}
	root, closeBundle, err := clientfs.Mount(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	r := &Resources{}
	defer r.Close()
	o := Options{Root: root, Shared: r, UserRoot: t.TempDir(), ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")}
	c, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Net.Shutdown()
	c.Net.Dial = func(string, string) (net.Conn, error) { return nil, errNoNetwork }
	icon, err := c.lib.Image(xaolanPortraitFamily, xaolanPortraitID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if icon.Bounds().Dx() != 24 || icon.Bounds().Dy() != 24 {
		t.Fatal("compiled Xaolan icon missing")
	}
	c.Frame()
	if c.background == nil || c.Servers.Background == nil || c.Login.Background == nil || len(c.Pics.Entries) == 0 || len(c.SkillState.Catalog.Definitions) == 0 {
		t.Fatal("compiled login/skills assets unavailable")
	}
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Tester"))
	if c.World == nil {
		for _, notice := range c.Notices.items {
			t.Log(string(notice.text))
		}
		t.Fatal("compiled world entry failed")
	}
	c.loadTalks()
	if c.items[10034].Definition.Name != "Pure Iron Knife" || c.npcTemplates[17738].Name != "Half Beast Ghost" || c.talks[57166] != "Breillat" {
		t.Fatal("bundle text fields were not translated during asset compilation")
	}
	c.Frame()
	next, err := New(o)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Net.Shutdown()
	next.Net.Dial = c.Net.Dial
	next.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Observer"))
	if next.World == nil || next.World.Scene == c.World.Scene || next.World.Scene.Layers[0].Image != c.World.Scene.Layers[0].Image {
		t.Fatal("scene descriptors not isolated or map pixels duplicated")
	}
	c.mapReady = true
	c.dispatch([]byte{104, 1, 1, 0, 1, 216, 132, 3})
	c.dispatch([]byte{104, 1, 2, 1, 1})
	c.LuckyDraw.Show()
	c.LuckyDraw.Preview.Open()
	c.Frame()
	if c.LuckyDraw.Image < 0 || c.LuckyDraw.Spin.Image < 0 || c.LuckyDraw.Preview.Image < 0 || c.LuckyDraw.State.Slot != 1 {
		t.Fatal("compiled Lucky Draw artwork or result unavailable")
	}
	if next.LuckyDraw.State.Ready || next.LuckyDraw.Visible || next.LuckyDraw.Preview.Visible {
		t.Fatal("shared resources leaked Lucky Draw state across sessions")
	}
	c.LuckyDraw.Hide()
	c.Frame()
	c.dispatch([]byte{15, 20, 6, 0})
	if run := c.constellation; run == nil || run.picture < 0 || run.sky < 0 {
		t.Fatal("compiled constellation artwork unavailable")
	}
	if next.constellation != nil {
		t.Fatal("shared resources leaked constellation cutscene state")
	}
	c.stopConstellation()
	if path := os.Getenv("BUNDLE_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
