package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"wonderland-go/internal/clientassets"
)

func TestEditableExportPixelsAnchorsAndEmptyFrames(t *testing.T) {
	root := t.TempDir()
	source := &clientassets.Sprite{Frames: []clientassets.SpriteFrame{
		{Name: "a", CanvasWidth: 64, CanvasHeight: 64, X: 7, Y: 8, Width: 1, Height: 2, Stride: 4, Pixels: []byte{1, 0, 0, 0, 0, 0, 0, 0}},
		{Name: "b", CanvasWidth: 64, CanvasHeight: 64, X: 9, Y: 10, Width: 3, Height: 1, Stride: 4, Pixels: []byte{2, 3, 1, 0}},
		{Name: "empty", Width: 0, Height: 3},
	}}
	source.Palette[1] = [3]uint8{18, 50, 90}
	source.Palette[2] = [3]uint8{0, 255, 0}
	source.Palette[3] = [3]uint8{0, 0, 0}
	frames, err := exportSpriteSheets(root, 0, "12400.jmp", source)
	if err != nil {
		t.Fatal(err)
	}
	if frames[0].Sheet != "12400/page_000.png" || frames[0].AnchorX != 7 || frames[0].AnchorY != 8 || frames[1].Rect.X != 1 || frames[2].Rect.Height != 3 || frames[2].Sheet != "" {
		t.Fatalf("frame placement: %+v", frames)
	}
	document := clientassets.EditableSprites{Version: 1, Sprites: []clientassets.EditableSprite{{Name: "hero", Frames: frames}}}
	path := filepath.Join(root, "editable.json")
	if err = jsonWrite(path, document); err != nil {
		t.Fatal(err)
	}
	archive, err := clientassets.OpenEditableSprites(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := archive.FrameImage(frames[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := first.NRGBAAt(0, 0); got != (color.NRGBA{R: 18, G: 50, B: 90, A: 255}) {
		t.Fatalf("colored pixel: %v", got)
	}
	if got := first.NRGBAAt(0, 1); got.A != 0 {
		t.Fatalf("transparent pixel: %v", got)
	}
	second, err := archive.FrameImage(frames[1])
	if err != nil {
		t.Fatal(err)
	}
	if second.NRGBAAt(1, 0).A != 0 || second.NRGBAAt(2, 0) != (color.NRGBA{G: 4, A: 255}) {
		t.Fatal("native color-key conversion changed")
	}
	// Existing editable work is protected before any regeneration starts.
	if err = exportEditable(filepath.Dir(root), filepath.Base(root), false); err == nil {
		t.Fatal("existing editable sprites overwritten")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestSourceRegenerationProtectsExistingEdits(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "001", "editable.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte("hand-edited sprite metadata")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	// Refusal must happen before any native source/key is opened or decrypted.
	if err := exportEditableFromSource("missing-originals", "missing-executable", root, "", false); err == nil {
		t.Fatal("edited metadata overwritten")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatal("metadata changed")
	}
}
