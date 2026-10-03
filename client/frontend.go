package main

import (
	"errors"
	"image"
	"math/rand/v2"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"wonderland-go/client/session"
	"wonderland-go/client/ui"
)

// defaultServerINI is used when no SERVER.INI is found: one local server.
const defaultServerINI = "01[Local]1\r\nLocal1*127.0.0.1\r\n"

const (
	screenServers = iota
	screenLogin
	screenLoggedIn
)

// frontend runs the recreated server selection and login screens.
type frontend struct {
	assets  *ui.Assets
	screen  int
	servers *ui.ServerSelect
	login   *ui.Login
	conn    *session.Conn
	server  ui.Server
	reject  bool // a bare 63/2 arrived; the next packet says why
	results chan func()

	frame    *image.RGBA
	texture  *ebiten.Image
	lastDown time.Time
	lastPos  image.Point
	snapshot string
	quit     bool
	fatal    error
}

func newFrontend(assets *ui.Assets, iniPath, snapshot string) *frontend {
	ini := []byte(defaultServerINI)
	if b, err := os.ReadFile(iniPath); err == nil {
		ini = b
	}
	regions, _ := ui.ParseServerINI(ini)
	f := &frontend{assets: assets, results: make(chan func(), 16), frame: ui.NewFrame(), snapshot: snapshot}
	f.servers = ui.NewServerSelect(regions)
	f.servers.OnRegion = f.queryStatus
	f.servers.OnChoose = f.connect
	f.servers.OnLeave = func() { f.quit = true }
	f.login = ui.NewLogin()
	f.login.OnPrevious = f.disconnect
	f.login.OnLogin = f.submit
	return f
}

// queryStatus asks one random server of the region for signals.
func (f *frontend) queryStatus(r ui.Region) {
	if len(r.Servers) == 0 {
		return
	}
	host := r.Servers[rand.IntN(len(r.Servers))].Address
	go func() {
		signals, err := session.Status(host, 3*time.Second)
		if err != nil {
			return // unreachable servers stay grey
		}
		f.results <- func() {
			for id, s := range signals {
				f.servers.Signals[id] = s
			}
		}
	}()
}

func (f *frontend) connect(s ui.Server) {
	f.server = s
	go func() {
		c, err := session.Dial(s.Address, 5*time.Second)
		f.results <- func() {
			if err != nil {
				// The original reports this with a dialog that is not
				// traced yet; the list stays open.
				return
			}
			f.conn = c
			f.login.Reset()
			f.screen = screenLogin
		}
	}()
}

// disconnect is Previous: close the connection and return to the list.
func (f *frontend) disconnect() {
	if f.conn != nil {
		f.conn.Close()
		f.conn = nil
	}
	f.screen = screenServers
}

func (f *frontend) submit(account, password string) {
	if f.conn == nil {
		return
	}
	f.reject = false
	if err := f.conn.Login([]byte(account), []byte(password)); err != nil {
		f.login.Notify(err.Error())
	}
}

func (f *frontend) handle(p []byte) {
	if f.screen != screenLogin {
		return
	}
	result, reject := session.Classify(p, f.reject)
	f.reject = reject
	switch result {
	case session.LoginOK:
		// Character selection is the next milestone.
		f.screen = screenLoggedIn
		f.login.Notify("Login accepted. Character selection is not recreated yet.")
	case session.LoginRejected:
		f.login.Notify("Incorrect account or password")
	case session.LoginDuplicate:
		f.login.Notify("This account is already online")
	}
}

func (f *frontend) input() *ui.Input {
	x, y := ebiten.CursorPosition()
	in := &ui.Input{
		X: x, Y: y,
		Down:     ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft),
		Pressed:  inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft),
		Released: inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft),
	}
	if in.Pressed {
		// Windows' default double-click time and 4-pixel rectangle.
		p := image.Pt(x, y)
		d := p.Sub(f.lastPos)
		in.DoubleClick = time.Since(f.lastDown) < 500*time.Millisecond && d.X*d.X <= 16 && d.Y*d.Y <= 16
		f.lastDown, f.lastPos = time.Now(), p
	}
	for _, r := range ebiten.AppendInputChars(nil) {
		if r < 0x80 {
			in.Text = append(in.Text, byte(r))
		}
	}
	keys := map[ebiten.Key]ui.Key{
		ebiten.KeyEnter: ui.KeyEnter, ebiten.KeyNumpadEnter: ui.KeyEnter,
		ebiten.KeyBackspace: ui.KeyBackspace, ebiten.KeyTab: ui.KeyTab,
	}
	for k, v := range keys {
		if inpututil.IsKeyJustPressed(k) || inpututil.KeyPressDuration(k) > 30 && inpututil.KeyPressDuration(k)%3 == 0 {
			in.Keys = append(in.Keys, v)
		}
	}
	return in
}

func (f *frontend) Update() error {
	if f.quit {
		return ebiten.Termination
	}
	for {
		select {
		case fn := <-f.results:
			fn()
			continue
		default:
		}
		break
	}
	if f.conn != nil {
		for done := false; !done; {
			select {
			case p, ok := <-f.conn.Packets():
				if !ok {
					f.conn = nil
					if f.screen != screenServers {
						f.screen = screenServers
					}
					done = true
					break
				}
				f.handle(p)
			default:
				done = true
			}
		}
	}
	in := f.input()
	switch f.screen {
	case screenServers:
		f.servers.Update(f.assets, in)
	case screenLogin:
		f.login.Update(f.assets, in)
	}
	return nil
}

func (f *frontend) Draw(screen *ebiten.Image) {
	var err error
	switch f.screen {
	case screenServers:
		err = f.servers.Draw(f.assets, f.frame)
	case screenLogin, screenLoggedIn:
		err = f.login.Draw(f.assets, f.frame)
	}
	if err != nil {
		f.fatal, f.quit = err, true
		return
	}
	if f.texture == nil {
		f.texture = ebiten.NewImage(ui.ScreenWidth, ui.ScreenHeight)
	}
	f.texture.WritePixels(f.frame.Pix)
	screen.DrawImage(f.texture, nil)
	if f.snapshot != "" {
		f.fatal = savePNG(f.snapshot, f.frame)
		f.snapshot = ""
		f.quit = true
	}
}

func (*frontend) Layout(int, int) (int, int) { return ui.ScreenWidth, ui.ScreenHeight }

func runFrontend(assetDir, iniPath, snapshot string) error {
	assets, err := ui.NewAssets(assetDir)
	if err != nil {
		return err
	}
	f := newFrontend(assets, iniPath, snapshot)
	ebiten.SetWindowSize(ui.ScreenWidth, ui.ScreenHeight)
	ebiten.SetWindowTitle("WLO Rhodes Island")
	ebiten.SetScreenClearedEveryFrame(false)
	if err := ebiten.RunGame(f); err != nil && !errors.Is(err, ebiten.Termination) {
		return err
	}
	return f.fatal
}
