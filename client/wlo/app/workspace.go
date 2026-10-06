package app

import (
	"errors"
	"fmt"
	"image"
	"log"
	"path/filepath"
	"wonderland-gonline/client/wlo/surface"
)

const (
	sessionPanelWidth           = 220
	workspaceWidth              = ScreenWidth + sessionPanelWidth
	sessionCloseWidth           = 24
	sessionPanelPadding         = 10
	sessionListTop              = 40
	sessionToggleSide           = 24
	sessionSlideTicks           = 12 // 200 ms at 60 Hz.
	sessionCardGap              = 8
	sessionCardHeight           = 100
	sessionRowHeight            = sessionCardHeight + sessionCardGap
	sessionVisibleRows          = 5 // List slots include the add card.
	sessionPreviewWidth         = 104
	sessionPreviewHeight        = 78
	sessionPreviewTop           = 18
	sessionConfirmTop           = 60
	sessionConfirmHeight        = 28
	sessionPlusHalfSpan         = 16
	sessionPlusHalfStroke       = 2
	sessionBackgroundFrameTicks = 15 // 4 preview frames/second; logic stays at 60 Hz.
	sessionPanelInk             = 0xffff
	sessionPanelFill            = 0x1083
	sessionRowFill              = 0x18e5
	sessionActiveFill           = 0x2148
	sessionHoverFill            = 0x2127
	sessionBorderInk            = 0x31c8
	sessionAccentInk            = 0xdd69
	sessionShadowFill           = 0x0841
)

type Session struct {
	ID     int
	Client *Client
}

// Workspace owns a window, shared caches and independently connected clients.
// Tick and input methods run serially on the platform game thread.
type Workspace struct {
	Sessions       []*Session
	Active         int
	Collapsed      bool
	slide          int
	Scroll         int
	PendingRemove  int
	options        Options
	resources      *Resources
	nextID         int
	ticks          uint64
	audio          bool
	hover          image.Point
	captured       bool
	screen         *surface.Surface
	factory        func(Options) (*Client, error)
	attachTarget   func(*surface.Surface)
	profileEnabled bool
	profileLast    []byte
	profileError   string
}

