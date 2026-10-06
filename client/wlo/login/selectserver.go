package login

import (
	"bytes"
	"image"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
)

// Server list layout from FUN_003fecf4 and the paint and light functions.
const (
	serverBackX, serverBackY = 0xf6, 0x82 // FUN_003ff65c
	signalX                  = 0x202      // FUN_003ff704
	signalRowY               = 0xe4       // y of row 1; row 0 is 20 px higher
	signalSize               = 0x12
	signalTipOffsetX         = 0x28
	listRowPixels            = 0x14
	skinTextColor            = 0x841
	selectionColor           = 0xf5bf89 // TColor
)

// Signal tooltip colours (TColor) and the light names (PTR_PTR_004c9858).
const (
	signalTipPen  = 0xff0000
	signalTipFill = 0xf98b3d
)

var signalNames = [maxSignal + 1]string{"Offline", "Good", "Active", "Crowded"}

// Region colour tags in SERVER.INI ("<B>", "<G>", "<R>").
const (
	tagBlue  = 0x1f
	tagGreen = 0x540
	tagRed   = 0xf800
)

// Tags of the server list's buttons (handler 0x3fe880).
const (
	tagLeave = 1
	tagNext  = 2
	tagUp    = 3
	tagDown  = 4
)

const connectSound = `sound\wav0151.wav`

// SelectServer is TSe_SelectServer (constructor 0x3fe778, components
// FUN_003fecf4). The form itself has no size; its children are placed in
// screen coordinates.
type SelectServer struct {
	seui.Form
	G      *Globals
	Net    *Net
	Assets Assets // the client directory (PTR_DAT_004c9d7c) and its resources
	INI    string // SERVER.INI override
	// FallbackINI is used when the file cannot be read; the original
	// shows an empty list.
	FallbackINI []byte

	Background *surface.Surface   // +0x154 Form_ServerList_1.jpg
	SignalImg  int                // +0x144 icon_ServerSignal
	Frame      *seui.Panel        // +0x13c
	Addresses  [][]byte           // +0x138 addresses of the shown region
	Regions    *seui.SelectText   // +0x168
	Servers    *seui.SelectText   // +0x130
	Scroll     *seui.ScrollButton // +0x134
	Up, Down   *seui.FixedButton  // +0x174, +0x178
	Leave      *seui.FixedButton  // +0x140
	Next       *seui.FixedButton  // +0x170
	TopLine    int                // +0x14c
	Signals    []byte             // +0x150 one per server row
	Current    int                // +0x17c region slot shown
	RegionIdx  byte               // +0x4a0
	ServerRow  byte               // +0x4a1

	names [regionSlots][][]byte // +0x180
	addrs [regionSlots][][]byte // +0x310

	// OnExit is the Leave button (TForm1 +0x5d0 set, then Close).
	OnExit func()
	// OnConnecting runs after the login socket is opened: the original
	// resets an object at PTR_DAT_004ca958 (not traced) and queues action 0.
	OnConnecting  func()
	LastSelection *ServerSelection
}

func NewSelectServer(env *seui.Env, g *Globals, n *Net, a Assets) *SelectServer {
	s := &SelectServer{G: g, Net: n, Assets: a}
	s.InitForm(s, env)
	s.Name = "SelectServer"
	s.build()
	return s
}

func (s *SelectServer) build() {
	env := s.Env
	if m, err := loadJPEG(s.Assets.SkinPicture("Menu", "Skins", "White", "Form_ServerList_1.jpg")); err == nil {
		s.Background = m
	}
	s.SignalImg = env.Pics.Find("icon_ServerSignal")
	s.Shown = false

	s.Frame = seui.NewPanel(env, s)
	s.Frame.Init("", 0x10, 0x32, 0x32, 0, 0, true, 0xd8, 0x178, 0x29)
	s.Frame.SetMargins(8, 8, 8, 8)
	s.Frame.Hover = false

	s.Regions = newServerList(env, s, 0x10d)
	s.Regions.OnSelect = func(i int) { s.SelectRegion(byte(i)) }
	s.Servers = newServerList(env, s, 0x19d)
	s.Servers.OnDblClick = func(i int) { s.Connect(i) }

	s.Scroll = seui.NewScrollButton(env, s)
	s.Scroll.SetThumb("bar_H4", 0, 0x15, 9, 0)
	s.Scroll.Init("", 0x188, 0xb6, 0x11, 0, 0, true, 0xb4, 0xb, 0xe5)
	s.Scroll.SetCaps(7, 0xd)
	s.Scroll.TabStop = false
	s.Servers.AttachScroll(s.Scroll)

	s.Up = s.button("Btn_ArrowUp_1", 0x188, 0x11, 0x11, 0xd2, tagUp)
	s.Down = s.button("Btn_ArrowDn_1", 0x188, 0x11, 0x11, 0x19b, tagDown)
	s.Leave = s.button("btn_Leave_1", 0x1ab, 0x14, 0x38, 0x1ba, tagLeave)
	s.Leave.SetColor(0xffff)
	s.Next = s.button("Btn_Next", 0x143, 0x14, 0x38, 0x1ba, tagNext)
}

