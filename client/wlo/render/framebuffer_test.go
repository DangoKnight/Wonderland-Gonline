package render

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"image"
	"os"
	"reflect"
	"testing"
	"wonderland-gonline/client/wlo/surface"
)

func TestDirtyRegions(t *testing.T) {
	const w, h = 67, 41
	previous, pixels := make([]uint16, w*h), make([]uint16, w*h)
	if got := changedRects(nil, pixels, previous, w, h, true); !reflect.DeepEqual(got, []image.Rectangle{image.Rect(0, 0, w, h)}) {
		t.Fatal(got)
	}
	if got := changedRects(nil, pixels, previous, w, h, false); len(got) != 0 {
		t.Fatal(got)
	}
	pixels[40*w+66] = 1
	if got := changedRects(nil, pixels, previous, w, h, false); !reflect.DeepEqual(got, []image.Rectangle{image.Rect(64, 32, 67, 41)}) {
		t.Fatal(got)
	}
	pixels[0], pixels[32] = 2, 3
	if got := changedRects(nil, pixels, previous, w, h, false); !reflect.DeepEqual(got, []image.Rectangle{image.Rect(0, 0, w, h)}) {
		t.Fatal(got)
	}
}
func TestEncodeClippedRows(t *testing.T) {
	pixels := []uint16{0, 0xf800, 0x07e0, 0x001f, 0xffff, 0x1234}
	r := image.Rect(1, 0, 3, 2)
	gpu, cpu := make([]byte, 16), make([]byte, 16)
	encode(gpu, pixels, 3, r, true)
	encode(cpu, pixels, 3, r, false)
	want := []byte{0, 0xf8, 0, 255, 0xe0, 7, 0, 255, 255, 255, 0, 255, 0x34, 0x12, 0, 255}
	if !reflect.DeepEqual(gpu, want) {
		t.Fatalf("%x", gpu)
	}
	want = []byte{255, 0, 0, 255, 0, 255, 0, 255, 255, 255, 255, 255, 16, 69, 165, 255}
	if !reflect.DeepEqual(cpu, want) {
		t.Fatalf("%x", cpu)
	}
}
func TestModes(t *testing.T) {
	for _, mode := range []string{"", Auto, CPU, GPU} {
		p, err := New(mode)
		if err != nil {
			t.Fatal(err)
		}
		if mode == CPU && p.Mode() != CPU || mode != CPU && p.Mode() != GPU {
			t.Fatal(p.Mode())
		}
		p.Close()
	}
	if _, err := New("invalid"); err == nil {
		t.Fatal("accepted invalid renderer")
	}
}

// Explicit graphics-context test, separate from ordinary unit runs:
// WONDERLAND_TEST_GPU=1 go test ./wlo/render -run TestGPUColorParity -count=1
func TestGPUColorParity(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1 for graphics readback")
	}
	gpu, err := New(GPU)
	if err != nil {
		t.Fatal(err)
	}
	defer gpu.Close()
	cpu, err := New(CPU)
	if err != nil {
		t.Fatal(err)
	}
	defer cpu.Close()
	s := surface.New(256, 256)
	for i := range s.Pix {
		s.Pix[i] = uint16(i)
	}
	g := &parityGame{gpu: gpu, cpu: cpu, src: s}
	ebiten.SetWindowSize(256, 256)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if g.stage != 4 {
		t.Fatalf("only %d frames checked", g.stage)
	}
}

type parityGame struct {
	gpu, cpu          *Presenter
	src               *surface.Surface
	target, reference *ebiten.Image
	stage             int
	drawn             bool
	err               error
}

func (g *parityGame) Layout(int, int) (int, int) { return 256, 256 }
func (g *parityGame) Update() error {
	if g.err != nil {
		return ebiten.Termination
	}
	if !g.drawn {
		return nil
	}
	g.drawn = false
	for _, target := range []*ebiten.Image{g.target, g.reference} {
		got := make([]byte, g.src.W*g.src.H*4)
		target.ReadPixels(got)
		for i, v := range g.src.Pix {
			r, green, b := byte(v>>11), byte(v>>5&63), byte(v&31)
			want := [4]byte{r<<3 | r>>2, green<<2 | green>>4, b<<3 | b>>2, 255}
			for c := range want {
				if got[i*4+c] != want[c] {
					g.err = fmt.Errorf("stage %d pixel %d channel %d: got %d want %d", g.stage, i, c, got[i*4+c], want[c])
					return ebiten.Termination
				}
			}
		}
	}
	expected := g.src.W * g.src.H * 4
	switch g.stage {
	case 1:
		expected = 0
	case 2:
		expected = 32 * 32 * 4
	}
	if g.gpu.UploadedBytes != expected || g.cpu.UploadedBytes != expected {
		g.err = fmt.Errorf("stage %d: upload gpu=%d cpu=%d want=%d", g.stage, g.gpu.UploadedBytes, g.cpu.UploadedBytes, expected)
		return ebiten.Termination
	}
	g.stage++
	switch g.stage {
	case 2:
		g.src.Pix[len(g.src.Pix)-1] = 0
	case 3:
		g.src = surface.New(67, 41)
		for i := range g.src.Pix {
			g.src.Pix[i] = uint16(i * 23)
		}
		g.target.Deallocate()
		g.reference.Deallocate()
		g.target, g.reference = nil, nil
	case 4:
		return ebiten.Termination
	}
	return nil
}
func (g *parityGame) Draw(screen *ebiten.Image) {
	if g.stage == 0 && !g.drawn {
		if err := exerciseRaster(g.gpu.device); err != nil {
			g.err = err
		}
	}
	if g.drawn {
		screen.DrawImage(g.target, nil)
		return
	}
	if g.target == nil {
		g.target = ebiten.NewImage(g.src.W, g.src.H)
		g.reference = ebiten.NewImage(g.src.W, g.src.H)
	}
	g.gpu.Draw(g.target, g.src)
	g.cpu.Draw(g.reference, g.src)
	screen.DrawImage(g.target, nil)
	g.drawn = true
}
