package role

import (
	"image/color"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/surface"
)

func TestBattleCloudNativeSpriteAndLoop(t *testing.T) {
	lib := NewLibrary(filepath.Join("..", "..", "..", "data"))
	arc, key := lib.lookup("009", 9000)
	if arc == nil {
		t.Skip("client sprites not installed")
	}
	s := arc.sprite(key)
	if s == nil || s.frameCount(0) < 2 {
		t.Fatal("native battle cloud missing animation")
	}
	for _, elapsed := range []time.Duration{0, 100 * time.Millisecond, time.Second, 100 * time.Second} {
		dst := surface.New(200, 200)
		if !lib.DrawBattleCloud(dst, 100, 100, elapsed) {
			t.Fatal("cloud failed", elapsed)
		}
		visible := false
		for _, px := range dst.Pix {
			if px != 0 {
				visible = true
				break
			}
		}
		if !visible {
			t.Fatal("empty cloud frame", elapsed)
		}
	}
}

func TestBattleCloudMissingAssetAndOrigin(t *testing.T) {
	root := t.TempDir()
	lib := NewLibrary(root)
	if lib.DrawBattleCloud(surface.New(100, 100), 50, 50, 0) {
		t.Fatal("missing cloud reported success")
	}
	writePlacementSprite(t, root, "009", "9000.jmp", color.NRGBA{R: 255, A: 255})
	lib = NewLibrary(root)
	dst := surface.New(100, 100)
	if !lib.DrawBattleCloud(dst, 50, 50, 0) || dst.Pix[50*100+50] != 0xf800 {
		t.Fatal("native origin misplaced")
	}
}