func newServerList(env *seui.Env, owner seui.Control, left int) *seui.SelectText {
	l := seui.NewSelectText(env, owner)
	l.Highlight = selectionColor
	l.SetBounds(left, 0xd2, 0xdc, 0x78)
	l.SetColor(skinTextColor)
	l.Embossed, l.FocusBox = false, false
	l.ShowSel = true
	return l
}

func (s *SelectServer) button(name string, left, h, w, top, tag int) *seui.FixedButton {
	b := seui.NewFixedButton(s.Env, s)
	b.Init(name, left, h, w, 0, 0, true, h, w, top)
	b.Tag = tag
	b.OnClickTag = s.clicked
	return b
}

// clicked is the tag handler at 0x3fe880.
func (s *SelectServer) clicked(tag int) {
	switch tag {
	case tagLeave:
		if s.OnExit != nil {
			s.OnExit()
		}
	case tagNext:
		if s.Servers.Selected != -1 {
			s.Connect(s.Servers.Selected)
		}
	case tagUp:
		s.Servers.ScrollUp()
	case tagDown:
		s.Servers.ScrollDown()
	}
}

// Show is slot +0x20 (0x3fde2c): the form appears and SERVER.INI is read
// into the region list.
func (s *SelectServer) Show() {
	if s.G.Mode == 1 || s.G.Mode == 2 {
		return
	}
	s.Form.Show()
	s.G.InGame = false
	s.Addresses = nil
	s.Servers.Clear()
	s.Regions.Clear()
	raw, err := os.ReadFile(s.ServerINI())
	if err != nil {
		raw = s.FallbackINI
	}
	s.load(splitLines(raw))
	s.restoreTopLine()
}

// load is the SERVER.INI part of 0x3fde2c.
func (s *SelectServer) load(lines [][]byte) {
	var first [maxRegion]int
	for i := range first {
		first[i] = -1
	}
	for i, l := range lines {
		if len(l) == 0 || at(l, 3) != '[' {
			continue
		}
		v, ok := strToInt(copyStr(l, 1, 2))
		if !ok {
			continue
		}
		if n := byte(v); n != 0 && n < 100 {
			first[n-1] = i
		}
	}
	for r := 1; r <= maxRegion; r++ {
		li := first[r-1]
		if li == -1 {
			continue
		}
		if li >= len(lines) {
			break
		}
		l := lines[li]
		if len(l) == 0 || at(l, 3) != '[' {
			continue
		}
		name, color, flag := parseRegion(l)
		s.G.Names[r] = name
		s.G.FlagRegion[flag] = r
		if r == hiddenRegion {
			s.G.HiddenName = name
		} else {
			if color == 0 {
				s.Regions.Add(name, "", "0")
			} else {
				s.Regions.AddColored(name, color, "0")
			}
			k := s.Regions.Count()
			s.names[k], s.addrs[k] = nil, nil
		}
		for j := 1; j <= serversPerRegion; j++ {
			li++
			if li >= len(lines) {
				break
			}
			l := lines[li]
			if len(l) == 0 {
				continue
			}
			if at(l, 3) == '[' {
				break
			}
			srv, addr, ok := parseServer(l)
			if r == hiddenRegion {
				s.G.HiddenAddr = addr
			} else if ok {
				k := s.Regions.Count()
				s.names[k] = append(s.names[k], srv)
				s.addrs[k] = append(s.addrs[k], addr)
				idx := indexOf(s.names[k], srv)
				if id := idx + flag*statusIDsPerFlag + 1; id >= 0 && id < nameTableEntries {
					s.G.Names[id] = srv
				}
			}
		}
	}
}

func indexOf(list [][]byte, s []byte) int {
	for i, x := range list {
		if bytes.Equal(x, s) {
			return i
		}
	}
	return -1
}

// parseRegion reads "NN[Name]<C>flag".
func parseRegion(l []byte) (name []byte, color uint16, flag int) {
	p := pos("]", l)
	if p == 0 {
		p = len(l) - 1
	}
	if p < 2 {
		name = []byte("[]")
	} else {
		name = copyStr(l, 4, p-4)
	}
	if at(l, len(l)) == ']' {
		return name, 0, 0
	}
	a, b := pos("<", l), pos(">", l)
	if a > 0 && b > 0 && b-a == 2 {
		switch at(l, a+1) {
		case 'B':
			color = tagBlue
		case 'G':
			color = tagGreen
		case 'R':
			color = tagRed
		}
		p += 3
	}
	flag, _ = strToInt(copyStr(l, p+1, len(l)-p))
	return name, color, flag
}

