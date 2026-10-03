package ui

import (
	"bytes"
	"image"
)

// Server signal frames in icon_ServerSignal.bmp (18x18 each).
const (
	SignalUnknown = iota // grey
	SignalNormal         // green
	SignalBusy           // yellow
	SignalFull           // red
)

// ServerSelect is the server list form built by aLogin FUN_003fecf4.
type ServerSelect struct {
	regions    []Region
	regionList selectList
	serverList selectList
	scroll     scrollBar
	buttons    []*button
	region     int

	// Signals maps Server.ID to a signal frame; other values draw grey.
	Signals map[int]int
	// OnRegion runs when a region is selected; the original then queries a
	// random server of the region for signals (FUN_003ff82c).
	OnRegion func(Region)
	// OnChoose runs when a server is chosen with Next (or a double click).
	OnChoose func(Server)
	// OnLeave runs for the Leave button.
	OnLeave func()
}

var (
	serverForm  = image.Pt(246, 130) // Form_ServerList_1.jpg, measured
	serverLight = image.Pt(0x202, 0xe4-listRowHeight)
	// TSe_SelectText selection colour (constructor field +0x134).
	listFill = tColor(0xf5bf89)
)

func NewServerSelect(regions []Region) *ServerSelect {
	s := &ServerSelect{regions: regions, region: -1, Signals: map[int]int{}}
	s.scroll = scrollBar{at: image.Pt(0x188, 0xe5), width: 0x11, length: 0xb4, sheet: "bar_H4.bmp", frameW: 9, frameH: 0x15, capLen: 7}
	s.regionList = selectList{bounds: image.Rect(0x10d, 0xd2, 0x10d+0x78, 0xd2+0xdc), selected: -1, textColor: 0x0841, fill: listFill, onSelect: s.selectRegion}
	s.serverList = selectList{bounds: image.Rect(0x19d, 0xd2, 0x19d+0x78, 0xd2+0xdc), selected: -1, textColor: 0x0841, fill: listFill, scroll: &s.scroll,
		onChoose: s.choose}
	items := make([]listItem, len(regions))
	for i, r := range regions {
		items[i] = listItem{[]byte(r.Name), r.Color}
	}
	s.regionList.set(items)
	s.serverList.set(nil)
	s.buttons = []*button{
		{sprite: "Btn_ArrowUp_1.bmp", at: image.Pt(0x188, 0xd2), onClick: func() { s.serverList.scrollBy(-1) }},
		{sprite: "Btn_ArrowDn_1.bmp", at: image.Pt(0x188, 0x19b), onClick: func() { s.serverList.scrollBy(1) }},
		{sprite: "btn_Leave_1.bmp", at: image.Pt(0x1ab, 0x1ba), onClick: func() {
			if s.OnLeave != nil {
				s.OnLeave()
			}
		}},
		{sprite: "Btn_Next.bmp", at: image.Pt(0x143, 0x1ba), onClick: func() {
			if s.serverList.selected >= 0 {
				s.choose(s.serverList.selected)
			}
		}},
	}
	return s
}

// selectRegion is FUN_003fe8e8: list the region's servers, taking a
// "<B>", "<G>" or "<R>" suffix as the name's colour.
func (s *ServerSelect) selectRegion(i int) {
	s.region = i
	var items []listItem
	for _, sv := range s.regions[i].Servers {
		name, c := []byte(sv.Name), uint16(0)
		lt, gt := bytes.IndexByte(name, '<')+1, bytes.IndexByte(name, '>')+1
		if lt > 0 && gt > 0 && gt-lt == 2 {
			switch name[lt] {
			case 'B':
				c = 0x001f
			case 'G':
				c = 0x0540
			case 'R':
				c = 0xf800
			}
			if c != 0 {
				name = name[:lt-1]
			}
		}
		items = append(items, listItem{name, c})
	}
	s.serverList.set(items)
	if s.OnRegion != nil {
		s.OnRegion(s.regions[i])
	}
}

// choose is FUN_003ff4f4.
func (s *ServerSelect) choose(i int) {
	if s.region < 0 || i >= len(s.regions[s.region].Servers) || s.OnChoose == nil {
		return
	}
	s.OnChoose(s.regions[s.region].Servers[i])
}

// SelectRegion and SelectServer drive the form as clicks would.
func (s *ServerSelect) SelectRegion(i int) {
	s.regionList.selected = i
	s.selectRegion(i)
}
func (s *ServerSelect) SelectServer(i int) { s.serverList.selected = i }

func (s *ServerSelect) Update(a *Assets, in *Input) {
	s.regionList.update(in)
	s.serverList.update(in)
	for _, b := range s.buttons {
		b.update(a, in)
	}
}

func (s *ServerSelect) Draw(a *Assets, dst *image.RGBA) error {
	if err := drawBackdrop(a, dst); err != nil {
		return err
	}
	form, err := a.Skin("Form_ServerList_1.jpg", 1)
	if err != nil {
		return err
	}
	form.Draw(dst, serverForm.X, serverForm.Y, 0)
	s.regionList.draw(a, dst)
	s.serverList.draw(a, dst)
	if err := s.scroll.draw(a, dst); err != nil {
		return err
	}
	for _, b := range s.buttons {
		if err := b.draw(a, dst); err != nil {
			return err
		}
	}
	// FUN_003ff704: one signal per visible server row.
	if s.region >= 0 {
		light, err := a.Skin("icon_ServerSignal.bmp", 4)
		if err != nil {
			return err
		}
		servers := s.regions[s.region].Servers
		for row := range s.serverList.visible() {
			i := s.serverList.top + row
			if i >= len(servers) {
				break
			}
			state := s.Signals[servers[i].ID]
			if state < 0 || state > 3 {
				state = 0
			}
			light.Draw(dst, serverLight.X, serverLight.Y+row*listRowHeight, state)
		}
	}
	return nil
}

// drawBackdrop draws the shared login background and logo.
func drawBackdrop(a *Assets, dst *image.RGBA) error {
	bg, err := a.Pic("LogPic1.jpg")
	if err != nil {
		return err
	}
	bg.Draw(dst, 0, 0, 0)
	logo, err := a.Skin("Icon_LoginLogo_1.bmp", 1)
	if err != nil {
		return err
	}
	logo.Draw(dst, 617, 0, 0)
	return nil
}