func NewWorkspace(first *Client) *Workspace {
	o := first.options
	o.Shared = first.resources
	if o.UserRoot == "" {
		o.UserRoot = filepath.Dir(filepath.Dir(first.Assets.UserPath("settings.json")))
	}
	return &Workspace{Sessions: []*Session{{ID: 1, Client: first}}, options: o, resources: first.resources, nextID: 2, factory: New, hover: image.Pt(-1, -1), screen: surface.New(workspaceWidth, ScreenHeight)}
}
func (w *Workspace) Current() *Client {
	if len(w.Sessions) == 0 {
		return nil
	}
	return w.Sessions[w.Active].Client
}
func (w *Workspace) sessionOptions(id int) Options {
	o := w.options
	base := o.UserRoot
	if base == "" {
		base = filepath.Dir(filepath.Dir(w.Sessions[0].Client.Assets.UserPath("settings.json")))
	}
	if o.SettingsPath != "" {
		base = filepath.Dir(o.SettingsPath)
	}
	o.UserRoot = filepath.Join(base, fmt.Sprintf("session-%d", id))
	if id == 1 {
		return w.options
	}
	o.SettingsPath = ""
	return o
}
func (w *Workspace) Add() error {
	if len(w.Sessions) >= workspaceProfileMaximumSessions {
		return fmt.Errorf("workspace supports at most %d sessions", workspaceProfileMaximumSessions)
	}
	c, err := w.factory(w.sessionOptions(w.nextID))
	if err != nil {
		return err
	}
	if w.attachTarget != nil {
		w.attachTarget(c.Screen)
	}
	c.backgroundSession = true
	w.Sessions = append(w.Sessions, &Session{ID: w.nextID, Client: c})
	w.nextID++
	if w.audio {
		w.initAudio(c)
	}
	w.Switch(len(w.Sessions) - 1)
	w.Scroll = max(0, len(w.Sessions)-w.visibleRows())
	return nil
}
func (w *Workspace) Switch(index int) {
	if index < 0 || index >= len(w.Sessions) || index == w.Active {
		return
	}
	previous := w.Current()
	previous.cancelSessionInput()
	previous.backgroundSession = true
	previous.applyLocalSettings()
	w.Active = index
	current := w.Current()
	current.backgroundSession = false
	current.cancelSessionInput()
	current.applyLocalSettings()
	current.Frame()
}
func (c *Client) cancelSessionInput() {
	c.Input.X, c.Input.Y = -100, -100
	c.Input.Pressed, c.Input.Captured, c.Input.Hovered = nil, nil, nil
	c.hotbar.drag = nil
	if c.Inventory != nil {
		c.Inventory.CancelDrag()
	}
	c.groundHeld = false
	c.uiHovered = false
	c.WalkKeys(false, false, false, false)
}
func (c *Client) closeSession() {
	c.stopRemote()
	c.stopAmbience()
	if c.Music != nil {
		c.Music.Stop()
	}
	c.Net.Shutdown()
	c.Screen.Close()
	if c.fade.frozen != nil {
		c.fade.frozen.Close()
	}
	c.hotbar.drag = nil
	c.closeHotbarTarget()
	c.closeSettingsPrompt()
}
func (w *Workspace) Remove(id int) {
	index := -1
	for i, s := range w.Sessions {
		if s.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return
	}
	w.Sessions[index].Client.closeSession()
	copy(w.Sessions[index:], w.Sessions[index+1:])
	w.Sessions[len(w.Sessions)-1] = nil
	w.Sessions = w.Sessions[:len(w.Sessions)-1]
	if index < w.Active {
		w.Active--
	} else if w.Active >= len(w.Sessions) {
		w.Active = max(0, len(w.Sessions)-1)
	}
	w.PendingRemove = 0
	w.Scroll = min(w.Scroll, w.maxScroll())
	if current := w.Current(); current != nil {
		current.backgroundSession = false
		current.cancelSessionInput()
		current.applyLocalSettings()
	}
}
func (w *Workspace) Close() error {
	profileErr := w.SaveProfile()
	for _, s := range w.Sessions {
		s.Client.closeSession()
	}
	w.Sessions = nil
	w.screen.Close()
	return errors.Join(profileErr, w.resources.Close())
}
func (w *Workspace) Tick() {
	defer w.checkpointProfile()
	w.stepLayout()
	w.ticks++
	for _, s := range w.Sessions {
		s.Client.frame(s.Client == w.Current() || w.ticks%sessionBackgroundFrameTicks == 0)
	}
	for i := len(w.Sessions) - 1; i >= 0; i-- {
		if w.Sessions[i].Client.Exit {
			w.Remove(w.Sessions[i].ID)
		}
	}
}
func (w *Workspace) checkpointProfile() {
	if err := w.SaveProfile(); err != nil {
		if err.Error() != w.profileError {
			log.Printf("workspace profile: %v", err)
			w.profileError = err.Error()
		}
	} else {
		w.profileError = ""
	}
}
func (w *Workspace) visibleRows() int {
	return sessionVisibleRows
}

