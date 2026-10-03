package app

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var errNoNetwork = errors.New("network disabled in tests")

// testClient builds the client over the local asset copy with a one-server
// list and no network.
func testClient(t *testing.T) *Client {
	t.Helper()
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(root); err != nil {
		t.Skip("client assets not installed")
	}
	c, err := New(Options{
		Root:        root,
		ServerINI:   filepath.Join(t.TempDir(), "SERVER.INI"),
		FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n"),
		// SNAPSHOT_SPRITES selects sprite packs or the editable export.
		SpritesRoot: os.Getenv("SNAPSHOT_SPRITES"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Sprites.Close() })
	c.Net.Dial = func(string, string) (net.Conn, error) { return nil, errNoNetwork }
	c.Input.X, c.Input.Y = -100, -100
	return c
}

// compare counts pixels that differ from a capture of the original,
// skipping the window frame's rounded corners. DIFF_DIR receives the
// frame and a difference image.
func compare(t *testing.T, name string, c *Client) int {
	t.Helper()
	f, err := os.Open(filepath.Join("..", "..", "reference", "screenshots", "Login", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ref, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	got := c.Screen.RGBA()
	diff := image.NewRGBA(got.Rect)
	bad := 0
	for y := range ScreenHeight {
		for x := range ScreenWidth {
			if y >= ScreenHeight-5 && (x < 5 || x >= ScreenWidth-5) {
				continue
			}
			r, g, b, _ := ref.At(x, y).RGBA()
			p := got.RGBAAt(x, y)
			o := diff.PixOffset(x, y)
			if uint8(r>>8) != p.R || uint8(g>>8) != p.G || uint8(b>>8) != p.B {
				bad++
				diff.Pix[o], diff.Pix[o+3] = 255, 255
			} else {
				diff.Pix[o], diff.Pix[o+1], diff.Pix[o+2], diff.Pix[o+3] = p.R/3, p.G/3, p.B/3, 255
			}
		}
	}
	if dir := os.Getenv("DIFF_DIR"); dir != "" {
		for n, m := range map[string]*image.RGBA{"got": got, "diff": diff} {
			if out, err := os.Create(filepath.Join(dir, fmt.Sprintf("%s.%s.png", name, n))); err == nil {
				png.Encode(out, m)
				out.Close()
			}
		}
	}
	return bad
}

func TestServerListMatchesOriginal(t *testing.T) {
	c := testClient(t)
	c.Frame()
	if bad := compare(t, "server_select.png", c); bad > 0 {
		t.Errorf("server list: %d pixels differ", bad)
	}
	c.G.Status[101] = 1
	// A click selects the row, then calls OnSelect (FUN_0046f090).
	c.Servers.Regions.Selected = 0
	c.Servers.Regions.OnSelect(0)
	c.Frame()
	if bad := compare(t, "server_select_region.png", c); bad > 0 {
		t.Errorf("server list with a region: %d pixels differ", bad)
	}
}

func TestLoginMatchesOriginal(t *testing.T) {
	c := testClient(t)
	c.Frame()
	c.Login.ServerDescription([]byte{9, 101, 0, 1})
	c.Now = func() time.Time { return time.Now() }
	c.Frame()
	if bad := compare(t, "login.png", c); bad > 0 {
		t.Errorf("login: %d pixels differ", bad)
	}
}