// parseServer reads "Name*address"; ok is false without an address.
func parseServer(l []byte) (name, addr []byte, ok bool) {
	p := pos("*", l)
	if p < 2 {
		name = []byte("No Name")
	} else {
		name = copyStr(l, 1, p-1)
	}
	if p != 0 && p != len(l) {
		return name, copyStr(l, p+1, len(l)-p), true
	}
	return name, nil, false
}

func (s *SelectServer) saveFile() string { return s.Assets.UserPath("save.dat") }

// ServerINI is the server list file: INI when set, else SERVER.INI in the
// client directory.
func (s *SelectServer) ServerINI() string {
	if s.INI != "" {
		return s.INI
	}
	return Path(s.Assets.Root, "SERVER.INI")
}

// restoreTopLine is FUN_003ff16c.
func (s *SelectServer) restoreTopLine() {
	raw, err := os.ReadFile(s.saveFile())
	if err != nil {
		return
	}
	s.TopLine = 0
	if v := value(splitLines(raw), "TopLine"); len(v) > 0 {
		s.TopLine, _ = strToInt(v)
	}
	l := s.Servers
	if l.Count() > l.RowsShown && l.Count()-l.RowsShown < s.TopLine {
		s.TopLine = l.Count() - l.RowsShown
	}
	if l.Count() <= l.RowsShown {
		s.TopLine = 0
	}
	l.SetTop(s.TopLine)
}

// saveTopLine is FUN_003ff348.
func (s *SelectServer) saveTopLine() {
	raw, err := os.ReadFile(s.saveFile())
	var lines [][]byte
	if err == nil {
		lines = splitLines(raw)
	}
	lines = setValue(lines, "TopLine", []byte(strconv.Itoa(s.TopLine)))
	os.WriteFile(s.saveFile(), joinLines(lines), 0o644)
}

// value is TStrings.Values[name].
func value(lines [][]byte, name string) []byte {
	for _, l := range lines {
		if i := bytes.IndexByte(l, '='); i >= 0 && bytes.EqualFold(l[:i], []byte(name)) {
			return l[i+1:]
		}
	}
	return nil
}

// setValue is the assignment TStrings.Values[name] := v, adding the line
// when it is missing.
func setValue(lines [][]byte, name string, v []byte) [][]byte {
	line := append([]byte(name+"="), v...)
	for i, l := range lines {
		if j := bytes.IndexByte(l, '='); j >= 0 && bytes.EqualFold(l[:j], []byte(name)) {
			lines[i] = line
			return lines
		}
	}
	return append(lines, line)
}

// Hide is slot +0x24 (0x3fe848).
func (s *SelectServer) Hide() {
	s.Form.Hide()
	s.saveTopLine()
	s.Signals = nil
	s.Regions.Selected, s.Servers.Selected = -1, -1
}

// SelectRegion is FUN_003fe8e8: the region's servers are listed and their
// signals are requested from one of them at random.
func (s *SelectServer) SelectRegion(idx byte) {
	s.RegionIdx = idx
	r := int(idx) + 1
	if idx >= maxRegion {
		return
	}
	s.Servers.Clear()
	s.Addresses = nil
	for i, name := range s.names[r] {
		a, b := pos("<", name), pos(">", name)
		var color uint16
		if a > 0 && b > 0 && b-a == 2 {
			switch at(name, a+1) {
			case 'B':
				color = tagBlue
			case 'G':
				color = tagGreen
			case 'R':
				color = tagRed
			}
		}
		if color == 0 {
			s.Servers.Add(name, "", "")
		} else {
			s.Servers.AddColored(copyStr(name, 1, a-1), color, "")
		}
		s.Addresses = append(s.Addresses, s.addrs[r][i])
	}
	s.Signals = make([]byte, len(s.Addresses))
	offset := 0
	for k := 1; k < r; k++ {
		offset += len(s.names[k])
	}
	s.G.Offset = byte(offset)
	for c := offset; c < offset+len(s.names[r]); c++ {
		for k := 1; k < nameTableEntries; k++ {
			if bytes.Equal(s.names[r][c-offset], s.G.Names[k]) {
				s.G.ServerIndex[c] = uint16(k)
			}
		}
	}
	pick := 0 // Random(0) is 0
	if n := len(s.names[r]); n > 0 {
		pick = rand.IntN(n)
	}
	s.queryStatus(pick)
	s.applySignals(offset, len(s.names[r]))
	s.Current = r
}

