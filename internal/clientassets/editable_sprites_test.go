package clientassets

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writeEditableTest(t *testing.T, root string, document EditableSprites) string {
	t.Helper()
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "editable.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func TestEditableSpritesTrueColorAndAlpha(t *testing.T) {
	root := t.TempDir()
	sheet := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	sheet.SetNRGBA(1, 1, color.NRGBA{R: 137, G: 82, B: 249, A: 128})
	file, err := os.Create(filepath.Join(root, "page.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(file, sheet); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	document := EditableSprites{Version: 1, Sprites: []EditableSprite{{Name: "hero", Animations: []SpriteAnimation{{Name: "idle", Frames: []int{0, -1}}}, Frames: []EditableFrame{{Sheet: "page.png", Rect: SpriteRect{X: 1, Y: 1, Width: 1, Height: 1}, CanvasWidth: 256, CanvasHeight: 256, AnchorX: 10, AnchorY: -3}}}}}
	path := writeEditableTest(t, root, document)
	archive, err := OpenEditableSprites(path)
	if err != nil {
		t.Fatal(err)
	}
	pixels, err := archive.FrameImage(archive.Sprites[0].Frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := pixels.NRGBAAt(1, 1); got != (color.NRGBA{R: 137, G: 82, B: 249, A: 128}) {
		t.Fatalf("RGBA pixel changed: %v", got)
	}
	// Metadata is editable directly, with no native archive or executable.
	document.Sprites[0].Frames[0].AnchorX = 23
	writeEditableTest(t, root, document)
	archive, err = OpenEditableSprites(path)
	if err != nil || archive.Sprites[0].Frames[0].AnchorX != 23 {
		t.Fatalf("edited anchor: %v", err)
	}
	frame := archive.Sprites[0].Frames[0]
	frame.Rect.X = 3
	if _, err = archive.FrameImage(frame); err == nil {
		t.Fatal("out-of-sheet rectangle accepted")
	}
}
func TestEditableSpritesRejectInvalidMetadata(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"../outside.png", "/outside.png", "C:outside.png", "..\\outside.png", "image.jpg"} {
		d := EditableSprites{Version: 1, Sprites: []EditableSprite{{Frames: []EditableFrame{{Sheet: path, Rect: SpriteRect{Width: 1, Height: 1}}}}}}
		if _, err := OpenEditableSprites(writeEditableTest(t, root, d)); err == nil {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
	d := EditableSprites{Version: 1, Sprites: []EditableSprite{{Frames: []EditableFrame{{}}, Animations: []SpriteAnimation{{Frames: []int{1}}}}}}
	if _, err := OpenEditableSprites(writeEditableTest(t, root, d)); err == nil {
		t.Fatal("invalid animation frame accepted")
	}
}
func TestNativeSpriteColorKeys(t *testing.T) {
	var palette [256][3]uint8
	palette[2] = [3]uint8{0, 255, 0}
	palette[3] = [3]uint8{0, 0, 255}
	palette[4] = [3]uint8{18, 40, 80}
	for _, tc := range []struct {
		index byte
		want  [4]byte
	}{{0, [4]byte{}}, {1, [4]byte{0, 4, 0, 255}}, {2, [4]byte{}}, {3, [4]byte{}}, {4, [4]byte{18, 40, 80, 255}}} {
		r, g, b, a := NativeSpriteColor(tc.index, palette)
		if [4]byte{r, g, b, a} != tc.want {
			t.Fatalf("palette index %d: %d %d %d %d", tc.index, r, g, b, a)
		}
	}
}

func TestEditableSpriteIndexIsIndependentOfArrayOrder(t *testing.T) {
	root := t.TempDir()
	document := EditableSprites{Version: 1, Sprites: []EditableSprite{{Index: 7, Name: "seven"}, {Index: 0, Name: "zero"}}}
	archive, err := OpenEditableSprites(writeEditableTest(t, root, document))
	if err != nil {
		t.Fatal(err)
	}
	if archive.Sprite(7).Name != "seven" || archive.Sprite(0).Name != "zero" || archive.Sprite(2) != nil {
		t.Fatal("sprite indexes depend on JSON array order")
	}
	document.Sprites[1].Index = 7
	if _, err = OpenEditableSprites(writeEditableTest(t, root, document)); err == nil {
		t.Fatal("duplicate sprite index accepted")
	}
}
