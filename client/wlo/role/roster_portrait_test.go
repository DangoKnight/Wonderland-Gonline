package role

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

func TestNativeRosterPortraitUsesSmallSpriteAndCanvas(t *testing.T) {
	root := t.TempDir()
	// Independent native sprite references for head 1, from FUN_001cb9e8.
	for _, tt := range []struct {
		body                   byte
		family, regular, small string
	}{
		{1, "002f", "2602.jmp", "2642.jmp"},
		{2, "003f", "3602.jmp", "3642.jmp"},
		{3, "004f", "4602.jmp", "4642.jmp"},
		{4, "005f", "5602.jmp", "5642.jmp"},
	} {
		dir := filepath.Join(root, "sprites", tt.family)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		im := image.NewNRGBA(image.Rect(0, 0, 2, 1))
		im.SetNRGBA(0, 0, color.NRGBA{R: 248, A: 255})
		im.SetNRGBA(1, 0, color.NRGBA{B: 248, A: 255})
		f, err := os.Create(filepath.Join(dir, "sheet.png"))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, im)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		doc := clientassets.EditableSprites{Version: 1}
		for n, entry := range []struct {
			index int
			name  string
		}{{1, tt.regular}, {41, tt.small}} {
			doc.Sprites = append(doc.Sprites, clientassets.EditableSprite{Index: entry.index, Name: entry.name,
				Animations: []clientassets.SpriteAnimation{{Frames: []int{0}}},
				Frames:     []clientassets.EditableFrame{{Sheet: "sheet.png", Rect: clientassets.SpriteRect{X: n, Width: 1, Height: 1}, CanvasWidth: 32, CanvasHeight: 32, AnchorX: 2, AnchorY: 5}},
			})
		}
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "editable.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		lib := NewLibrary(root)
		h := NewHuman(lib, nil)
		h.SetCharacter(&login.CharacterSlot{Body: tt.body, Head: 1, Color1: 444444444, Color2: 444444444})
		dst := surface.New(100, 100)
		// Roster cell (20,30); native draw origin is cell+(16,-3).
		h.DrawRosterPortrait(dst, 36, 27)
		lib.Sprites.Close()
		if dst.Pix[32*100+22] != surface.RGB565(0, 0, 248) {
			t.Fatal("wrong roster sprite or scaled canvas", tt.family)
		}
		count := 0
		for _, p := range dst.Pix {
			if p != 0 {
				count++
			}
		}
		if count != 1 {
			t.Fatal("native portrait was resized", tt.family, count)
		}
	}
}
