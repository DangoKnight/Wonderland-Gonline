package spritepack

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestBuilderPacksDedupesAndPaginates(t *testing.T) {
	var pages []*image.NRGBA
	b := NewBuilder("test", func(p Page) error { pages = append(pages, p.RGBA); return nil })
	b.PageSide = 64
	red, blue := solid(40, 30, color.NRGBA{R: 255, A: 255}), solid(40, 30, color.NRGBA{B: 255, A: 255})
	frames := []FrameImage{
		{Image: red, CanvasWidth: 128, CanvasHeight: 128, AnchorX: 50, AnchorY: 20},
		{Image: blue}, {Image: red}, {Image: solid(40, 40, color.NRGBA{G: 9, A: 255})},
		{}, // no pixels
	}
	if err := b.AddSprite(0, 2200, "2200.jmp", frames, []Animation{{Action: 0, Frames: []int{0, 1, 2, -1}}}, nil); err != nil {
		t.Fatal(err)
	}
	p, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	f := p.Sprites[0].Frames
	if f[0].Rect != f[2].Rect || f[0].Page != f[2].Page {
		t.Fatal("identical frames should share pixels")
	}
	if f[0].OffsetX != 50-64 || f[0].OffsetY != 20-(128-GroundOffset) {
		t.Fatalf("offsets %d,%d", f[0].OffsetX, f[0].OffsetY)
	}
	// Two 40-wide frames cannot share a 64-wide shelf; the 40-high one
	// then needs a new page.
	if len(pages) != 2 || len(p.Pages) != 2 || f[1].Page != 0 || f[3].Page != 1 {
		t.Fatalf("pages %d, last frame page %d", len(pages), f[3].Page)
	}
	if f[4].Rect.W != 0 {
		t.Fatal("an empty frame takes no space")
	}
	got := pages[f[1].Page].NRGBAAt(f[1].Rect.X+5, f[1].Rect.Y+5)
	if got != (color.NRGBA{B: 255, A: 255}) {
		t.Fatalf("pixel %v", got)
	}
	if pages[1].Bounds().Dy() >= 64 {
		t.Fatal("the last page should be trimmed to its used height")
	}
}

// TestBuilderKeepsIndices stores palette indices on index pages and keeps
// frames that look alike but recolour differently apart.
func TestBuilderKeepsIndices(t *testing.T) {
	var pages []Page
	b := NewBuilder("test", func(p Page) error { pages = append(pages, p); return nil })
	var pal [256][3]uint8
	pal[0x20], pal[0x90] = [3]uint8{9, 9, 9}, [3]uint8{9, 9, 9}
	look := solid(3, 2, color.NRGBA{9, 9, 9, 255})
	ix := func(v byte) *image.Gray {
		g := image.NewGray(image.Rect(0, 0, 3, 2))
		for i := range g.Pix {
			g.Pix[i] = v
		}
		return g
	}
	frames := []FrameImage{{Image: look, Indices: ix(0x20)}, {Image: look, Indices: ix(0x90)}, {Image: look}}
	if err := b.AddSprite(0, 1, "1.jmp", frames, nil, &pal); err != nil {
		t.Fatal(err)
	}
	p, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	f := p.Sprites[0].Frames
	if f[0].Rect == f[1].Rect || f[0].Rect == f[2].Rect || !f[0].Indexed || !f[1].Indexed || f[2].Indexed {
		t.Fatalf("frames %+v", f)
	}
	if len(p.IndexPages) != 1 || pages[0].Indices == nil || pages[0].Indices.GrayAt(f[1].Rect.X, f[1].Rect.Y).Y != 0x90 {
		t.Fatal("indices not on the index page")
	}
	if got, _ := DecodePalette(p.Sprites[0].Palette); got != pal {
		t.Fatal("palette did not round-trip")
	}
}

