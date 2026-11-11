package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/client/wlo/surface"
)

// Display settings belong to the application window, shared by every session.
// Ebitengine fullscreen uses desktop resolution, not exclusive display modes.
const (
	displayResolutionPickingEnabled = false // Temporarily keep the native game + sidebar canvas.
	displayWindowed                 = "windowed"
	displayFullscreen               = "fullscreen"
	displayBorderless               = "borderless"
	displayFPSVSync                 = -1
	displayFPSUnlimited             = 0
	displayConfirmDuration          = 15 * time.Second
	displayPanelLeft                = 260
	displayPanelTop                 = 140
	displayPanelWidth               = 500
	displayPanelHeight              = 400
	displayRowTop                   = 200
	displayRowHeight                = 44
	displayButtonTop                = 470
	displayButtonHeight             = 28
	displayControlLeft              = 470
	displayControlWidth             = 260
	displayMaximumBytes             = 4096
	displayMinimumWindowWidth       = 640
	displayMinimumWindowHeight      = 360
	displaySelectItemHeight         = 24
	// Select identifiers are one-based; zero means no open dropdown.
	displayModeRow       = 1
	displayResolutionRow = 2
	displayFrameRateRow  = 3
	displayScalingRow    = 4
	displayLanguageRow   = 5
	displayRowCount      = 5
)

var displayModes = []string{displayWindowed, displayFullscreen, displayBorderless}
var displaySizes = [][2]int{{1020, 600}, {1024, 768}, {1280, 720}, {1280, 800}, {1366, 768}, {1600, 900}, {1920, 1080}, {2560, 1440}, {3840, 2160}}
var displayFrameRates = []int{displayFPSVSync, 30, 60, 90, 120, 144, 165, 240, displayFPSUnlimited}

