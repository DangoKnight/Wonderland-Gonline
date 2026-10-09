package app

import (
	"fmt"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/internal/clientfs"
)

// Requires a graphics context and installed exports or compiled packs. CPU and
// GPU windows run the same deterministic login/world/UI states without dialing.
const gpuParityStages = 20

func TestGPUClientParity(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1")
	}
	// RunGame owns process-global graphics state and shuts down the native
	// backend on return. Keep that lifecycle out of subsequent CPU/UI tests.
	if os.Getenv("WONDERLAND_TEST_GPU_CHILD") == "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestGPUClientParity$", "-test.v", "-test.timeout=5m")
		cmd.Env = append(os.Environ(), "WONDERLAND_TEST_GPU_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GPU parity subprocess: %v\n%s", err, output)
		}
		t.Logf("GPU parity subprocess:\n%s", output)
		return
	}
	root := filepath.Join("..", "..", "..", "data")
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		var closePacks func() error
		var err error
		root, closePacks, err = clientfs.Mount(bundle)
		if err != nil {
			t.Fatal(err)
		}
		defer closePacks()
	}
	r := &Resources{}
	defer r.Close()
	o := Options{Root: root, Shared: r, UserRoot: t.TempDir(), ServerINI: filepath.Join(t.TempDir(), "SERVER.INI"), FallbackINI: []byte("01[Local]1\r\nLocal1*127.0.0.1\r\n")}
	makeClient := func(o Options) (*Client, error) {
		c, err := New(o)
		if err != nil {
			return nil, err
		}
		c.Now = func() time.Time { return time.Unix(0, 0) }
		c.MainStatus.Now = c.Now
		c.Net.Dial = func(string, string) (net.Conn, error) { return nil, errNoNetwork }
		c.Input.X, c.Input.Y = -100, -100
		return c, nil
	}
	c, err := makeClient(o)
	if err != nil {
		t.Fatal(err)
	}
	o.UserRoot = t.TempDir()
	second, err := makeClient(o)
	if err != nil {
		t.Fatal(err)
	}
	cpu, gpu := NewWorkspace(c), NewWorkspace(second)
	defer cpu.Close()
	defer gpu.Close()
	cpu.factory, gpu.factory = makeClient, makeClient
	p, err := render.New(render.GPU)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	gpu.attachRenderer(p.Attach)
	g := &gpuParityGame{cpu: cpu, gpu: gpu, presenter: p}
	ebiten.SetWindowSize(workspaceWidth, ScreenHeight)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatal(err)
	}
	if g.err != nil {
		t.Fatal(g.err)
	}
	if g.stage != gpuParityStages {
		t.Fatalf("only %d states checked", g.stage)
	}
}

type gpuParityGame struct {
	cpu, gpu  *Workspace
	presenter *render.Presenter
	stage     int
	drawn     bool
	err       error
}

