package app

import (
	"errors"
	"image"
	"log"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
)

// Application branding and Xaolan's small portrait from the NPC asset table.
const WindowTitle = "Wonderland Gonline"
const xaolanPortraitFamily = "007"
const xaolanPortraitID = 7148

// TShiftState bits passed with input events.
const (
	shiftShift  = 1 << 0
	shiftAlt    = 1 << 1
	shiftCtrl   = 1 << 2
	shiftLeft   = 1 << 3
	shiftRight  = 1 << 4
	shiftDouble = 1 << 6
)

// Windows input timing: the double-click interval and keyboard repeat.
const (
	doubleClickInterval = 500 * time.Millisecond
	keyRepeatDelayTicks = 30 // 500 ms at 60 ticks per second
	keyRepeatEveryTicks = 2
)

// virtualKeys maps keys to Windows virtual-key codes for KeyDown/KeyUp.
var virtualKeys = map[ebiten.Key]uint16{
	ebiten.KeyBackspace: 0x08, ebiten.KeyTab: 0x09, ebiten.KeyEnter: 0x0d, ebiten.KeyNumpadEnter: 0x0d,
	ebiten.KeyEscape: 0x1b, ebiten.KeySpace: 0x20, ebiten.KeyPageUp: 0x21, ebiten.KeyPageDown: 0x22,
	ebiten.KeyEnd: 0x23, ebiten.KeyHome: 0x24, ebiten.KeyArrowLeft: 0x25, ebiten.KeyArrowUp: 0x26,
	ebiten.KeyArrowRight: 0x27, ebiten.KeyArrowDown: 0x28, ebiten.KeyInsert: 0x2d, ebiten.KeyDelete: 0x2e,
}

func init() {
	for k := ebiten.KeyA; k <= ebiten.KeyZ; k++ {
		virtualKeys[k] = uint16('A' + (k - ebiten.KeyA))
	}
	for k := ebiten.KeyF1; k <= ebiten.KeyF8; k++ {
		virtualKeys[k] = uint16(0x70 + k - ebiten.KeyF1)
	}
	for k := ebiten.Key0; k <= ebiten.Key9; k++ {
		virtualKeys[k] = uint16('0' + (k - ebiten.Key0))
	}
}

// ErrExit ends the game loop when Leave is clicked.
var ErrExit = errors.New("exit")

// Game adapts the client to Ebitengine: window messages become the
// original's input handlers and each tick runs one frame.
type Game struct {
	C         *Client
	Workspace *Workspace

	presenter      *render.Presenter
	lastDown       time.Time
	lastPt         [2]int
	cursors        map[*cursor.Frame]*ebiten.CursorImage
	suppressedKeys map[ebiten.Key]bool
	shown          *cursor.Frame // the frame the system is showing
}

func NewGame(c *Client) *Game { return &Game{C: c, Workspace: NewWorkspace(c)} }

func (g *Game) shift() byte {
	var s byte
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		s |= shiftShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		s |= shiftAlt
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) {
		s |= shiftCtrl
	}
	return s
}

