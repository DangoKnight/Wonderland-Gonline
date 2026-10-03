package app

import (
	"errors"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"wonderland-go/client/wlo/cursor"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/clientassets"
)

// Window title from the form design.
const WindowTitle = "WLO Rhodes Island"

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
	for k := ebiten.Key0; k <= ebiten.Key9; k++ {
		virtualKeys[k] = uint16('0' + (k - ebiten.Key0))
	}
}

// ErrExit ends the game loop when Leave is clicked.
var ErrExit = errors.New("exit")

// Game adapts the client to Ebitengine: window messages become the
// original's input handlers and each tick runs one frame.
type Game struct {
	C *Client

	img      *ebiten.Image
	pix      []byte
	lastDown time.Time
	lastPt   [2]int
	cursors  map[*cursor.Frame]*ebiten.CursorImage
	shown    *cursor.Frame // the frame the system is showing
}

func NewGame(c *Client) *Game { return &Game{C: c} }

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

func (g *Game) Update() error {
	ui := g.C.UI
	x, y := ebiten.CursorPosition()
	held := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	mouse := g.shift()
	if held {
		mouse |= shiftLeft
	}
	if ebiten.IsMouseButtonPressed(ebiten.MouseButtonRight) {
		mouse |= shiftRight
	}
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
			if !ui.MouseDown(seui.ButtonLeft, mouse, x, y) {
				g.C.GroundClick(x, y)
			}
			g.lastDown, g.lastPt = now, [2]int{x, y}
		}
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		ui.MouseDown(seui.ButtonRight, mouse, x, y)
	}
	g.C.GroundHold(held, x, y)
	g.C.WalkKeys(ebiten.IsKeyPressed(ebiten.KeyArrowLeft), ebiten.IsKeyPressed(ebiten.KeyArrowUp),
		ebiten.IsKeyPressed(ebiten.KeyArrowRight), ebiten.IsKeyPressed(ebiten.KeyArrowDown))
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		ui.MouseUp(seui.ButtonLeft, mouse, x, y)
	}
	if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonRight) {
		ui.MouseUp(seui.ButtonRight, mouse, x, y)
	}
	keys := g.shift()
	for k, vk := range virtualKeys {
		d := inpututil.KeyPressDuration(k)
		if d == 1 || d > keyRepeatDelayTicks && (d-keyRepeatDelayTicks)%keyRepeatEveryTicks == 0 {
			ui.KeyDown(vk, keys)
		}
		if inpututil.IsKeyJustReleased(k) {
			ui.KeyUp(vk, keys)
		}
	}
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
	g.C.Frame()
	if g.C.Exit {
		return ErrExit
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	s := g.C.Screen
	if g.img == nil {
		g.img = ebiten.NewImage(s.W, s.H)
		g.pix = make([]byte, s.W*s.H*4)
	}
	for i, v := range s.Pix {
		c := surface.Expand(v)
		g.pix[i*4], g.pix[i*4+1], g.pix[i*4+2], g.pix[i*4+3] = c.R, c.G, c.B, 0xff
	}
	g.img.WritePixels(g.pix)
	screen.DrawImage(g.img, nil)
	g.drawCursor(screen)
}

// drawCursor shows the selected ANI frame as the system cursor, as the
// original's Screen.Cursors were Windows cursors: the system draws it at the
// display's rate with the pointer, outside the game's frames. Each frame
// becomes a native cursor once (the patched Ebitengine's SetCursorImage,
// third_party/ebiten/PATCHES.md); the animation swaps them on its jiffy
// timings. Without a frame the system pointer shows.
func (g *Game) drawCursor(*ebiten.Image) {
	var f *cursor.Frame
	if g.C.Cursors != nil {
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

func (g *Game) Layout(int, int) (int, int) { return ScreenWidth, ScreenHeight }

// Run opens the window and runs the client until Leave or the window
// closes.
func Run(c *Client) error {
	sounds := &Sounds{Root: c.Assets.Media} // exported PCM WAV login sounds
	c.Env.Sound = sounds.Play
	c.sfx = sounds
	c.Music = &Music{Root: c.Assets.Media, Context: sounds.Context}
	c.Music.Play(loginMusic) // CheckStartMusic
	ebiten.SetWindowSize(ScreenWidth, ScreenHeight)
	ebiten.SetWindowTitle(WindowTitle)
	ebiten.SetTPS(60)
	// Remove the temporary decoded sprite archives on exit.
	defer c.Sprites.Close()
	err := ebiten.RunGame(NewGame(c))
	if errors.Is(err, ErrExit) {
		return nil
	}
	return err
}