func TestValidateRejectsBadReferences(t *testing.T) {
	bad := []Pack{
		{Version: 2},
		{Version: Version, Pages: []string{"../x.png"}},
		{Version: Version, Sprites: []Sprite{{Index: 1}, {Index: 1}}},
		{Version: Version, Pages: []string{"a.png"}, Sprites: []Sprite{{Frames: []Frame{{Page: 1, Rect: Rect{W: 1, H: 1}}}}}},
		{Version: Version, Sprites: []Sprite{{Animations: []Animation{{Frames: []int{0}}}}}},
		{Version: Version, Pages: []string{"a.png"}, Sprites: []Sprite{{Frames: []Frame{{Rect: Rect{W: 1, H: 1}, Indexed: true}}}}},
		{Version: Version, Pages: []string{"a.png"}, IndexPages: []string{"a.png", "b.png"}},
	}
	for i, p := range bad {
		if p.Validate() == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestWriteDirRoundTripAndRefusesForeignDirectories(t *testing.T) {
	dir := t.TempDir()
	build := func(sink PageSink) (*Pack, error) {
		b := NewBuilder("002c", sink)
		if err := b.AddSprite(0, 2200, "2200.jmp", []FrameImage{{Image: solid(4, 3, color.NRGBA{R: 1, A: 255}), CanvasWidth: 8, CanvasHeight: 32}}, nil, nil); err != nil {
			return nil, err
		}
		return b.Finish()
	}
	if _, err := WriteDir(dir, "002c", build); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDir(dir, "002c", build); err != nil {
		t.Fatal("rebuilding a pack should replace it:", err)
	}
	p, err := Read(filepath.Join(dir, "002c"))
	if err != nil || p.Sprites[0].ID != 2200 || p.Sprites[0].Frames[0].Rect.W != 4 {
		t.Fatalf("read back %+v %v", p, err)
	}
	foreign := filepath.Join(dir, "other")
	os.MkdirAll(foreign, 0o755)
	os.WriteFile(filepath.Join(foreign, "keep.txt"), []byte("x"), 0o644)
	if _, err := WriteDir(dir, "other", build); err == nil {
		t.Fatal("a directory that is not a pack must not be replaced")
	}
}

// TestNativeSourcesAgree compares the editable export with the original
// archive for one archive, frame by frame, when both are installed.
func TestNativeSourcesAgree(t *testing.T) {
	root := filepath.Join("..", "..")
	editable := filepath.Join(root, "data", "sprites", "002c")
	jmaPath := filepath.Join(root, "..", "Wonderland-Client", "jma", "002c.jma")
	if _, err := os.Stat(filepath.Join(editable, EditableFile)); err != nil {
		t.Skip("editable export not present")
	}
	if _, err := os.Stat(jmaPath); err != nil {
		t.Skip("client assets not installed")
	}
	collect := func(build func(PageSink) (*Pack, error)) (*Pack, []Page) {
		var pages []Page
		p, err := build(func(pg Page) error { pages = append(pages, pg); return nil })
		if err != nil {
			t.Fatal(err)
		}
		return p, pages
	}
	a, ap := collect(func(s PageSink) (*Pack, error) { return BuildEditable(editable, "002c", s) })
	j, err := OpenJMA(jmaPath, filepath.Join(filepath.Dir(jmaPath), "002c.Jxa"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	b, bp := collect(func(s PageSink) (*Pack, error) { return BuildJMA(j, "002c", s) })
	if len(a.Sprites) != len(b.Sprites) {
		t.Fatalf("sprites %d vs %d", len(a.Sprites), len(b.Sprites))
	}
	crop := func(pages []Page, f Frame) []byte {
		var buf bytes.Buffer
		r := image.Rect(f.Rect.X, f.Rect.Y, f.Rect.X+f.Rect.W, f.Rect.Y+f.Rect.H)
		png.Encode(&buf, pages[f.Page].RGBA.SubImage(r))
		if f.Indexed {
			png.Encode(&buf, pages[f.Page].Indices.SubImage(r))
		}
		return buf.Bytes()
	}
	for i := range a.Sprites {
		sa, sb := a.Sprites[i], b.Sprites[i]
		if sa.ID != sb.ID || len(sa.Frames) != len(sb.Frames) || sa.Palette != sb.Palette {
			t.Fatalf("sprite %d differs", i)
		}
		for k := range sa.Frames {
			fa, fb := sa.Frames[k], sb.Frames[k]
			if fa.OffsetX != fb.OffsetX || fa.OffsetY != fb.OffsetY || fa.Rect.W != fb.Rect.W || fa.Rect.H != fb.Rect.H || fa.Indexed != fb.Indexed {
				t.Fatalf("%s frame %d placement differs", sa.Name, k)
			}
			if fa.Rect.W > 0 && !bytes.Equal(crop(ap, fa), crop(bp, fb)) {
				t.Fatalf("%s frame %d pixels differ", sa.Name, k)
			}
		}
	}
}
