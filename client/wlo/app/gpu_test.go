package app

import (
	"fmt"
	"image"
	"image/png"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// Requires a graphics context and installed exports or compiled packs. CPU and
// GPU windows run the same deterministic login/world/UI states without dialing.
const gpuParityStages = 101

func isolateGPUFixture(t *testing.T) bool {
	t.Helper()
	// RunGame owns process-global graphics state and shuts down the native
	// backend on return. Keep that lifecycle out of subsequent CPU/UI tests.
	if os.Getenv("WONDERLAND_TEST_GPU_CHILD") == "" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v", "-test.timeout=5m")
		cmd.Env = append(os.Environ(), "WONDERLAND_TEST_GPU_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GPU fixture subprocess: %v\n%s", err, output)
		}
		t.Logf("GPU fixture subprocess:\n%s", output)
		return true
	}
	return false
}

func TestGPUClientParity(t *testing.T) {
	if os.Getenv("WONDERLAND_TEST_GPU") == "" {
		t.Skip("set WONDERLAND_TEST_GPU=1")
	}
	if isolateGPUFixture(t) {
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
		case 90:
			c.stopMapLoading()
			c.sceneFade.mode = sceneFadeNone
			c.dispatch(mallCatalogFixture())
			c.dispatch([]byte{75, 7, 1})
			c.Mall.Show()
		case 91:
			c.Mall.AddRow(0)
		case 92:
			c.Mall.ShowCatalog()
			c.Mall.ShowContents(55134)
		case 93:
			c.Mall.ShowContents(34172)
		case 94:
			c.Mall.ContentsForm.Hide()
			c.Mall.TryOn(26017)
			c.Mall.TryOn(26013)
		case 95:
			c.Mall.Tabs[2].Click()
		case 96:
			c.Mall.Tabs[3].Click()
		case 97:
			c.Mall.Tabs[5].Click()
		case 98:
			c.Mall.Next.Click()
		case 99:
			c.InventoryState.Bag[0] = game.Item{ID: 12101, Count: 1}
			c.Mall.Tabs[6].Click()
			c.Mall.LeftUp(0, c.Mall.Left+275, c.Mall.Top+51)
		case 100:
			c.Mall.Tabs[2].Click()
			c.Mall.TryOn(12101)
		case 86:
			c.mapReady = false
			c.startMapLoading()
		case 87:
			at := c.Now().Add(mapLoadingLeave + mapLoadingShow)
			c.Now = func() time.Time { return at }
		case 88:
			c.mapReady = true
			at := c.Now().Add(mapLoadingHold)
			c.Now = func() time.Time { return at }
		case 89:
			at := c.Now().Add(mapLoadingHide + mapLoadingReveal/2)
			c.Now = func() time.Time { return at }
		case 84:
			c.World.Areas = []world.Area{{ID: 1, Light: world.LargeLight, LightAt: image.Pt(c.World.Player.X+80, c.World.Player.Y)}}
		case 85:
			at := c.Now().Add(16 * time.Millisecond)
			c.Now = func() time.Time { return at }
		case 82, 83:
			if g.stage == 82 {
				c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Tester"))
				freezeGPUFixtureAnimations(c)
			}
			c.World.CameraAt = nil
			c.World.Step(c.Now())
			c.World.Player.X += 3
			at := c.Now().Add(16 * time.Millisecond)
			c.Now = func() time.Time { return at }
			c.World.Step(at)
			x, _ := c.World.CameraPosition()
			if x == float64(int(x)) {
				g.err = fmt.Errorf("subpixel camera fixture did not move fractionally")
				return
			}
			if g.stage == 83 {
				c.World.Marker.Show(image.Pt(c.World.Player.X, c.World.Player.Y))
				c.World.Marker.SetHeld(true, at)
				c.World.TrackHeldMarker(320, 360)
			}
		case 79:
			c.Inventory.UseDialog.Hide()
			c.Inventory.Hide()
			c.worldZoom.level = worldZoomMinimum
		case 80:
			c.worldZoom.level = worldZoomMaximum
		case 81:
			c.worldZoom.level = worldZoomDefault
		case 73:
			c.Input.X, c.Input.Y, c.Input.Hovered = -100, -100, nil
			c.InventoryState.Bag = game.Inventory{{ID: game.ItemPotentialPill, Count: 50}}
			c.InventoryState.Pets[2] = inventory.UsePet{ID: 12178, Name: []byte("Robinson"), Stats: world.Stats{Level: 1, STR: 15, CON: 20, INT: 5, WIS: 8, AGI: 12}}
			c.Stats.Potential = 0
			c.inventoryPacket([]byte{23, 5})
			c.Inventory.Show()
			c.Inventory.Use(1, false)
			if c.Inventory.UseDialog == nil || c.Inventory.UseDialog.Image < 0 || c.Pics.Find("PotentialFire1") < 0 || c.Pics.Find("PotentialFire2") < 0 {
				g.err = fmt.Errorf("native Potential window assets missing")
				return
			}
		case 74:
			c.Inventory.UseDialog.SelectTarget(0)
		case 75:
			c.Inventory.UseDialog.SelectTarget(3)
		case 76:
			a := c.Inventory.UseDialog.Cancel.Abs()
			c.Input.X, c.Input.Y, c.Input.Hovered = a.X+8, a.Y+8, nil
		case 77:
			a := c.Inventory.UseDialog.Confirm.Abs()
			c.Input.X, c.Input.Y, c.Input.Hovered = a.X+8, a.Y+8, nil
		case 78:
			c.InventoryState.Pets[2].Potential = 1
			c.InventoryState.Bag[0].Count = 49
		case 72:
			at := c.floorItemPoint(c.floorItems[1])
			c.Input.X, c.Input.Y = at.X, at.Y
		case 71:
			c.worldItemFlights = nil
			c.World.Player.X, c.World.Player.Y = 1322, 895
		case 69:
			c.battle.state.Active = false
			c.resetBattleResults()
			c.dispatch(selfPacket(10001, 2, 11016, 562, 635, 0, 444444444, 444444444, nil, "Dango"))
			freezeGPUFixtureAnimations(c)
			c.dispatch([]byte{5, 4})
			c.dispatch([]byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0})
			c.dispatch([]byte{20, 8})
		case 70:
			c.dispatch([]byte{23, 2, 2, 0, 1})
			c.dispatch([]byte{23, 6, 106, 160, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
			at := c.Now().Add(300 * time.Millisecond)
			c.Now = func() time.Time { return at }
		case 67:
			at := c.Now().Add(time.Second)
			c.Now = func() time.Time { return at }
			c.battleFrame(false)
			c.dispatch([]byte{50, 1, 11, 0, 4, 2, 142, 234, 0, 1, 4, 2, 1, 0, 0})
		case 68:
			cell := clientbattle.Cell{X: 4, Y: 2}
			motion := c.battle.assets.Motions[60046]
			for elapsed := time.Duration(0); elapsed < motion.Duration(); elapsed += 10 * time.Millisecond {
				pose, _ := motion.PoseAt(cell, clientbattle.Cell{}, elapsed, 0)
				if pose == 26 || pose == 27 {
					at := c.battle.playing[0].at.Add(elapsed)
					c.Now = func() time.Time { return at }
					break
				}
			}
		case 60:
			at := c.Now().Add(5 * time.Second)
			c.Now = func() time.Time { return at }
			c.battleFrame(false) // Complete the previous contact fixture.
			incoming := remoteRecord(5, 4, 3, 2, 12032, 10001)
			incoming[2], incoming[8] = 5, 2
			for _, p := range [][]byte{{11, 1, 3, 2}, incoming, {50, 1, 17, 0, 2, 2, 17, 39, 0, 1, 4, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}} {
				c.dispatch(p)
			}
		case 61:
			motion := battleArrivalMotion()
			at := c.battle.arrivals[clientbattle.Cell{X: 3, Y: 2}].Add((motion.Duration() - motion.ReturnStart()) / 2)
			c.Now = func() time.Time { return at }
		case 62:
			at := c.battle.petDepartures[clientbattle.Cell{X: 3, Y: 2}].at.Add(c.battleDepartureMotion().Duration())
			c.Now = func() time.Time { return at }
		case 63:
			at := c.Now().Add(time.Millisecond)
			c.Now = func() time.Time { return at }
		case 64:
			at := c.Now().Add(5 * time.Second)
			c.Now = func() time.Time { return at }
		case 65:
			c.dispatch([]byte{50, 6, 4, 2, 0})
			c.dispatch([]byte{52, 1})
		case 66:
			at := c.Now().Add(time.Second)
			c.Now = func() time.Time { return at }
		case 53:
			c.Team.Hide()
			c.dispatch([]byte{11, 1, 3, 2, 0})
			c.dispatch(remoteRecord(5, 4, 3, 2, 14162, 10001))
			c.dispatch([]byte{50, 1, 17, 0, 2, 2, 17, 39, 0, 1, 3, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1})
		case 54:
			motion := battleArrivalMotion()
			at := c.battle.arrivals[clientbattle.Cell{X: 3, Y: 2}].Add((motion.Duration() - motion.ReturnStart()) / 2)
			c.Now = func() time.Time { return at }
		case 55:
			at := c.battle.petDepartures[clientbattle.Cell{X: 3, Y: 2}].at.Add(c.battleDepartureMotion().Duration())
			c.Now = func() time.Time { return at }
		case 56:
			at := c.Now().Add(time.Millisecond)
			c.Now = func() time.Time { return at }
		case 57, 58, 59:
			motion := c.battle.assets.Motions[10001]
			elapsed := motion.EventAt(motion.Events[0])
			if g.stage == 57 {
				elapsed /= 2
			}
			if g.stage == 59 {
				elapsed = motion.Duration() + time.Second
			}
			at := c.battle.playing[0].at.Add(elapsed)
			c.Now = func() time.Time { return at }
		case 50:
			c.resetBattleResults()
			c.dispatch([]byte{20, 12})
			c.dispatch([]byte{6, 2, 1})
			enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
			enemy[2] = 2
			for _, packet := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), remoteRecord(5, 4, 3, 2, 12032, 10001), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
				c.dispatch(packet)
			}
			at := c.Now().Add(2 * sceneFadeSteps * sceneFadeFrame)
			c.Now = func() time.Time { return at }
			c.sceneFadeTick(at)
			c.MainButtons.Buttons[mainSocialButton].OnClick()
		case 51:
			c.Social.Hide()
			c.MainButtons.Buttons[mainInventoryButton].OnClick()
		case 52:
			c.Inventory.Hide()
			c.TeamState.BattlePet = 12032
			c.InventoryState.Pets[1].Amity = 60
			c.MainButtons.Buttons[mainTeamButton].OnClick()
		case 46:
			c.dispatch([]byte{8, 1, 36, 1, 232, 3, 0, 0, 0, 0, 0, 0})
			c.dispatch([]byte{26, 4, 244, 1, 0, 0})
			c.dispatch(resultItemPacket(50, 300, 3))
			c.dispatch([]byte{11, 12, 1})
			c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
		case 47:
			at := c.Now().Add(sceneFadeSteps * sceneFadeFrame)
			c.Now = func() time.Time { return at }
		case 48:
			lines := append([]string(nil), c.battleResults.lines...)
			for i := 0; i < 20; i++ {
				lines = append(lines, fmt.Sprintf("Material %02d x1", i))
			}
			c.battleResults.form.setLines(lines)
		case 49:
			f := c.battleResults.form
			f.list.SetTop(len(f.list.Items) - f.list.RowsShown)
		case 42:
			enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
			enemy[2] = 2
			c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
			c.dispatch(enemy)
			at := c.Now().Add(2 * sceneFadeSteps * sceneFadeFrame)
			c.Now = func() time.Time { return at }
			c.sceneFadeTick(at)
			c.combat.enabled = true
			data, err := protocol.EncodeCombatRound([]protocol.CombatStep{
				{Packets: [][]byte{{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}}, Delay: time.Second},
				{Packets: [][]byte{{51, 1, 4, 2, 26, 7, 0, 0, 0}}},
			})
			if err != nil {
				g.err = err
				return
			}
			chunks, err := protocol.CombatRoundChunks(100, data)
			if err != nil {
				g.err = err
				return
			}
			for _, p := range chunks {
				c.dispatch(p)
			}
			c.dispatch([]byte{254, 3, 101, 0, 0, 0, 4, 2})
		case 43:
			motion := c.battle.assets.Motions[10001]
			at := c.battle.playing[0].at.Add(motion.EventAt(motion.Events[0]))
			c.Now = func() time.Time { return at }
		case 44:
			at := c.Now().Add(time.Minute)
			c.Now = func() time.Time { return at }
		case 45:
			at := c.Now().Add(time.Millisecond)
			c.Now = func() time.Time { return at }
		case 37:
			c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1})
		case 38:
			motion := c.battle.assets.Motions[10001]
			at := c.battle.playing[0].at.Add(motion.EventAt(motion.Events[0]))
			c.Now = func() time.Time { return at }
			c.dispatch([]byte{53, 3, 2, 2})
		case 39, 40:
			at := c.Now().Add(150 * time.Millisecond)
			c.Now = func() time.Time { return at }
		case 41:
			at := c.Now().Add(time.Minute)
			c.Now = func() time.Time { return at }
		case 33:
			enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
			enemy[2] = 2
			c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
			c.dispatch(enemy)
		case 34:
			// The battle field is already visible; sample its initial reveal.
			at := c.Now()
			c.Now = func() time.Time { return at }
		case 35:
			at := c.Now().Add(7 * sceneFadeFrame)
			c.Now = func() time.Time { return at }
		case 36:
			at := c.Now().Add(8 * sceneFadeFrame)
			c.Now = func() time.Time { return at }
		case 26:
			c.Input.X, c.Input.Y = -100, -100
			enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
			enemy[2] = 2
			c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
			c.dispatch(enemy)
			at := c.Now().Add(2 * sceneFadeSteps * sceneFadeFrame)
			c.Now = func() time.Time { return at }
			c.sceneFadeTick(at)
		case 27:
			c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1})
			c.dispatch([]byte{53, 1, 2, 2})
			c.dispatch([]byte{11, 12, 1})
			c.dispatch([]byte{11, 1, 4, 2, 0})
			c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
		case 28:
			motion := c.battle.assets.Motions[10001]
			at := c.battle.playing[0].at.Add(motion.EventAt(motion.Events[0]))
			c.Now = func() time.Time { return at }
		case 29:
			at := c.Now().Add(time.Minute)
			c.Now = func() time.Time { return at }
		case 30, 31:
			cell := clientbattle.Cell{X: 4, Y: 2}
			duration := c.battleDepartureMotion().Duration()
			if g.stage == 30 {
				duration /= 4
			}
			at := c.battle.departures[cell].Add(duration)
			c.Now = func() time.Time { return at }
		case 32:
			at := c.Now().Add(sceneFadeSteps * sceneFadeFrame)
			c.Now = func() time.Time { return at }
		case 23:
			c.LuckyDraw.Hide()
			c.InventoryState.Pets[1] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Stats: world.Stats{Level: 3, HP: 100, MaxHP: 140, SP: 20, MaxSP: 40}}
			c.dispatch([]byte{19, 1, 0, 47, 0, 0})
			c.Frame()
			c.Sprites.WaitNative()
		case 24:
			c.dispatch([]byte{8, 2, 4, 2, 0, 25, 1, 20, 0, 0, 0, 0, 0, 0, 0})
			c.dispatch([]byte{8, 2, 4, 2, 0, 26, 1, 5, 0, 0, 0, 0, 0, 0, 0})
			c.Input.X, c.Input.Y = 195, 60
		case 25:
			c.dispatch([]byte{19, 2})
		case 21:
			c.Talk.Hide()
			c.Social.Hide()
			c.mapReady = true
			c.dispatch([]byte{104, 1, 1, 0, 2, 176, 125, 2, 216, 132, 3})
			c.dispatch([]byte{104, 1, 2, 2, 1})
			c.LuckyDraw.Show()
		case 22:
			catalog := []byte{104, 1, 1, 1, 20}
			for i := 0; i < 20; i++ {
				catalog = append(catalog, 216, 132, byte(i+1))
			}
			c.dispatch(catalog)
			c.LuckyDraw.Preview.Open()
		case 20:
			c.Social.Show()
			c.Social.SelectTab(hud.SocialSetUp)
			c.confirm([]byte("Ensure your data is correct. System will be sending bday gift on specified date. Log in game to receive it."), func() {})
			start := c.Now()
			for i := 1; i <= 10; i++ {
				at := start.Add(time.Duration(i) * 20 * time.Millisecond)
				c.Now = func() time.Time { return at }
				c.Frame()
			}
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
			c.World.Player.Body, c.World.Player.Head = 2, 1
			world.Dress(c.World.Body, c.World.Player)
			freezeGPUFixtureAnimations(c)
			// Installed used-arcade speech, including its trailing event flag.
			c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 7, 0, 0, 1, 0, 0, 0, 0, 156, 119, 1})
			start := c.Now()
			for i := 1; i <= 6; i++ {
				at := start.Add(time.Duration(i) * 16 * time.Millisecond)
				c.Now = func() time.Time { return at }
				c.Frame()
			}
			// Force a blink while the same player-art line remains open.
			at := start.Add(9 * time.Second)
			c.Now = func() time.Time { return at }
			c.Frame()
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
		if g.stage >= 79 && !worldZoomEnabled && (c.World.ViewWidth != 0 || c.worldZoom.level != worldZoomDefault) {
			g.err = fmt.Errorf("disabled map zoom did not reset to standard view")
			return
		}
		if g.stage == 72 && c.floorHovered != 1 {
			g.err = fmt.Errorf("floor-item hover fixture failed: held=%v event=%v UI=%v hit=%d", c.held, c.event.active, c.uiHovered, c.floorItemAt(c.Input.X, c.Input.Y))
			return
		}
		if g.stage == 68 && (len(c.battle.playing) != 1 || c.battle.shown[clientbattle.Cell{X: 4, Y: 2}].HP != 75 || len(c.battle.departures) != 0) {
			g.err = fmt.Errorf("failed escape lost its native falling animation")
			return
		}
		if g.stage >= 53 && g.stage <= 59 {
			pet := c.battle.state.At(clientbattle.Cell{X: 3, Y: 2})
			if pet == nil || pet.ID != 14162 ||
				g.stage <= 54 && (len(c.battle.petDepartures) != 1 || len(c.battle.arrivals) != 1) ||
				g.stage <= 55 && len(c.battle.playing) != 0 ||
				g.stage >= 58 && c.battle.shown[pet.Cell].HP != 75 {
				g.err = fmt.Errorf("concurrent pet switch failed at stage %d", g.stage)
				return
			}
		}
		if g.stage >= 60 && g.stage <= 68 {
			pet := c.battle.state.At(clientbattle.Cell{X: 3, Y: 2})
			if pet == nil || pet.ID != 12032 || !c.battle.petPresented(*pet) || pet.Waiting != (g.stage <= 64) ||
				g.stage <= 61 && (len(c.battle.petDepartures) != 1 || len(c.battle.arrivals) != 1) ||
				g.stage >= 62 && (len(c.battle.petDepartures) != 0 || len(c.battle.arrivals) != 0) {
				g.err = fmt.Errorf("waiting pet visibility/readiness failed at stage %d", g.stage)
				return
			}
		}
		if g.stage >= 47 && g.stage <= 49 && (c.battleResults.form == nil || !c.battleResults.form.Visible) {
			g.err = fmt.Errorf("post-combat results missing")
			return
		}
		if g.stage >= 50 && g.stage <= 68 && (!c.battle.state.Active || c.Inventory.CanAct() ||
			g.stage == 50 && !c.Social.Visible || g.stage == 51 && !c.Inventory.Visible || g.stage == 52 && !c.Team.Visible) {
			g.err = fmt.Errorf("battle module missing at stage %d", g.stage)
			return
		}
		if g.stage == 49 && c.battleResults.form.list.TopIndex == 0 {
			g.err = fmt.Errorf("reward list did not scroll")
			return
		}
		if g.stage == 39 && (len(c.battle.overkills) != 1 || c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].Removed) {
			g.err = fmt.Errorf("overkill flight missing")
			return
		}
		if g.stage == 42 && (c.combat.round == nil || c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 100) {
			g.err = fmt.Errorf("round chain skipped contact timing")
			return
		}
		if g.stage == 43 && c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 75 {
			g.err = fmt.Errorf("round chain contact missing")
			return
		}
		if g.stage == 45 && (c.combat.round != nil || !c.battle.state.Ready || c.combat.choiceToken != 101) {
			g.err = fmt.Errorf("round chain did not release readiness")
			return
		}
		if g.stage == 40 && !c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].Removed {
			g.err = fmt.Errorf("overkill target did not vanish")
			return
		}
		if g.stage >= 27 && g.stage <= 32 {
			cell := clientbattle.Cell{X: 2, Y: 2}
			switch {
			case g.stage == 27 && (!c.battle.closing || c.battle.shown[cell].HP != 100):
				g.err = fmt.Errorf("battle damage appeared before contact")
			case g.stage == 28 && c.battle.shown[cell].HP != 0:
				g.err = fmt.Errorf("battle damage absent at contact")
			case g.stage == 28 && (c.battle.zoom.Dx() >= ScreenWidth || c.battle.zoomCanvas == nil || c.battle.camera == battleCameraOrigin()):
				g.err = fmt.Errorf("battle camera/zoom absent at contact")
			case g.stage == 30 && (!c.battle.state.Active || len(c.battle.departures) == 0):
				g.err = fmt.Errorf("departure was cut short")
			case g.stage == 31 && (c.battle.state.Active || c.sceneFade.mode != sceneFadeLeaveBattle):
				g.err = fmt.Errorf("departure did not start map fade")
			case g.stage == 32 && c.sceneFade.mode != sceneFadeNone:
				g.err = fmt.Errorf("map fade did not finish")
			}
			if g.err != nil {
				return
			}
		}
		if (g.stage == 34 || g.stage == 35) && c.sceneFade.mode != sceneFadeRevealBattle {
			g.err = fmt.Errorf("battle reveal missing at stage %d", g.stage)
			return
		}
		if g.stage == 36 && c.sceneFade.mode != sceneFadeNone {
			g.err = fmt.Errorf("battle reveal did not finish")
			return
		}
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
