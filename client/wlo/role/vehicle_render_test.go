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
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/game"
)

func writePlacementSprite(t *testing.T, root, family, name string, color color.NRGBA) {
	t.Helper()
	dir := filepath.Join(root, "sprites", family)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color)
	f, err := os.Create(filepath.Join(dir, "sheet.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	f.Close()
	actions := make([]clientassets.SpriteAnimation, 54)
	for i := range actions {
		actions[i] = clientassets.SpriteAnimation{Frames: []int{0}}
	}
	index := 0
	if family == "006" {
		index = 5
	}
	if family == "001" {
		index = 867
	}
	data, err := json.Marshal(clientassets.EditableSprites{Version: 1, Sprites: []clientassets.EditableSprite{{Index: index, Name: name, Animations: actions, Frames: []clientassets.EditableFrame{{Sheet: "sheet.png", Rect: clientassets.SpriteRect{Width: 1, Height: 1}, CanvasWidth: 2, CanvasHeight: 32, AnchorX: 1}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "editable.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestNativeRaftAndWeaponPlacement(t *testing.T) {
	root := t.TempDir()
	writePlacementSprite(t, root, "002", "2000.jmp", color.NRGBA{R: 255, A: 255})
	writePlacementSprite(t, root, "006", "6005.jmp", color.NRGBA{B: 255, A: 255})
	writePlacementSprite(t, root, "002w", "2001.jmp", color.NRGBA{R: 255, G: 255, A: 255})
	items := map[uint16]assets.NativeItem{99: {Definition: game.ItemDefinition{Type: 5}, Sprites: [4]uint16{2001}}}
	h := NewHuman(NewLibrary(root), items)
	h.body = 1
	h.colors = NeutralColors()
	h.equip[3] = 99
	h.Now = func() time.Time { return time.Unix(0, 0) }
	dst := surface.New(250, 250)
	h.DrawBody(dst, 100, 100, 12)
	// FUN_002fe8e8's weapon correction: one pixel at the canvas origin +68 Y.
	if dst.Pix[168*dst.W+100] != surface.RGB565(255, 255, 0) || dst.Pix[100*dst.W+100] != surface.RGB565(255, 0, 0) {
		t.Fatalf("weapon/body origins differ from native placement: weapon=%#x body=%#x", dst.Pix[168*dst.W+100], dst.Pix[100*dst.W+100])
	}
	h.SetVehiclePose(6005, true)
	dst = surface.New(250, 250)
	h.DrawBody(dst, 100, 100, 12)
	// Direction 4: native body-1 rider offset (0,+13); vehicle origin +68.
	if dst.Pix[113*dst.W+100] != surface.RGB565(255, 0, 0) || dst.Pix[168*dst.W+100] != surface.RGB565(0, 0, 255) {
		t.Fatal("raft and rider placement differ from native tables")
	}
	for _, pixel := range dst.Pix {
		if pixel == surface.RGB565(255, 255, 0) {
			t.Fatal("native riding pose should hide this hand weapon")
		}
	}
	items[99] = assets.NativeItem{Definition: game.ItemDefinition{Type: 6}, Sprites: [4]uint16{2001}}
	dst = surface.New(250, 250)
	h.DrawBody(dst, 100, 100, 12)
	if dst.Pix[181*dst.W+100] != surface.RGB565(255, 255, 0) {
		t.Fatal("type-6 mounted weapon exception missing")
	}
}
