package app

import "wonderland-gonline/internal/localization"

import (
	"image"
	"time"

	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

const (
	mapLoadingPicture = "Anim_load0"
	mapLoadingLeave   = 120 * time.Millisecond
	mapLoadingShow    = 100 * time.Millisecond
	mapLoadingHold    = 260 * time.Millisecond
	mapLoadingHide    = 100 * time.Millisecond
	mapLoadingReveal  = 220 * time.Millisecond
	mapLoadingMinimum = mapLoadingLeave + mapLoadingShow + mapLoadingHold + mapLoadingHide + mapLoadingReveal
)

// Presentation only: map acknowledgements and incoming packets continue at
// their normal pace. Never sleep the game loop to lengthen a loading screen.
type mapLoading struct {
	active           bool
	started, closing time.Time
	departing        *surface.Surface
	assetsReady      bool // A preceding covered frame finished local sprite preparation.
	progress         float64
}

func (c *Client) startMapLoading() {
	c.stopMapLoading()
	c.mapLoading = mapLoading{active: true, departing: c.Screen.Clone()}
	c.cancelSessionInput()
}

func (c *Client) stopMapLoading() {
	if c.mapLoading.departing != nil {
		c.mapLoading.departing.Close()
	}
	c.mapLoading = mapLoading{}
}

func loadingEase(elapsed, duration time.Duration) float64 {
	p := min(1.0, max(0.0, float64(elapsed)/float64(duration)))
	return p * p * (3 - 2*p)
}

// tickMapLoading advances readiness and the transition clock even when no scene
// is drawn. Headless hosts and hidden sessions must not depend on a viewer or
// screenshot request to release their input lock after server readiness.
func (c *Client) tickMapLoading(now time.Time) {
	f := &c.mapLoading
	if !f.active {
		return
	}
	if f.started.IsZero() {
		f.started = now
	}
	elapsed := now.Sub(f.started)
	ready, progress := c.mapLoadingProgress()
	// Local preparation can discover additional archives as peers arrive. The
	// display stays monotonic, but completion always uses the actual readiness.
	f.progress = max(f.progress, progress)
	if f.closing.IsZero() && ready && elapsed >= mapLoadingLeave+mapLoadingShow+mapLoadingHold {
		f.closing = now
	}
	if !f.closing.IsZero() && now.Sub(f.closing) >= mapLoadingHide+mapLoadingReveal {
		c.stopMapLoading()
	}
}

// Draw after the destination has rendered underneath, including its UI. A slow
// server holds the illustration until its readiness packet. Direct rendering
// callers also advance the transition, as the ordinary frame loop does.
func (c *Client) drawMapLoading(now time.Time) {
	c.tickMapLoading(now)
	f := &c.mapLoading
	if !f.active {
		return
	}
	elapsed := now.Sub(f.started)
	r := image.Rect(0, 0, ScreenWidth, ScreenHeight)
	if !f.closing.IsZero() && now.Sub(f.closing) >= mapLoadingHide {
		reveal := now.Sub(f.closing) - mapLoadingHide
		c.Screen.FillAlpha(r, 0, int(255*(1-loadingEase(reveal, mapLoadingReveal))))
		return
	}
	if elapsed < mapLoadingLeave {
		c.Screen.Draw(0, 0, f.departing, false)
		c.Screen.FillAlpha(r, 0, int(255*loadingEase(elapsed, mapLoadingLeave)))
		return
	}
	var art *surface.Surface
	if c.resources != nil {
		art = c.resources.loadingArtwork
	}
	if art == nil {
		art = c.background
	}
	c.Screen.Fill(r, 0)
	if art != nil {
		c.Screen.DrawStretch(r, art, false)
	}
	// A small backing keeps the status readable over either loading illustration.
	c.Screen.FillAlpha(image.Rect(0, ScreenHeight-64, ScreenWidth, ScreenHeight), 0, 180)
	name := localization.UI("ui.app.loading_dc380888")
	if c.World != nil && c.World.SceneName != "" {
		name += " - " + string(clientassets.Big5Text(c.World.SceneName))
	}
	const dotInterval = 300 * time.Millisecond
	for i := 0; i < int(elapsed/dotInterval)%4; i++ {
		name += "."
	}
	if c.Env.Text != nil {
		c.Env.Text.Draw(24, ScreenHeight-48, 0, false, true, c.Screen, []byte(name), 16, ScreenWidth-48, 0, 0xffff, 0)
	}
	c.drawMapLoadingBar(f.progress)
	shade := int(255 * (1 - loadingEase(elapsed-mapLoadingLeave, mapLoadingShow)))
	if !f.closing.IsZero() {
		shade = int(255 * loadingEase(now.Sub(f.closing), mapLoadingHide))
	}
	c.Screen.FillAlpha(r, 0, shade)
}

// Map/terrain, NPC definitions, questions and minimap load synchronously before
// AC12:1. Rendering underneath this screen warms visible terrain and actor frames;
// editable sprite palettes finish asynchronously. Require a further covered frame
// after those jobs settle so the actors are rendered with their final colours.
func (c *Client) mapLoadingProgress() (ready bool, progress float64) {
	f := &c.mapLoading
	settled, total := 0, 0
	if c.Sprites != nil {
		settled, total = c.Sprites.NativeProgress()
	}
	localReady := settled == total
	ready = c.World != nil && c.mapReady && localReady && f.assetsReady
	f.assetsReady = localReady
	progress = 0.5 // Synchronous map assets have returned before the first covered frame.
	if total > 0 {
		progress += 0.25 * float64(settled) / float64(total)
	} else {
		progress += 0.25
	}
	if c.mapReady {
		progress += 0.2
	}
	if ready {
		progress = 1
	}
	return
}

func (c *Client) drawMapLoadingBar(progress float64) {
	bar := image.Rect(24, ScreenHeight-24, ScreenWidth-24, ScreenHeight-12)
	c.Screen.Fill(bar, 0x8c71) // Muted silver frame, RGB565.
	bar = bar.Inset(1)
	c.Screen.Fill(bar, 0x1082)
	fill := bar
	fill.Max.X = fill.Min.X + int(float64(fill.Dx())*min(1.0, max(0.0, progress)))
	c.Screen.Fill(fill, 0x4d9f)
	fill.Max.Y = fill.Min.Y + max(1, fill.Dy()/3)
	c.Screen.Fill(fill, 0x9eff)
}