type DisplaySettings struct {
	Mode     string `json:"mode"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FPS      int    `json:"fps"`
	Scaling  string `json:"scaling,omitempty"`
	Language string `json:"language,omitempty"`
}

// Matches the existing workspace-level preference directory, independently of
// per-session game settings and the mounted asset location.
func applicationSettingsPath(o Options, a login.Assets) string {
	base := o.UserRoot
	if base == "" {
		base = filepath.Dir(filepath.Dir(a.UserPath("settings.json")))
	}
	if o.SettingsPath != "" {
		base = filepath.Dir(o.SettingsPath)
	}
	return filepath.Join(base, "display.json")
}

func defaultDisplaySettings() DisplaySettings {
	return DisplaySettings{Mode: displayWindowed, Width: workspaceWidth, Height: ScreenHeight, FPS: displayFPSVSync}
}

// InitializeLanguage stores a confirmed first-launch choice without changing
// existing display options. Later launches use ordinary live Settings changes.
func InitializeLanguage(o Options, language string) error {
	language, err := localization.Normalize(language)
	if err != nil {
		return err
	}
	path := applicationSettingsPath(o, login.NewAssets(o.Root))
	prefs, err := loadDisplaySettings(path)
	if err != nil {
		return err
	}
	prefs.Language = language
	return prefs.save(path)
}
func (s DisplaySettings) validate() error {
	modeOK, sizeOK, fpsOK := false, false, false
	for _, mode := range displayModes {
		modeOK = modeOK || mode == s.Mode
	}
	for _, size := range displaySizes {
		sizeOK = sizeOK || (size[0] == s.Width && size[1] == s.Height)
	}
	for _, fps := range displayFrameRates {
		fpsOK = fpsOK || fps == s.FPS
	}
	if !modeOK || !sizeOK || !fpsOK {
		return localization.Error("ui.app.invalid_display_settings_e1fac1d2")
	}
	if _, err := localization.Normalize(s.Language); err != nil {
		return err
	}
	return render.ValidateScaling(s.Scaling)
}
func loadDisplaySettings(path string) (DisplaySettings, error) {
	s := defaultDisplaySettings()
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return s, err
	}
	if info.Size() > displayMaximumBytes {
		return s, localization.Error("ui.app.display_settings_file_too_large_9af763ae")
	}
	raw := make([]byte, info.Size())
	if _, err = f.ReadAt(raw, 0); err != nil {
		return s, err
	}
	if err = json.Unmarshal(raw, &s); err == nil {
		err = s.validate()
	}
	if err != nil {
		return defaultDisplaySettings(), err
	}
	if s.Language != "" {
		s.Language, _ = localization.Normalize(s.Language)
	}
	return s, nil
}
func (s DisplaySettings) save(path string) error {
	if err := s.validate(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".display-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = f.Write(append(raw, '\n'))
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

type displayControls struct {
	settings, draft        DisplaySettings
	open                   bool
	preview                bool
	deadline               time.Time
	path                   string
	error                  string
	nextFrame              time.Time
	windowPosition         image.Point
	windowSize             image.Point
	windowMaximized        bool
	haveWindowPosition     bool
	apply                  func(DisplaySettings)
	prepareLanguage        func(string) (func(), error)
	applied                bool
	selectRow, selectIndex int
	selectPointer          image.Point
}

func newDisplayControls(path string) *displayControls {
	s, err := loadDisplaySettings(path)
	if s.Language == "" {
		s.Language = localization.CurrentLanguage()
	}
	d := &displayControls{settings: s, draft: s, path: path, windowSize: image.Pt(workspaceWidth, ScreenHeight)}
	if err != nil {
		d.error = localization.UI("ui.app.could_not_load_display_settings_defaults_8a0ed994")
	}
	return d
}
func (d *displayControls) openMenu() { d.open = true; d.draft = d.settings; d.selectRow = 0 }
func (d *displayControls) revert() {
	if d.preview {
		d.apply(d.settings)
	}
	d.preview = false
	d.selectRow = 0
	d.draft = d.settings
	d.nextFrame = time.Time{}
}
func (d *displayControls) applyDraft(now time.Time) {
	d.selectRow = 0
	if err := d.draft.validate(); err != nil {
		d.error = err.Error()
		return
	}
	before, after := d.settings, d.draft
	before.Language, after.Language = "", ""
	if before == after {
		d.keep()
		return
	}
	d.apply(d.draft)
	d.nextFrame = time.Time{}
	d.preview = true
	d.deadline = now.Add(displayConfirmDuration)
	d.error = ""
}
func (d *displayControls) keep() {
	commitLanguage := func() {}
	if d.prepareLanguage != nil {
		var err error
		commitLanguage, err = d.prepareLanguage(d.draft.Language)
		if err != nil {
			d.error = err.Error()
			d.revert()
			return
		}
	}
	if err := d.draft.save(d.path); err != nil {
		d.error = localization.UI("ui.app.could_not_save_display_settings_1a98165e")
		d.revert()
		return
	}
	commitLanguage()
	d.settings = d.draft
	d.preview = false
	d.open = false
	d.error = ""
}
func (d *displayControls) current() DisplaySettings {
	if d.preview {
		return d.draft
	}
	return d.settings
}
func (d *displayControls) expire(now time.Time) {
	if d.preview && !now.Before(d.deadline) {
		d.revert()
	}
}
func (d *displayControls) pace() {
	fps := d.current().FPS
	if fps <= 0 {
		return
	}
	now := time.Now()
	if d.nextFrame.After(now) {
		time.Sleep(d.nextFrame.Sub(now))
		now = time.Now()
	}
	interval := time.Second / time.Duration(fps)
	if d.nextFrame.IsZero() || now.Sub(d.nextFrame) > interval {
		d.nextFrame = now
	}
	d.nextFrame = d.nextFrame.Add(interval)
}
func (g *Game) applyDisplay(s DisplaySettings) {
	d := g.display
	previousMode := d.current().Mode
	if d.applied && previousMode == s.Mode {
		ebiten.SetVsyncEnabled(s.FPS == displayFPSVSync)
		return
	}
	if s.Mode != displayWindowed && d.current().Mode == displayWindowed && !d.preview {
		x, y := ebiten.WindowPosition()
		d.windowPosition = image.Pt(x, y)
		w, h := ebiten.WindowSize()
		d.windowSize = image.Pt(w, h)
		d.windowMaximized = ebiten.IsWindowMaximized()
		d.haveWindowPosition = true
	}
	ebiten.SetFullscreen(false)
	if s.Mode == displayWindowed {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	} else {
		if ebiten.IsWindowMaximized() || ebiten.IsWindowMinimized() {
			ebiten.RestoreWindow()
		}
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	}
	ebiten.SetWindowDecorated(s.Mode != displayBorderless)
	switch s.Mode {
	case displayFullscreen:
		ebiten.SetFullscreen(true)
	case displayBorderless:
		if monitor := ebiten.Monitor(); monitor != nil {
			w, h := monitor.Size()
			ebiten.SetWindowSize(w, h)
			ebiten.SetWindowPosition(0, 0)
		}
	default:
		if previousMode != displayWindowed {
			ebiten.SetWindowSize(d.windowSize.X, d.windowSize.Y)
			if d.haveWindowPosition {
				ebiten.SetWindowPosition(d.windowPosition.X, d.windowPosition.Y)
			}
			if d.windowMaximized {
				ebiten.MaximizeWindow()
			}
		}
	}
	ebiten.SetVsyncEnabled(s.FPS == displayFPSVSync)
	d.applied = true
}
func displaySettingsButton() image.Rectangle {
	return image.Rect(workspaceWidth-138, 8, workspaceWidth-38, 32)
}
func displayValueRect(row int) image.Rectangle {
	return image.Rect(displayControlLeft, displayRowTop+row*displayRowHeight, displayControlLeft+displayControlWidth, displayRowTop+row*displayRowHeight+30)
}
func displayActionRect(right bool) image.Rectangle {
	x := displayPanelLeft + 30
	if right {
		x = displayPanelLeft + displayPanelWidth - 150
	}
	return image.Rect(x, displayButtonTop, x+120, displayButtonTop+displayButtonHeight)
}

func displayFrameRateLabel(fps int) string {
	if fps == displayFPSVSync {
		return "VSync"
	}
	if fps == displayFPSUnlimited {
		return localization.UI("ui.app.unlimited_11dde17d")
	}
	return fmt.Sprintf(localization.UI("ui.app.d_fps_1353da03"), fps)
}

func displayModeLabel(mode string) string {
	switch mode {
	case displayFullscreen:
		return localization.UI("ui.app.fullscreen_c461dbb2")
	case displayBorderless:
		return localization.UI("ui.app.borderless_fullscreen_2b0a3048")
	default:
		return localization.UI("ui.app.windowed_3f26603a")
	}
}

func (d *displayControls) selectLabels() []string {
	var labels []string
	if d.selectRow == displayModeRow {
		for _, mode := range displayModes {
			labels = append(labels, displayModeLabel(mode))
		}
	} else if d.selectRow == displayResolutionRow {
		for _, size := range displaySizes {
			labels = append(labels, fmt.Sprintf("%d x %d", size[0], size[1]))
		}
	} else if d.selectRow == displayFrameRateRow {
		for _, fps := range displayFrameRates {
			labels = append(labels, displayFrameRateLabel(fps))
		}
	} else if d.selectRow == displayScalingRow {
		for _, mode := range render.ScalingModes() {
			labels = append(labels, render.ScalingLabel(mode))
		}
	} else if d.selectRow == displayLanguageRow {
		labels = []string{localization.UI("ui.settings.english"), localization.UI("ui.settings.chinese")}
	}
	return labels
}

func (d *displayControls) selectRect() image.Rectangle {
	r := displayValueRect(d.selectRow - 1)
	return image.Rect(r.Min.X, r.Max.Y, r.Max.X, r.Max.Y+len(d.selectLabels())*displaySelectItemHeight)
}

func (d *displayControls) openSelect(row int, pointer image.Point) {
	if row == displayResolutionRow && !displayResolutionPickingEnabled {
		return
	}
	d.selectRow, d.selectIndex, d.selectPointer = row, 0, pointer
	if row == displayModeRow {
		for i, mode := range displayModes {
			if mode == d.draft.Mode {
				d.selectIndex = i
				break
			}
		}
	} else if row == displayResolutionRow {
		for i, size := range displaySizes {
			if size[0] == d.draft.Width && size[1] == d.draft.Height {
				d.selectIndex = i
				break
			}
		}
	} else if row == displayFrameRateRow {
		for i, fps := range displayFrameRates {
			if fps == d.draft.FPS {
				d.selectIndex = i
				break
			}
		}
	} else if row == displayScalingRow {
		for i, mode := range render.ScalingModes() {
			if mode == d.draft.Scaling {
				d.selectIndex = i
				break
			}
		}
	} else if row == displayLanguageRow && d.draft.Language == localization.Chinese {
		d.selectIndex = 1
	}
}

func (d *displayControls) chooseSelect(index int) {
	if index < 0 || index >= len(d.selectLabels()) {
		return
	}
	if d.selectRow == displayModeRow {
		d.draft.Mode = displayModes[index]
	} else if d.selectRow == displayResolutionRow {
		if !displayResolutionPickingEnabled {
			d.selectRow = 0
			return
		}
		size := displaySizes[index]
		d.draft.Width, d.draft.Height = size[0], size[1]
	} else if d.selectRow == displayFrameRateRow {
		d.draft.FPS = displayFrameRates[index]
	} else if d.selectRow == displayScalingRow {
		d.draft.Scaling = render.ScalingModes()[index]
	} else if d.selectRow == displayLanguageRow {
		d.draft.Language = []string{localization.English, localization.Chinese}[index]
	}
	d.selectRow = 0
}

// The open list owns the press, including outside clicks which only dismiss it.
func (d *displayControls) selectClick(pt image.Point) {
	r := d.selectRect()
	if pt.In(r) {
		d.chooseSelect((pt.Y - r.Min.Y) / displaySelectItemHeight)
	} else {
		d.selectRow = 0
	}
}

// Consume modal input before it can reach any session. Network and simulation
// updates continue while the display controls are open.
func (g *Game) displayInput(now time.Time) bool {
	d := g.display
	if d == nil {
		return false
	}
	d.expire(now)
	press := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft)
	x, y := g.workspaceCursor()
	pt := image.Pt(x, y)
	if inpututil.IsKeyJustPressed(ebiten.KeyF10) || (!d.open && !g.Workspace.Collapsed && press && pt.In(displaySettingsButton())) {
		if d.open {
			d.revert()
			d.open = false
		} else {
			d.openMenu()
		}
		g.C.cancelSessionInput()
		return true
	}
	if !d.open {
		return false
	}
	if d.selectRow != 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
			d.selectRow = 0
			return true
		}
		r := d.selectRect()
		if pt != d.selectPointer {
			d.selectPointer = pt
			if pt.In(r) {
				d.selectIndex = (pt.Y - r.Min.Y) / displaySelectItemHeight
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) {
			d.selectIndex = max(0, d.selectIndex-1)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) {
			d.selectIndex = min(len(d.selectLabels())-1, d.selectIndex+1)
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyNumpadEnter) {
			d.chooseSelect(d.selectIndex)
			return true
		}
		if press {
			d.selectClick(pt)
		}
		return true
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		d.revert()
		d.open = false
		return true
	}
	if !press {
		return true
	}
	if pt.In(displayActionRect(true)) {
		if d.preview {
			d.keep()
		} else {
			d.revert()
			d.open = false
		}
		return true
	}
	if pt.In(displayActionRect(false)) {
		if d.preview {
			d.revert()
		} else {
			d.applyDraft(now)
		}
		return true
	}
	if d.preview {
		return true
	}
	for row := 0; row < displayRowCount; row++ {
		if !pt.In(displayValueRect(row)) {
			continue
		}
		d.openSelect(row+1, pt)
	}
	return true
}
func (g *Game) drawDisplay(dst *surface.Surface) {
	d := g.display
	if d == nil {
		return
	}
	text := func(x, y, width int, s string) {
		g.C.Env.Text.Draw(x, y, 0, false, true, dst, []byte(s), 16, width, 0, sessionPanelInk, 0)
	}
	button := func(r image.Rectangle, label string) {
		dst.Fill(r, sessionRowFill)
		dst.Frame(r, sessionBorderInk)
		text(r.Min.X+9, r.Min.Y+6, r.Dx()-18, label)
	}
	if !g.Workspace.Collapsed {
		button(displaySettingsButton(), localization.UI("ui.app.settings"))
	}
	if !d.open {
		return
	}
	panel := image.Rect(displayPanelLeft, displayPanelTop, displayPanelLeft+displayPanelWidth, displayPanelTop+displayPanelHeight)
	dst.FillAlpha(image.Rect(0, 0, workspaceWidth, ScreenHeight), 0, 140)
	dst.Fill(panel, sessionPanelFill)
	dst.Frame(panel, sessionAccentInk)
	text(displayPanelLeft+24, displayPanelTop+18, 450, localization.UI("ui.app.settings_f10"))
	mode := displayModeLabel(d.draft.Mode)
	size := fmt.Sprintf("%d x %d", d.draft.Width, d.draft.Height)
	if !displayResolutionPickingEnabled {
		size = fmt.Sprintf(localization.UI("ui.app.d_x_d_fixed_4475e024"), workspaceWidth, ScreenHeight)
	}
	fps := displayFrameRateLabel(d.draft.FPS)
	language := localization.UI("ui.settings.english")
	if d.draft.Language == localization.Chinese {
		language = localization.UI("ui.settings.chinese")
	}
	for i, row := range []struct{ name, value string }{{localization.UI("ui.app.mode_5e23ec6a"), mode}, {localization.UI("ui.app.render_resolution_992e3361"), size}, {localization.UI("ui.app.frame_limit_cd2533e0"), fps}, {localization.UI("ui.app.scaling_filter_325b8c64"), render.ScalingLabel(d.draft.Scaling)}, {localization.UI("ui.settings.language"), language}} {
		text(displayPanelLeft+24, displayRowTop+i*displayRowHeight+7, 180, row.name)
		value := row.value
		if !d.preview && (i != displayResolutionRow-1 || displayResolutionPickingEnabled) {
			value += "   v"
		}
		button(displayValueRect(i), value)
	}
	if d.preview {
		text(displayPanelLeft+24, displayPanelTop+272, 450, fmt.Sprintf(localization.UI("ui.app.keep_these_settings_reverts_in_d_seconds_7abc12e5"), max(0, int(time.Until(d.deadline).Seconds())+1)))
		button(displayActionRect(false), localization.UI("ui.app.revert_0026c505"))
		button(displayActionRect(true), localization.UI("ui.app.keep_183f00f4"))
	} else {
		text(displayPanelLeft+24, displayPanelTop+272, 450, localization.UI("ui.settings.language_live"))
		button(displayActionRect(false), localization.UI("ui.app.apply_31e392d1"))
		button(displayActionRect(true), localization.UI("ui.app.cancel_19766ed6"))
	}
	if d.error != "" {
		text(displayPanelLeft+24, displayPanelTop+298, 450, d.error)
	}
	text(displayPanelLeft+24, displayPanelTop+380, 450, localization.UI("ui.app.resize_or_maximize_the_window_independen_9f4b37e4"))
	if d.selectRow != 0 {
		r := d.selectRect()
		dst.Fill(r, sessionRowFill)
		for i, label := range d.selectLabels() {
			y := r.Min.Y + i*displaySelectItemHeight
			if i == d.selectIndex {
				dst.Fill(image.Rect(r.Min.X+1, y+1, r.Max.X-1, y+displaySelectItemHeight), sessionActiveFill)
			}
			text(r.Min.X+10, y+5, r.Dx()-20, label)
		}
		dst.Frame(r, sessionAccentInk)
	}
}
