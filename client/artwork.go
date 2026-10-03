package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	_ "golang.org/x/image/bmp"
	"wonderland-go/internal/clientassets"
)

func decodeImage(archive *clientassets.ImageArchive, index int) (image.Image, error) {
	b, err := archive.Read(index)
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 128_000_000 {
		return nil, fmt.Errorf("image dimensions %dx%d exceed 128-million-pixel limit", cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

func savePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

type artWorkbench struct {
	archive          *clientassets.ImageArchive
	index            int
	sourceImage      image.Image
	tiles            []artTile
	zoom, panX, panY float64
	source, snapshot string
	snapshotDone     bool
	drawError        error
	assetError       error
}

type artTile struct {
	bounds  image.Rectangle
	texture *ebiten.Image
}

func (g *artWorkbench) clearImage() {
	for _, tile := range g.tiles {
		if tile.texture != nil {
			tile.texture.Deallocate()
		}
	}
	g.tiles = nil
	g.sourceImage = nil
}

func (g *artWorkbench) load(index int) error {
	g.index = index
	img, err := decodeImage(g.archive, index)
	if err != nil {
		g.clearImage()
		g.assetError = fmt.Errorf("%s: %w", g.archive.Entries[index].Name, err)
		return nil
	}
	g.assetError = nil
	g.clearImage()
	g.sourceImage = img
	// Upload visible 1024-pixel tiles lazily rather than exceeding the GPU's
	// texture limit on the large official map panoramas.
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 1024 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 1024 {
			g.tiles = append(g.tiles, artTile{bounds: image.Rect(x, y, min(x+1024, bounds.Max.X), min(y+1024, bounds.Max.Y))})
		}
	}
	g.index = index
	g.fit()
	return nil
}
func (g *artWorkbench) fit() {
	if g.sourceImage == nil {
		return
	}
	g.zoom = min(994/float64(g.sourceImage.Bounds().Dx()), 565/float64(g.sourceImage.Bounds().Dy()))
	g.panX, g.panY = 15, 125
}
func (g *artWorkbench) Update() error {
	if g.snapshot != "" && g.assetError != nil {
		return g.assetError
	}
	if g.drawError != nil {
		return g.drawError
	}
	if g.snapshotDone {
		return ebiten.Termination
	}
	if ebiten.IsKeyPressed(ebiten.KeyLeft) {
		g.panX += 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyRight) {
		g.panX -= 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyUp) {
		g.panY += 5
	}
	if ebiten.IsKeyPressed(ebiten.KeyDown) {
		g.panY -= 5
	}
	_, wheel := ebiten.Wheel()
	if wheel > 0 {
		g.zoom = min(g.zoom*1.1, 8)
	} else if wheel < 0 {
		g.zoom = max(g.zoom/1.1, .05)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		g.fit()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageDown) {
		return g.load((g.index + 1) % len(g.archive.Entries))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyPageUp) {
		return g.load((g.index + len(g.archive.Entries) - 1) % len(g.archive.Entries))
	}
	return nil
}
func (g *artWorkbench) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{18, 23, 30, 255})
	for i := range g.tiles {
		tile := &g.tiles[i]
		x, y := g.panX+float64(tile.bounds.Min.X)*g.zoom, g.panY+float64(tile.bounds.Min.Y)*g.zoom
		if x > 1024 || y > 720 || x+float64(tile.bounds.Dx())*g.zoom < 0 || y+float64(tile.bounds.Dy())*g.zoom < 110 {
			continue
		}
		if tile.texture == nil {
			var part image.Image
			if sub, ok := g.sourceImage.(interface {
				SubImage(image.Rectangle) image.Image
			}); ok {
				part = sub.SubImage(tile.bounds)
			} else {
				crop := image.NewRGBA(image.Rect(0, 0, tile.bounds.Dx(), tile.bounds.Dy()))
				draw.Draw(crop, crop.Bounds(), g.sourceImage, tile.bounds.Min, draw.Src)
				part = crop
			}
			tile.texture = ebiten.NewImageFromImage(part)
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(g.zoom, g.zoom)
		op.GeoM.Translate(x, y)
		screen.DrawImage(tile.texture, op)
	}
	vector.FillRect(screen, 0, 0, 1024, 110, color.RGBA{25, 33, 43, 255}, false)
	w, h := 0, 0
	if g.sourceImage != nil {
		w, h = g.sourceImage.Bounds().Dx(), g.sourceImage.Bounds().Dy()
	}
	ebitenutil.DebugPrintAt(screen, "Wonderland - native artwork workbench", 15, 15)
	ebitenutil.DebugPrintAt(screen, filepath.Base(filepath.Dir(filepath.Dir(g.source)))+" / "+filepath.Base(g.source), 15, 38)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("%s | resource %d of %d | %dx%d pixels", g.archive.Entries[g.index].Name, g.index+1, len(g.archive.Entries), w, h), 15, 61)
	ebitenutil.DebugPrintAt(screen, "Page Up/Down: browse | arrows: pan | wheel: zoom | Home: fit", 15, 84)
	if g.assetError != nil {
		ebitenutil.DebugPrintAt(screen, "Unable to decode this resource:\n"+g.assetError.Error(), 15, 140)
	}
	if g.snapshot != "" && !g.snapshotDone {
		img := image.NewRGBA(screen.Bounds())
		screen.ReadPixels(img.Pix)
		g.drawError = savePNG(g.snapshot, img)
		g.snapshotDone = true
	}
}
func (*artWorkbench) Layout(int, int) (int, int) { return 1024, 720 }

func runArtwork(clientDir, archiveName, name string, list bool, export, snapshot string) error {
	if filepath.Base(archiveName) != archiveName {
		return fmt.Errorf("archive must be a filename under pic")
	}
	path := filepath.Join(clientDir, "pic", archiveName)
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	a, err := clientassets.ReadImageArchive(f, stat.Size())
	if err != nil {
		return err
	}
	if list {
		for _, e := range a.Entries {
			fmt.Printf("%s\t%d\t%d\n", e.Name, e.Offset, e.Size)
		}
		return nil
	}
	index := 0
	if name != "" {
		var found bool
		index, found = a.Find(name)
		if !found {
			return fmt.Errorf("resource %q not found", name)
		}
	}
	if export != "" {
		img, err := decodeImage(a, index)
		if err != nil {
			return err
		}
		return savePNG(export, img)
	}
	g := &artWorkbench{archive: a, source: path, snapshot: snapshot}
	if err := g.load(index); err != nil {
		return err
	}
	ebiten.SetWindowSize(1024, 720)
	ebiten.SetWindowTitle("Wonderland - native artwork workbench")
	return ebiten.RunGame(g)
}
