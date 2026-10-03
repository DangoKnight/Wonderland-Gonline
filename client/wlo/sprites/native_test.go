package sprites

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-go/internal/spritepack"
)

// TestEditableIndicesMatchPack: read directly from the editable export,
// a sprite gets the same palette and indices as a pack built from it, once
// the archive's sprites.json has been opened.
func TestEditableIndicesMatchPack(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data", "sprites")
	if _, err := os.Stat(filepath.Join(root, "002c", spritepack.NativeFile)); err != nil {
		t.Skip("editable export not present")
	}
	const archive, slot = "002c", 0 // a small archive keeps the pack build quick
	m := NewManager(nil, []string{root})
	defer m.Close()
	a, err := m.Archive(archive)
	if err != nil {
		t.Fatal(err)
	}
	if s := a.Sprite(slot); s.Palette != nil {
		t.Fatal("indices before sprites.json is open")
	}
	m.WaitNative()
	start := time.Now()
	s := a.Sprite(slot)
	t.Logf("matched %d frames in %v", len(s.Frames), time.Since(start))
	if s.Palette == nil {
		t.Fatal("no palette after sprites.json opened")
	}

	packs := t.TempDir()
	if _, err := spritepack.WriteDir(packs, archive, func(sink spritepack.PageSink) (*spritepack.Pack, error) {
		return spritepack.BuildEditable(filepath.Join(root, archive), archive, sink)
	}); err != nil {
		t.Fatal(err)
	}
	pm := NewManager([]string{packs}, nil)
	pa, err := pm.Archive(archive)
	if err != nil {
		t.Fatal(err)
	}
	ps := pa.Sprite(slot)
	if *ps.Palette != *s.Palette {
		t.Fatal("palettes differ")
	}
	for i := range s.Frames {
		got, err := s.Frames[i].Indices(m)
		if err != nil {
			t.Fatal(err)
		}
		want, err := ps.Frames[i].Indices(pm)
		if err != nil {
			t.Fatal(err)
		}
		if (got == nil) != (want == nil) {
			t.Fatalf("frame %d: indexed %v, pack %v", i, got != nil, want != nil)
		}
		if got == nil {
			continue
		}
		for y := 0; y < got.Rect.Dy(); y++ {
			g := got.Pix[got.PixOffset(got.Rect.Min.X, got.Rect.Min.Y+y):][:got.Rect.Dx()]
			w := want.Pix[want.PixOffset(want.Rect.Min.X, want.Rect.Min.Y+y):][:want.Rect.Dx()]
			if !bytes.Equal(g, w) {
				t.Fatalf("frame %d row %d differs", i, y)
			}
		}
	}
}
