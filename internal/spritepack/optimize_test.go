package spritepack

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wonderland-gonline/internal/clientassets"
)

func TestOptimizeEditablePreservesEditsAndMetadata(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hero")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	m := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	m.SetNRGBA(0, 0, color.NRGBA{R: 23, G: 41, B: 89, A: 137})
	for _, name := range []string{"a.png", "b.png"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, m)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	unique := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	unique.SetNRGBA(0, 0, color.NRGBA{R: 199, G: 17, B: 53, A: 83})
	f, err := os.Create(filepath.Join(dir, "c.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, unique); err != nil {
		t.Fatal(err)
	}
	f.Close()
	doc := clientassets.EditableSprites{Version: 1, Sprites: []clientassets.EditableSprite{{Index: 3, Name: "painted",
		Frames: []clientassets.EditableFrame{
			{Name: "one", Sheet: "a.png", Rect: clientassets.SpriteRect{Width: 1, Height: 1}, CanvasWidth: 80, CanvasHeight: 90, AnchorX: 7, AnchorY: 9},
			{Name: "two", Sheet: "b.png", Rect: clientassets.SpriteRect{Width: 1, Height: 1}, CanvasWidth: 80, CanvasHeight: 90, AnchorX: 11, AnchorY: 17},
			{Name: "empty", Rect: clientassets.SpriteRect{Height: 3}},
			{Name: "unique", Sheet: "c.png", Rect: clientassets.SpriteRect{Width: 1, Height: 1}},
		}, Animations: []clientassets.SpriteAnimation{{Name: "walk", Frames: []int{1, 0, 2, -1}}},
	}}}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, EditableFile), raw, 0644); err != nil {
		t.Fatal(err)
	}
	preservation := []byte("native preservation metadata")
	if err = os.WriteFile(filepath.Join(dir, "preservation.json"), preservation, 0644); err != nil {
		t.Fatal(err)
	}
	result, err := OptimizeEditable(dir, "hero")
	if err != nil {
		t.Fatal(err)
	}
	if result.BeforeSheets != 3 || result.AfterSheets != 1 {
		t.Fatal("not packed", result)
	}
	optimized, err := clientassets.OpenEditableSprites(filepath.Join(dir, EditableFile))
	if err != nil {
		t.Fatal(err)
	}
	frames := optimized.Sprites[0].Frames
	if frames[0].Sheet != frames[1].Sheet || frames[0].Rect != frames[1].Rect {
		t.Fatal("identical pixels not deduplicated")
	}
	if frames[0].AnchorX != 7 || frames[1].AnchorX != 11 || frames[1].AnchorY != 17 || frames[2].Rect.Height != 3 {
		t.Fatal("placement changed")
	}
	if !reflect.DeepEqual(optimized.Sprites[0].Animations, doc.Sprites[0].Animations) {
		t.Fatal("animations changed")
	}
	got, err := optimized.FrameImage(frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if got.NRGBAAt(0, 0) != m.NRGBAAt(0, 0) {
		t.Fatal("painted color changed")
	}
	uniqueFrame, err := optimized.FrameImage(frames[3])
	if err != nil {
		t.Fatal(err)
	}
	if uniqueFrame.NRGBAAt(uniqueFrame.Rect.Min.X, uniqueFrame.Rect.Min.Y) != unique.NRGBAAt(0, 0) {
		t.Fatal("distinct frame pixels changed")
	}
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("obsolete sheet remains", name)
		}
	}
	kept, err := os.ReadFile(filepath.Join(dir, "preservation.json"))
	if err != nil || string(kept) != string(preservation) {
		t.Fatal("preservation metadata changed", err)
	}
	// Repeat optimization must also work when atlases already occupy atlas/.
	if _, err = OptimizeEditable(dir, "hero"); err != nil {
		t.Fatal(err)
	}
}