// applySignals copies known signals into the rows (the loop ending
// FUN_003fe8e8 and ClientSocket3Read).
func (s *SelectServer) applySignals(offset, count int) {
	for c := offset; c <= offset+count; c++ {
		s.setSignal(c-offset+1, s.G.Status[s.G.ServerIndex[c]])
	}
}

// setSignal is FUN_003ff2f4 with a 1-based row.
func (s *SelectServer) setSignal(row int, v byte) {
	if row-1 < 0 || row-1 > len(s.Signals)-1 {
		return
	}
	if v > maxSignal {
		v = 0
	}
	s.Signals[row-1] = v
}

// StatusReceived is the end of ClientSocket3Read: the shown region's
// rows take the new signals.
func (s *SelectServer) StatusReceived(data []byte) {
	s.G.ReadStatus(data)
	offset := 0
	for k := 1; k < s.Current; k++ {
		offset += len(s.names[k])
	}
	if s.Current > 0 {
		s.applySignals(offset, len(s.names[s.Current]))
	}
}

// queryStatus is FUN_003ff82c.
func (s *SelectServer) queryStatus(i int) {
	if s.Net.StatusActive() {
		s.Net.CloseStatus()
	}
	if s.G.InGame || i > s.Servers.Count()-1 {
		return
	}
	s.playConnect()
	s.G.statusBuf = nil
	s.Net.QueryStatus(string(s.Addresses[i]))
	s.G.NetActive = true
}

func (s *SelectServer) playConnect() {
	if s.G.Mode == 0 && s.Env.Sound != nil {
		s.Env.Sound(connectSound)
	}
}

// Connect is FUN_003ff4f4: the login socket opens to the server.
func (s *SelectServer) Connect(i int) {
	if s.G.InGame || i < 0 || i >= s.Servers.Count() || i >= len(s.Addresses) {
		return
	}
	s.G.Offset += byte(i)
	s.ServerRow = s.G.Offset
	s.LastSelection = &ServerSelection{Host: string(s.Addresses[i]), Region: s.Current, Index: i}
	s.playConnect()
	s.TopLine = s.Servers.TopIndex
	s.Net.Connect(string(s.Addresses[i]))
	s.G.NetActive = true
	if s.OnConnecting != nil {
		s.OnConnecting()
	}
	s.Hide()
}

// Paint is slot +0x10 (FUN_003ff65c): the background through the canvas,
// then the form's own panel.
func (s *SelectServer) Paint() {
	if !s.Visible {
		return
	}
	if s.Background != nil {
		s.Env.Screen.Draw(serverBackX, serverBackY, s.Background, false)
	}
	s.Panel.Paint()
}

// Update is slot +0x18 (FUN_003ff6d0): the form, then the signal lights.
func (s *SelectServer) Update(in *seui.Input) {
	s.Form.Update(in)
	if s.Visible {
		s.paintSignals(image.Pt(in.X, in.Y))
	}
}

// paintSignals is FUN_003ff704.
func (s *SelectServer) paintSignals(pt image.Point) {
	top := s.Servers.TopIndex
	for i := 0; i < s.Servers.RowsShown; i++ {
		if i+top > len(s.Signals)-1 {
			continue
		}
		v := int(s.Signals[i+top])
		src := image.Rect(0, v*signalSize, signalSize, v*signalSize+signalSize)
		y := (i-1)*listRowPixels + signalRowY
		s.Env.Pics.DrawRect(s.Env.Screen, s.SignalImg, signalX, y, src, true)
		if pt.In(image.Rect(signalX, y, signalX+signalSize, y+signalSize)) {
			seui.Tooltip(s.Env, signalX+signalTipOffsetX, y, []byte(signalNames[v]), signalTipPen, signalTipFill)
		}
	}
}

// loadJPEG is TJPEGImage.LoadFromFile, converted as the canvas draws it.
func loadJPEG(path string) (*surface.Surface, error) {
	if strings.EqualFold(filepath.Ext(path), ".png") {
		if compiled, err := clientimage.Open(path); err == nil {
			pixels, err := compiled.Pixels(compiled.Bounds(), clientimage.Plain)
			if err != nil {
				return nil, err
			}
			b := compiled.Bounds()
			return &surface.Surface{W: b.Dx(), H: b.Dy(), Pix: pixels}, nil
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		m, err := clientassets.ReadPicturePNG(path, nil)
		if err != nil {
			return nil, err
		}
		return surface.FromImage(m), nil
	}
	raw, err := clientfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m, err := clientassets.DecodeJPEG(raw)
	if err != nil {
		return nil, err
	}
	return surface.FromImage(m), nil
}