func (g *gpuParityGame) Layout(int, int) (int, int) { return workspaceWidth, ScreenHeight }
func (g *gpuParityGame) Update() error {
	if g.err != nil {
		return ebiten.Termination
	}
	if g.drawn {
		g.stage++
		g.drawn = false
	}
	if g.stage == gpuParityStages {
		return ebiten.Termination
	}
	return nil
}
func (g *gpuParityGame) Draw(screen *ebiten.Image) {
	if g.drawn {
		return
	}
	before := g.presenter.Stats().Readbacks
	for _, w := range []*Workspace{g.cpu, g.gpu} {
		c := w.Current()
		switch g.stage {
		case 14:
			w.Switch(0)
			c = w.Current()
			enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
			enemy[2] = 2
			for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), remoteRecord(5, 4, 3, 2, 17162, 10001), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
				c.dispatch(p)
			}
			c.Frame()
			c.Sprites.WaitNative()
		case 15:
			c.dispatch([]byte{50, 1, 17, 0, 4, 2, 0x11, 0x27, 0, 1, 2, 2, 1, 0, 1, 25, 35, 0, 0, 0, 2})
			c.Frame()
			c.Now = func() time.Time { return time.Unix(0, 0).Add(800 * time.Millisecond) }
			c.Input.X, c.Input.Y = 230, 370
		case 16:
			motion := c.battle.assets.Motions[10001]
			hitAt := motion.EventAt(motion.Events[0]) + 10*time.Millisecond
			c.Now = func() time.Time { return time.Unix(0, 0).Add(hitAt) }
		case 17:
			c.Now = func() time.Time { return time.Unix(0, 0).Add(time.Minute) }
			c.dispatch([]byte{50, 6, 4, 2, 0})
			c.dispatch([]byte{52, 1})
		case 18:
			c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
		case 19:
			n := c.World.NPCs[1]
			if n == nil {
				g.err = fmt.Errorf("roaming fixture NPC is missing")
				return
			}
			n.X, n.Y, n.Shown = 1042, 1075, true
			c.dispatch([]byte{22, 2, 1, 0, 0xb0, 4, 0x33, 4, 2})
			c.Now = func() time.Time { return time.Unix(0, 0).Add(time.Minute + time.Second) }
		case 1:
			c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, []uint16{22003, 21002, 24002}, "Tester"))
			if c.World == nil {
				g.err = fmt.Errorf("world entry failed")
				return
			}
			freezeGPUFixtureAnimations(c)
			c.Frame()
			c.Sprites.WaitNative()
		case 2:
			c.Inventory.Show()
		case 3:
			c.Inventory.Hide()
			c.Skills.Show()
		case 4:
			c.Skills.Hide()
			c.Settings.Show()
		case 5:
			c.Settings.Hide()
			if err := w.Add(); err != nil {
				g.err = err
				return
			}
			w.Current().dispatch(selfPacket(10002, 2, 11016, 982, 1036, 0, 444444444, 444444444, nil, "Observer"))
			freezeGPUFixtureAnimations(w.Current())
			w.Current().Frame()
			w.Current().Sprites.WaitNative()
		case 8:
			w.Collapsed = false
			w.slide = 0
			for len(w.Sessions) < 5 {
				if err := w.Add(); err != nil {
					g.err = err
					return
				}
			}
		case 9:
			w.scrollTo(54, true)
		case 10:
			w.scrollTo(w.maxScrollPixels(), true)
		case 13:
			c.vehiclePacket([]byte{15, 15, 0x11, 0x27, 0, 0, 0x90, 0xbb})
			if len(c.vehicleEffects.effects) != 1 {
				g.err = fmt.Errorf("raft break fixture failed")
				return
			}
		case 12:
			w.beginTitle(w.Active)
			for _, r := range "Raft testing" {
				w.titleChar(r)
			}
		case 11:
			w.Switch(0)
			c = w.Current()
			c.vehiclePacket([]byte{15, 10, 1, 0x11, 0x27, 0, 0, 0x90, 0xbb})
			if c.World.Player.VehicleID != 48016 {
				g.err = fmt.Errorf("raft mount fixture failed")
				return
			}
		case 7:
			w.Remove(2)
		case 6:
			w.Switch(0)
			w.Collapsed = true
			w.slide = sessionSlideTicks
		}
		w.Tick()
	}
	want := g.cpu.Compose().RGBA()
	gpuSurface := g.gpu.Compose()
	if gpuSurface.Pix != nil {
		g.err = fmt.Errorf("GPU composition has CPU framebuffer")
		return
	}
	if g.presenter.Stats().Readbacks != before {
		g.err = fmt.Errorf("rendering performed a CPU readback")
		return
	}
	if g.stage == 7 && g.presenter.Stats().Targets != 2 {
		g.err = fmt.Errorf("removed session leaked target: %+v", g.presenter.Stats())
		return
	}
	got := gpuSurface.RGBA()
	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			if dir := os.Getenv("GPU_DIFF_DIR"); dir != "" {
				_ = os.MkdirAll(dir, 0755)
				for name, m := range map[string]*Workspace{"cpu": g.cpu, "gpu": g.gpu} {
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("stage-%d-%s.png", g.stage, name)))
					if err == nil {
						_ = png.Encode(f, m.screen.RGBA())
						f.Close()
					}
				}
			}
			g.err = fmt.Errorf("stage %d byte %d (pixel %d,%d channel %d): GPU %d CPU %d", g.stage, i, i/4%workspaceWidth, i/4/workspaceWidth, i%4, got.Pix[i], want.Pix[i])
			return
		}
	}
	g.presenter.Draw(screen, gpuSurface)
	g.drawn = true
}

// Actor clocks are independent of Client.Now in the legacy presentation model.
// Fix them before the first draw so slow/race-instrumented runs compare the same
// animation frames rather than two successive wall-clock poses.
func freezeGPUFixtureAnimations(c *Client) {
	if c.World == nil {
		return
	}
	if body, ok := c.World.Body.(*role.Human); ok {
		body.Now = c.Now
	}
	for _, npc := range c.World.NPCs {
		if painter, ok := npc.Painter.(*role.NPC); ok {
			painter.Now = c.Now
		}
	}
}