// A panel press stays captured until release; switching must not leak clicks.
func (w *Workspace) Pointer(x, y int, press, held bool, wheel float64) bool {
	w.hover = image.Pt(x, y)
	panel := w.SidebarContains(x, y)
	_, _, inside := w.GamePoint(x, y)
	consumed := panel || w.captured || !inside
	if press && panel {
		w.captured = true
		if c := w.Current(); c != nil {
			c.cancelSessionInput()
		}
		if image.Pt(x, y).In(w.toggleRect()) {
			w.Collapsed = !w.Collapsed
			w.PendingRemove = 0
		} else if !w.animating() && !w.Collapsed {
			w.panelClick(x, y)
		}
	}
	if panel && !w.animating() && !w.Collapsed && wheel != 0 {
		w.PendingRemove = 0
		if wheel > 0 {
			w.Scroll--
		} else {
			w.Scroll++
		}
		w.Scroll = max(0, min(w.Scroll, w.maxScroll()))
	}
	if !held {
		w.captured = false
	}
	return consumed
}
func (w *Workspace) cardRect(row int) image.Rectangle {
	top := sessionListTop + row*sessionRowHeight
	left := w.panelLeft()
	return image.Rect(left+sessionPanelPadding, top, left+sessionPanelWidth-sessionPanelPadding, top+sessionCardHeight)
}
func (w *Workspace) addRect() image.Rectangle {
	return w.cardRect(len(w.Sessions) - w.Scroll)
}
func (w *Workspace) closeRect(row int) image.Rectangle {
	r := w.cardRect(row)
	return image.Rect(r.Max.X-sessionCloseWidth, r.Min.Y, r.Max.X, r.Min.Y+sessionPreviewTop)
}
func (w *Workspace) confirmRect(row int, confirm bool) image.Rectangle {
	r := w.cardRect(row)
	middle := (r.Min.X + r.Max.X) / 2
	if confirm {
		return image.Rect(r.Min.X+8, r.Min.Y+sessionConfirmTop, middle-4, r.Min.Y+sessionConfirmTop+sessionConfirmHeight)
	}
	return image.Rect(middle+4, r.Min.Y+sessionConfirmTop, r.Max.X-8, r.Min.Y+sessionConfirmTop+sessionConfirmHeight)
}
func (w *Workspace) panelClick(x, y int) {
	point := image.Pt(x, y)
	if w.PendingRemove != 0 {
		for row := 0; row < w.visibleRows() && row+w.Scroll < len(w.Sessions); row++ {
			if w.Sessions[row+w.Scroll].ID != w.PendingRemove {
				continue
			}
			if point.In(w.confirmRect(row, true)) {
				w.Remove(w.PendingRemove)
			} else if point.In(w.confirmRect(row, false)) {
				w.PendingRemove = 0
			}
		}
		return
	}
	if w.addVisible() && point.In(w.addRect()) {
		if err := w.Add(); err != nil {
			w.Current().Chat.Notice("Could not open session: " + err.Error())
		}
		return
	}
	for row := 0; row < w.visibleRows() && row+w.Scroll < len(w.Sessions); row++ {
		if !point.In(w.cardRect(row)) {
			continue
		}
		if point.In(w.closeRect(row)) {
			w.PendingRemove = w.Sessions[row+w.Scroll].ID
		} else {
			w.Switch(row + w.Scroll)
		}
		return
	}
}
func (w *Workspace) Compose() *surface.Surface {
	c := w.Current()
	if c == nil {
		return w.screen
	}
	// Move the full game viewport without scaling it or modifying session pixels.
	w.screen.Fill(image.Rect(0, 0, workspaceWidth, ScreenHeight), sessionPanelFill)
	gameLeft := w.gameLeft()
	w.screen.Draw(gameLeft, 0, c.Screen, false)
	if w.slide < sessionSlideTicks {
		panelLeft := w.panelLeft()
		w.screen.Fill(image.Rect(panelLeft, 0, panelLeft+1, ScreenHeight), sessionShadowFill)
		for row := 0; row < w.visibleRows() && row+w.Scroll < len(w.Sessions); row++ {
			index := row + w.Scroll
			s := w.Sessions[index]
			r := w.cardRect(row)
			fill, border := uint16(sessionRowFill), uint16(sessionBorderInk)
			if w.hover.In(r) {
				fill = sessionHoverFill
			}
			if index == w.Active {
				fill, border = sessionActiveFill, sessionAccentInk
			}
			w.drawCard(r, fill, border)
			if index == w.Active {
				w.screen.Fill(image.Rect(r.Min.X, r.Min.Y+1, r.Min.X+3, r.Max.Y-1), sessionAccentInk)
			}
			name := fmt.Sprintf("%d: Login", s.ID)
			if s.Client.World != nil {
				name = fmt.Sprintf("%d: ", s.ID) + string(s.Client.World.Player.Name)
			}
			// Keep long character names clear of the close button.
			w.Current().Env.Text.Draw(r.Min.X+9, r.Min.Y+3, 0, false, true, w.screen, []byte(name), 15, r.Dx()-sessionCloseWidth-18, 0, sessionPanelInk, 0)
			w.label(w.closeRect(row).Min.X+8, r.Min.Y+3, "X")
			if w.PendingRemove == s.ID {
				w.label(r.Min.X+22, r.Min.Y+34, "Close instance?")
				for _, confirm := range []bool{true, false} {
					button := w.confirmRect(row, confirm)
					ink := uint16(sessionBorderInk)
					caption := "Cancel"
					if confirm {
						ink = sessionAccentInk
						caption = "Close"
					}
					w.screen.Fill(button, sessionRowFill)
					w.screen.Frame(button, ink)
					w.label(button.Min.X+12, button.Min.Y+7, caption)
				}
			} else {
				x := (r.Min.X + r.Max.X - sessionPreviewWidth) / 2
				preview := image.Rect(x, r.Min.Y+sessionPreviewTop, x+sessionPreviewWidth, r.Min.Y+sessionPreviewTop+sessionPreviewHeight)
				w.screen.Frame(preview.Inset(-1), sessionShadowFill)
				w.screen.DrawStretch(preview, s.Client.Screen, false)
			}
		}
		if w.addVisible() {
			add := w.addRect()
			fill, border := uint16(sessionRowFill), uint16(sessionBorderInk)
			if w.hover.In(add) {
				fill, border = sessionHoverFill, sessionAccentInk
			}
			w.drawCard(add, fill, border)
			cx, cy := (add.Min.X+add.Max.X)/2, (add.Min.Y+add.Max.Y)/2
			w.screen.Fill(image.Rect(cx-sessionPlusHalfSpan, cy-sessionPlusHalfStroke, cx+sessionPlusHalfSpan, cy+sessionPlusHalfStroke), sessionAccentInk)
			w.screen.Fill(image.Rect(cx-sessionPlusHalfStroke, cy-sessionPlusHalfSpan, cx+sessionPlusHalfStroke, cy+sessionPlusHalfSpan), sessionAccentInk)
		}
	}
	toggle := w.toggleRect()
	w.screen.Fill(toggle, sessionRowFill)
	w.screen.Frame(toggle, sessionBorderInk)
	caption := ">"
	if w.Collapsed {
		caption = "<"
	}
	w.label(toggle.Min.X+8, toggle.Min.Y+4, caption)
	return w.screen
}
func (w *Workspace) drawCard(r image.Rectangle, fill, border uint16) {
	w.screen.Fill(r.Add(image.Pt(2, 2)), sessionShadowFill)
	w.screen.Fill(r, fill)
	w.screen.Frame(r, border)
}
func (w *Workspace) label(x, y int, label string) {
	w.Current().Env.Text.Draw(x, y, 0, false, true, w.screen, []byte(label), 15, sessionPanelWidth-20, 0, sessionPanelInk, 0)
}
func (w *Workspace) initAudio(c *Client) {
	sounds := &Sounds{Root: c.Assets.Media, Shared: w.resources.audio}
	c.sfx = sounds
	c.Env.Sound = func(path string) {
		if !c.backgroundSession {
			sounds.Play(path)
		}
	}
	c.Music = &Music{Root: c.Assets.Media, Context: sounds.Context}
	c.applyLocalSettings()
	c.Music.Play(loginMusic)
}
func (w *Workspace) EnableAudio() {
	w.audio = true
	for _, s := range w.Sessions {
		w.initAudio(s.Client)
	}
}
func (w *Workspace) SidebarContains(x, y int) bool {
	return image.Pt(x, y).In(w.toggleRect()) || w.slide < sessionSlideTicks && image.Pt(x, y).In(image.Rect(w.panelLeft(), 0, workspaceWidth, ScreenHeight))
}