func (g *Game) updateInput(pointerBlocked bool) error {
	if g.Workspace.titleEditing != 0 {
		for k, vk := range virtualKeys {
			d := inpututil.KeyPressDuration(k)
			if d == 1 || d > keyRepeatDelayTicks && (d-keyRepeatDelayTicks)%keyRepeatEveryTicks == 0 {
				g.Workspace.titleKey(vk)
			}
		}
		if g.Workspace.titleEditing != 0 {
			for _, r := range ebiten.AppendInputChars(nil) {
				g.Workspace.titleChar(r)
			}
		}
		return nil
	}
	ui := g.C.UI
	windowX, windowY := ebiten.CursorPosition()
	x, y, inside := g.Workspace.GamePoint(windowX, windowY)
	pointerBlocked = pointerBlocked || !inside
	held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	mouse := g.shift()
	if held {
		mouse |= shiftLeft
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		mouse |= shiftRight
	}
	if !pointerBlocked {
		if x != g.C.Input.X || y != g.C.Input.Y {
			ui.MouseMove(mouse, x, y, held)
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			now := g.C.Now()
			// A second press within the interval is WM_LBUTTONDBLCLK: VCL
			// raises OnDblClick, then OnMouseDown with ssDouble.
			if now.Sub(g.lastDown) < doubleClickInterval && g.lastPt == [2]int{x, y} {
				ui.DblClick()
				ui.MouseDown(seui.ButtonLeft, mouse|shiftDouble, x, y)
				g.lastDown = time.Time{}
			} else {
				// A press on a speaker's icon in the chat log whispers to them
				// before the interface sees it (FUN_00496ca4).
				if !g.C.SpeakerPress() && !ui.MouseDown(seui.ButtonLeft, mouse, x, y) {
					g.C.GroundClick(x, y)
				}
				g.lastDown, g.lastPt = now, [2]int{x, y}
			}
		}
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
			ui.MouseDown(seui.ButtonRight, mouse, x, y)
		}
		g.C.GroundHold(held, x, y)
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			if g.C.dropHotbar(x, y) {
				g.C.Input.Pressed = nil
				g.C.Input.Captured = nil
			} else {
				ui.MouseUp(seui.ButtonLeft, mouse, x, y)
			}
		}
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonRight) {
			ui.MouseUp(seui.ButtonRight, mouse, x, y)
		}
		_, wheel := ebiten.Wheel()
		for ; wheel >= 1; wheel-- {
			g.C.scrollWheel(true)
		}
		for ; wheel <= -1; wheel++ {
			g.C.scrollWheel(false)
		}
	} else {
		g.C.GroundHold(false, x, y)
	}
	keyHeld := func(key ebiten.Key) bool { return ebiten.IsKeyPressed(key) && !g.suppressedKeys[key] }
	g.C.WalkKeys(keyHeld(ebiten.KeyArrowLeft), keyHeld(ebiten.KeyArrowUp), keyHeld(ebiten.KeyArrowRight), keyHeld(ebiten.KeyArrowDown))
	keys := g.shift()
	for k, vk := range virtualKeys {
		if g.suppressedKeys[k] {
			if !ebiten.IsKeyPressed(k) {
				delete(g.suppressedKeys, k)
			}
			continue
		}
		d := inpututil.KeyPressDuration(k)
		if d == 1 || d > keyRepeatDelayTicks && (d-keyRepeatDelayTicks)%keyRepeatEveryTicks == 0 {
			if !g.C.SportKey(int(vk), true) && !g.C.EmoteKey(vk, keys) && !g.C.HotbarKey(vk, keys) && !g.C.CompoundKey(vk, keys) && !g.C.SkillsKey(vk, keys) && !g.C.SettingsKey(vk) && !g.C.InventoryKey(vk) {
				ui.KeyDown(vk, keys)
			}
		}
		if inpututil.IsKeyJustReleased(k) {
			if !g.C.SportKey(int(vk), false) {
				ui.KeyUp(vk, keys)
			}
		}
	}
	if len(g.suppressedKeys) == 0 {
		for _, r := range ebiten.AppendInputChars(nil) {
			if r < 0x80 {
				ui.Char(byte(r))
				continue
			}
			// A double-byte character arrives as two WM_CHAR messages.
			for _, b := range clientassets.Big5Text(string(r)) {
				ui.Char(b)
			}
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	s := g.Workspace.Compose()
	if g.presenter == nil {
		// NewGame remains usable by embedding callers without Run.
		g.presenter, _ = render.New(render.CPU)
	}
	g.presenter.Draw(screen, s)
	g.drawCursor(screen)
}

// Update forwards input to the selected session and pumps every socket.
func (g *Game) Update() error {
	g.C = g.Workspace.Current()
	if g.C == nil {
		return ErrExit
	}
	previous := g.C
	x, y := ebiten.CursorPosition()
	_, wheel := ebiten.Wheel()
	consumed := g.Workspace.Pointer(x, y, inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft), ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft), wheel)
	g.C = g.Workspace.Current()
	if g.C == nil {
		return ErrExit
	}
	if g.C != previous {
		g.suppressedKeys = map[ebiten.Key]bool{}
		for key := range virtualKeys {
			if ebiten.IsKeyPressed(key) {
				g.suppressedKeys[key] = true
			}
		}
	}
	if consumed && (inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft)) {
		g.lastDown = time.Time{}
		g.C.cancelSessionInput()
	}
	if err := g.updateInput(consumed); err != nil {
		return err
	}
	g.Workspace.Tick()
	g.C = g.Workspace.Current()
	if g.C == nil {
		return ErrExit
	}
	return nil
}

// drawCursor shows the selected ANI frame as the system cursor, as the
// original's Screen.Cursors were Windows cursors: the system draws it at the
// display's rate with the pointer, outside the game's frames. Each frame
// becomes a native cursor once (the patched Ebitengine's SetCursorImage,
// third_party/ebiten/PATCHES.md); the animation swaps them on its jiffy
// timings. Without a frame the system pointer shows.
func (g *Game) drawCursor(*ebiten.Image) {
	var f *cursor.Frame
	x, y := ebiten.CursorPosition()
	_, _, inside := g.Workspace.GamePoint(x, y)
	if g.C.Cursors != nil && inside {
		f = g.C.Cursors.Frame(g.C.Now())
	}
	if f == g.shown {
		return
	}
	g.shown = f
	if f == nil {
		ebiten.SetCursorImage(nil)
		return
	}
	c := g.cursors[f]
	if c == nil {
		if g.cursors == nil {
			g.cursors = map[*cursor.Frame]*ebiten.CursorImage{}
		}
		c = ebiten.NewCursorImage(f.Image, f.Hotspot.X, f.Hotspot.Y)
		g.cursors[f] = c
	}
	ebiten.SetCursorImage(c)
}

func (g *Game) Layout(int, int) (int, int) { return workspaceWidth, ScreenHeight }

// Run opens the window and runs the client until Leave or the window
// closes.
func Run(c *Client) error {
	g := NewGame(c)
	presenter, err := render.New(c.options.Renderer)
	if err != nil {
		return err
	}
	g.presenter = presenter
	defer presenter.Close()
	log.Printf("renderer: %s", presenter.Mode())
	defer g.Workspace.Close()
	if err := g.Workspace.RestoreProfile(); err != nil {
		log.Printf("workspace profile: %v", err)
	}
	if presenter.Mode() == render.GPU {
		g.Workspace.attachRenderer(presenter.Attach)
	}
	g.Workspace.EnableAudio()
	if icon, err := c.lib.Image(xaolanPortraitFamily, xaolanPortraitID, 0, 0); err == nil {
		ebiten.SetWindowIcon([]image.Image{icon})
	} else {
		log.Printf("window icon: %v", err)
	}
	ebiten.SetWindowSize(workspaceWidth, ScreenHeight)
	ebiten.SetWindowTitle(WindowTitle)
	ebiten.SetTPS(60)
	ebiten.SetRunnableOnUnfocused(true)
	err = ebiten.RunGame(g)
	if errors.Is(err, ErrExit) {
		return nil
	}
	return err
}

// Match keyboard character encoding while letting the editor enforce its
// filters, caret position and length limit. Clipboard contents are never logged.
func loginClipboardText() []byte {
	value, err := ebiten.ClipboardText()
	if err != nil {
		return nil
	}
	return clientassets.Big5Text(value)
}
