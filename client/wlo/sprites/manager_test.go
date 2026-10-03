package sprites

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-go/internal/spritepack"
)

// writePack builds a one-sprite pack with a two-frame animation.
func writePack(t *testing.T, dir string) {
	t.Helper()
	frame := func(c color.NRGBA) *image.NRGBA {
		img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
		}
		return img
	}
	_, err := spritepack.WriteDir(dir, "002c", func(sink spritepack.PageSink) (*spritepack.Pack, error) {
		b := spritepack.NewBuilder("002c", sink)
		err := b.AddSprite(0, 2200, "2200.jmp", []spritepack.FrameImage{
			{Image: frame(color.NRGBA{R: 255, A: 255}), CanvasWidth: 128, CanvasHeight: 128, AnchorX: 60, AnchorY: 40},
			{Image: frame(color.NRGBA{B: 255, A: 255}), CanvasWidth: 128, CanvasHeight: 128, AnchorX: 60, AnchorY: 40},
		}, []spritepack.Animation{{Action: 0, Frames: []int{0, 1}}}, nil)
		if err != nil {
			return nil, err
		}
		return b.Finish()
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestManagerLoadsPacks(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	m := NewManager([]string{"", dir}, nil)
	a, err := m.Archive("002C")
	if err != nil {
		t.Fatal(err)
	}
	s := a.Sprite(0)
	if s == nil || s.ID != 2200 {
		t.Fatalf("sprite %+v", s)
	}
	f := s.Frame(s.Animation(0), 1)
	img, err := f.Image(m)
	if err != nil || img == nil {
		t.Fatal(err)
	}
	if c := img.NRGBAAt(img.Rect.Min.X, img.Rect.Min.Y); c != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("pixel %v", c)
	}
	if f.OffsetX != 60-64 || f.OffsetY != 40-(128-spritepack.GroundOffset) {
		t.Fatalf("offset %d,%d", f.OffsetX, f.OffsetY)
	}
	if _, err := m.Archive("absent"); err != ErrNotFound {
		t.Fatalf("missing archive: %v", err)
	}
}

func TestPlayerLoopsOnTheArchiveClock(t *testing.T) {
	dir := t.TempDir()
	writePack(t, dir)
	m := NewManager([]string{dir}, nil)
	a, _ := m.Archive("002c")
	p := NewPlayer(a, a.Sprite(0), 0)
	if p.Update(99*time.Millisecond) || p.Index() != 0 {
		t.Fatal("the frame changed early")
	}
	if !p.Update(time.Millisecond) || p.Index() != 1 {
		t.Fatal("the frame should advance after 100 ms")
	}
	p.Update(100 * time.Millisecond)
	if p.Index() != 0 {
		t.Fatal("the animation should loop")
	}
	p.Loop = false
	p.Update(250 * time.Millisecond)
	if p.Index() != 1 || p.Frame() == nil {
		t.Fatal("a non-looping animation stops on its last frame")
	}
}

// TestManagerSourcesAgree loads one archive from the editable export and
// from the built packs when both are present.
func TestManagerSourcesAgree(t *testing.T) {
	editable := filepath.Join("..", "..", "..", "data", "sprites")
	packs := filepath.Join("..", "..", "..", "var", "client-spritepacks")
	if _, err := os.Stat(filepath.Join(editable, "002h", "editable.json")); err != nil {
		t.Skip("editable export not present")
	}
	if _, err := os.Stat(filepath.Join(packs, "002h", spritepack.FileName)); err != nil {
		t.Skip("sprite packs not built")
	}
	a, err := NewManager(nil, []string{editable}).Archive("002h")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewManager([]string{packs}, nil).Archive("002h")
	if err != nil {
		t.Fatal(err)
	}
	if a.SourceBytes == 0 || a.SourceBytes != b.SourceBytes {
		t.Fatalf("source sizes %d and %d", a.SourceBytes, b.SourceBytes)
	}
	sa, sb := a.Sprite(0), b.Sprite(0)
	if sa == nil || sb == nil || len(sa.Frames) != len(sb.Frames) || len(sa.Animations) != len(sb.Animations) {
		t.Fatal("sprite shapes differ")
	}
	fa, fb := sa.Frame(sa.Animation(11), 0), sb.Frame(sb.Animation(11), 0)
	if fa.OffsetX != fb.OffsetX || fa.OffsetY != fb.OffsetY || fa.Width != fb.Width {
		t.Fatalf("placement %+v vs %+v", fa, fb)
	}
}