func (w *Workspace) maxScroll() int { return max(0, len(w.Sessions)+1-w.visibleRows()) }
func (w *Workspace) addVisible() bool {
	row := len(w.Sessions) - w.Scroll
	return row >= 0 && row < w.visibleRows()
}
func (w *Workspace) toggleRect() image.Rectangle {
	right := workspaceWidth - sessionPanelPadding
	return image.Rect(right-sessionToggleSide, sessionPanelPadding, right, sessionPanelPadding+sessionToggleSide)
}
func (w *Workspace) animating() bool {
	return w.Collapsed && w.slide < sessionSlideTicks || !w.Collapsed && w.slide > 0
}
func (w *Workspace) stepLayout() {
	if w.Collapsed {
		w.slide = min(sessionSlideTicks, w.slide+1)
	} else {
		w.slide = max(0, w.slide-1)
	}
}

// Smoothstep gives both sliding elements the same easing and keeps them apart.
func (w *Workspace) slideOffset(distance int) int {
	p, n := w.slide, sessionSlideTicks
	return distance * p * p * (3*n - 2*p) / (n * n * n)
}
func (w *Workspace) gameLeft() int  { return w.slideOffset(sessionPanelWidth / 2) }
func (w *Workspace) panelLeft() int { return ScreenWidth + w.slideOffset(sessionPanelWidth) }

// GamePoint translates window input to the active instance's unchanged viewport.
// Pointer actions pause during animation; networking and keyboard input continue.
func (w *Workspace) GamePoint(x, y int) (int, int, bool) {
	left := w.gameLeft()
	return x - left, y, !w.animating() && image.Pt(x, y).In(image.Rect(left, 0, left+ScreenWidth, ScreenHeight))
}

func (w *Workspace) attachRenderer(attach func(*surface.Surface)) {
	w.attachTarget = attach
	attach(w.screen)
	for _, s := range w.Sessions {
		attach(s.Client.Screen)
	}
}
