package firstlaunch

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/internal/localization"
)

// Optional visual acceptance uses the real asset-free renderer for every step.
// Run separately from the other tests: Ebitengine supports one game per process.
func TestFirstLaunchPreview(t *testing.T) {
	dir := os.Getenv("WONDERLAND_SETUP_PREVIEW")
	if dir == "" {
		t.Skip("set WONDERLAND_SETUP_PREVIEW to save setup preview frames")
	}
	face, cjk, err := setupFont()
	if err != nil {
		t.Fatal(err)
	}
	defer face.Close()
	w := &wizard{r: Request{Language: localization.English, NeedAssets: true}, face: face, chineseFont: cjk, sourceLanguage: localization.Chinese, source: `C:\Games\Wonderland Rhodes Island`}
	if err = json.Unmarshal(messageData, &w.messages); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	g := &preview{w: w, dir: dir, t: t}
	ebiten.SetWindowSize(windowWidth, windowHeight)
	if err = ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
}

type preview struct {
	w     *wizard
	dir   string
	t     *testing.T
	frame int
}

func (g *preview) Layout(int, int) (int, int) { return windowWidth, windowHeight }
func (g *preview) Update() error {
	if g.frame >= 4 {
		return ebiten.Termination
	}
	return nil
}
func (g *preview) Draw(dst *ebiten.Image) {
	if g.frame >= 4 {
		return
	}
	names := []string{"language", "install-en", "install-zh", "ready"}
	switch g.frame {
	case 0:
		g.w.step = stepLanguage
	case 1:
		g.w.step = stepInstall
	case 2:
		g.w.r.Language = localization.Chinese
	case 3:
		g.w.step = stepReady
		g.w.r.Assets = `C:\Users\Player\AppData\Roaming\Wonderland-Gonline\assets\client-assets.zip`
	}
	g.w.Draw(dst)
	im := image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
	dst.ReadPixels(im.Pix)
	f, err := os.Create(filepath.Join(g.dir, names[g.frame]+".png"))
	if err != nil {
		g.t.Fatal(err)
	}
	if err = png.Encode(f, im); err != nil {
		g.t.Fatal(err)
	}
	f.Close()
	g.frame++
}
