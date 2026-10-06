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
	if path := os.Getenv("BUNDLE_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}
