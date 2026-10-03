// Native Wonderland client. By default it runs the port of aLogin.exe's
// login phase (client/wlo); -legacy runs the earlier front end, and artwork
// and terrain flags open the asset workbench.
package main

import (
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"wonderland-go/internal/clientassets"
)

type workbench struct {
	ground     clientassets.GroundPrefix
	offset     int64
	zoom       float64
	panX, panY float64
	source     string
}

func (g *workbench) Update() error {
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
		g.zoom = min(g.zoom*1.1, 4)
	} else if wheel < 0 {
		g.zoom = max(g.zoom/1.1, .1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		g.zoom = .45
		g.panX, g.panY = 35, 130
	}
	return nil
}

func (g *workbench) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{18, 23, 30, 255})
	// Colors distinguish raw cell values only. They do not claim walkability.
	for x := 0; x < int(g.ground.GridWidth); x++ {
		for y := 0; y < int(g.ground.GridHeight); y++ {
			v, _ := g.ground.Cell(x, y)
			shade := color.RGBA{byte(35 + int(v)*31%160), byte(65 + int(v)*17%150), byte(90 + int(v)*11%140), 255}
			px, py := g.panX+float64(x)*20*g.zoom, g.panY+float64(y)*20*g.zoom
			if px > 1024 || py > 720 || px+20*g.zoom < 0 || py+20*g.zoom < 115 {
				continue
			}
			vector.FillRect(screen, float32(px), float32(py), float32(20*g.zoom-1), float32(20*g.zoom-1), shade, false)
		}
	}
	vector.FillRect(screen, 0, 0, 1024, 110, color.RGBA{25, 33, 43, 255}, false)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("Wonderland native asset workbench\n%s | offset %d | %d x %d pixels | %d x %d cells\n%d verified prefix bytes | %d layer references | raw terrain values (artwork not loaded)\nArrow keys: pan | mouse wheel: zoom | Home: reset", g.source, g.offset, g.ground.Width, g.ground.Height, g.ground.GridWidth, g.ground.GridHeight, g.ground.BytesRead, len(g.ground.Layers)))
	mx, my := ebiten.CursorPosition()
	x, y := int((float64(mx)-g.panX)/(20*g.zoom)), int((float64(my)-g.panY)/(20*g.zoom))
	if float64(mx) >= g.panX && float64(my) >= g.panY && my >= 115 {
		if v, ok := g.ground.Cell(x, y); ok {
			ebitenutil.DebugPrintAt(screen, fmt.Sprintf("Cell (%d,%d): raw value %d", x, y, v), 15, 690)
		}
	}
}

func (*workbench) Layout(int, int) (int, int) { return 1024, 720 }

func main() {
	sprites := flag.String("sprites", "", "optional sprite pack or editable directory override")
	assets := flag.String("assets", defaultAssetDir(), "decompiled data directory")
	clientDir := flag.String("client", "", "original client directory for the workbench or -legacy (required)")
	archive := flag.String("archive", "map.JMG", "BMg/JMg filename under the client pic directory")
	name := flag.String("name", "", "resource filename; defaults to first entry")
	list := flag.Bool("list", false, "list indexed image resources without opening a window")
	export := flag.String("export", "", "export selected decoded image as PNG without opening a window")
	snapshot := flag.String("snapshot", "", "save an Ebitengine-rendered frame as PNG, then exit")
	terrain := flag.Bool("terrain", false, "inspect the verified Ground.MMG prefix instead of artwork")
	path := flag.String("ground", "", "terrain archive override; defaults to <client>/data/Ground.MMG")
	offset := flag.Int64("offset", 0, "verified byte offset of a Ground.MMG record; no map ID is inferred")
	useWorkbench := flag.Bool("workbench", false, "open the artwork workbench instead of the game")
	serverINI := flag.String("serverini", "", "server list; defaults to SERVER.INI in the client directory (the working directory with -legacy); a single local server is used when missing")
	legacy := flag.Bool("legacy", false, "run the earlier screenshot-based front end instead of the decompile port")
	flag.Parse()
	// Any workbench option selects the workbench.
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "archive", "name", "list", "export", "terrain", "ground", "offset":
			*useWorkbench = true
		}
	})
	if (*useWorkbench || *legacy) && *clientDir == "" {
		log.Fatal("-client must point to an original sibling client directory for inspection modes")
	}
	if !*useWorkbench && *legacy {
		if *serverINI == "" {
			*serverINI = "SERVER.INI"
		}
		// The earlier front end reads the original files directly.
		if err := runFrontend(*clientDir, *serverINI, *snapshot); err != nil {
			log.Fatal(err)
		}
		return
	}
	if !*useWorkbench {
		if err := runClientWithSprites(*assets, *serverINI, *snapshot, *sprites); err != nil {
			log.Fatal(err)
		}
		return
	}
	// The workbench inspects the original archives.
	native := *clientDir
	if *path == "" {
		*path = filepath.Join(native, "data", "Ground.MMG")
	}
	if !*terrain {
		if err := runArtwork(native, *archive, *name, *list, *export, *snapshot); err != nil {
			log.Fatal(err)
		}
		return
	}
	f, err := os.Open(*path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		log.Fatal(err)
	}
	if *offset < 0 || *offset >= stat.Size() {
		log.Fatal("offset outside archive")
	}
	// The verified prefix can contain at most 900*900 grid bytes plus headers.
	buf := make([]byte, min(int64(812000), stat.Size()-*offset))
	n, err := f.ReadAt(buf, *offset)
	if err != nil {
		log.Fatal(err)
	}
	ground, err := clientassets.DecodeGroundPrefix(buf[:n])
	if err != nil {
		log.Fatal(err)
	}
	ebiten.SetWindowSize(1024, 720)
	ebiten.SetWindowTitle("Wonderland — native asset workbench")
	if err := ebiten.RunGame(&workbench{ground: ground, offset: *offset, zoom: .45, panX: 35, panY: 130, source: *path}); err != nil {
		log.Fatal(err)
	}
}

func defaultAssetDir() string {
	// Installed distributions keep assets beside the executable. Source runs
	// accept either the module directory or repository root as working directory.
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Join(filepath.Dir(exe), "data")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	for _, dir := range []string{"data", filepath.Join("..", "data")} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return "data"
}
