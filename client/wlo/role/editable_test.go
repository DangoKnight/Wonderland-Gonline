package role

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

func TestEditableSpriteDrawWithoutNativeArchives(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "sprites", "002s2")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 128})
	pixels.SetNRGBA(1, 0, color.NRGBA{A: 255})         // opaque black is allowed in PNGs
	pixels.SetNRGBA(2, 0, color.NRGBA{G: 255, A: 255}) // ordinary opaque green
	file, err := os.Create(filepath.Join(directory, "sheet.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, pixels); err != nil {
		t.Fatal(err)
	}
	file.Close()
	d := clientassets.EditableSprites{Version: 1, Sprites: []clientassets.EditableSprite{{Name: "12400.jmp", Animations: []clientassets.SpriteAnimation{{Name: "idle", Frames: []int{0, -1}}}, Frames: []clientassets.EditableFrame{{Sheet: "sheet.png", Rect: clientassets.SpriteRect{Width: 3, Height: 1}, CanvasWidth: 8, CanvasHeight: 32, AnchorX: 1, AnchorY: 2}}}}}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(directory, "editable.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	lib := NewLibrary(root)
	archive := lib.open("002S2", 12400)
	if archive == nil {
		t.Fatal("editable archive not loaded")
	}
	sprite := archive.sprite(12400)
	if sprite == nil {
		t.Fatal("editable sprite missing")
	}
	if sprite.frameCount(0) != 2 || sprite.frame(0, 1) != nil {
		t.Fatal("animation mapping changed")
	}
	dst := surface.New(12, 8)
	for i := range dst.Pix {
		dst.Pix[i] = surface.RGB565(0, 0, 255)
	}
	sprite.draw(dst, sprite.frame(0, 0), 6, 1)
	// Placement: left=6-(8/2-1)=3, top=1-(32-32)+2=3.
	if got := dst.Pix[3*dst.W+3]; got != surface.RGB565(128, 0, 127) {
		t.Fatalf("alpha blend: %#x", got)
	}
	if got := dst.Pix[3*dst.W+4]; got != 0 {
		t.Fatalf("opaque black: %#x", got)
	}
	if got := dst.Pix[3*dst.W+5]; got != surface.RGB565(0, 255, 0) {
		t.Fatalf("opaque green: %#x", got)
	}
	if got := dst.Pix[3*dst.W+6]; got != surface.RGB565(0, 0, 255) {
		t.Fatalf("outside frame changed: %#x", got)
	}
}

// TestIntervalFor: walking actions step every 100 ms, standing ones every
// 230 ms (FUN_004122ec).
func TestIntervalFor(t *testing.T) {
	for action, want := range map[int]time.Duration{0: 100 * time.Millisecond, 7: 100 * time.Millisecond,
		8: 230 * time.Millisecond, 15: 230 * time.Millisecond, 16: 100 * time.Millisecond} {
		if got := intervalFor(action); got != want {
			t.Errorf("action %d: %v, want %v", action, got, want)
		}
	}
}
