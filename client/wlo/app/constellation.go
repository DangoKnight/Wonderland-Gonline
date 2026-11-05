package app

import (
	"image"
	"math"
	"strconv"
	"time"
)

// Native CH_StarSystem: FUN_003113d4/00312168 and x87 motion in
// FUN_00310d10. The latter uses 0.2 pixels/ms on the major axis.
const (
	constellationLimit                = 89
	constellationRiseStep             = 10
	constellationRiseTick             = 60 * time.Millisecond
	constellationPixelsPerMillisecond = 0.2
	constellationHold                 = 5 * 300 * time.Millisecond
	constellationFadeSteps            = 20
	constellationFadeTick             = 30 * time.Millisecond
	constellationSkyTick              = 20 * time.Millisecond
	constellationCenterX              = 416
	constellationCenterY              = 300
	constellationSkyLeft              = -16
	constellationSkyTop               = -20
	constellationSkyWidth             = 832
	constellationSkyArchive           = "jRole"
	constellationSkyPicture           = "Star_Sky"
	constellationSound                = `Sound\WAV0149.wav`
	constellationMusic                = `Sound\BGM0017.wav`
)

type constellationRun struct {
	id             byte
	picture, sky   int
	width, height  int // Each picture has two vertical halves: grey and colored.
	startX, startY int
	started        time.Time
	rise, flight   time.Duration
	previousMusic  string
}

func (c *Client) receiveConstellations(p []byte) {
	if len(p) < 3 || len(p) != 3+int(p[2]) {
		return
	}
	var owned [constellationLimit]bool
	for _, id := range p[3:] {
		if id == 0 || int(id) >= len(owned) {
			return
		}
		owned[id] = true
	}
	c.constellations = owned
}

func (c *Client) startConstellation(p []byte) {
	if len(p) != 4 || p[2] == 0 || int(p[2]) >= constellationLimit || c.World == nil {
		return
	}
	name := "Star_" + strconv.Itoa(int(p[2]))
	c.loadPictures(name, "Star_Sky")
	if c.Pics.Find(constellationSkyPicture) < 0 {
		if compiled, err := c.Assets.CompiledPicture(constellationSkyArchive, constellationSkyPicture); err == nil {
			_ = c.Pics.AddCompiled(constellationSkyPicture, compiled)
		} else if picture, err := c.Assets.LoadPicture(constellationSkyArchive, constellationSkyPicture); err == nil {
			c.Pics.Add(constellationSkyPicture, picture)
		}
	}
	picture := c.Pics.Find(name)
	width, fullHeight := c.Pics.Size(picture)
	if picture < 0 || width <= 0 || fullHeight < 2 || fullHeight%2 != 0 {
		return
	}
	cx, cy := c.World.Camera()
	x, y := c.World.Player.X-cx, c.World.Player.Y-cy
	if p[3] != 0 {
		n := c.World.NPCs[uint16(p[3])]
		if n == nil {
			return
		}
		x, y = n.X-cx, n.Y-cy
	}
	c.stopConstellation()
	height := fullHeight / 2
	run := &constellationRun{id: p[2], picture: picture, sky: c.Pics.Find("Star_Sky"), width: width, height: height, startX: x - width/2, startY: y - height/2, started: c.Now()}
	steps := max(1, (run.startY+height+constellationRiseStep)/constellationRiseStep)
	run.rise = time.Duration(steps) * constellationRiseTick
	tx, ty := constellationCenterX-width/2, constellationCenterY-height/2
	distance := max(abs(tx-run.startX), abs(ty-ScreenHeight))
	run.flight = time.Duration(math.Ceil(float64(distance)/constellationPixelsPerMillisecond)) * time.Millisecond
	if c.Music != nil {
		c.Music.mu.Lock()
		run.previousMusic = c.Music.current
		c.Music.mu.Unlock()
		c.Music.Play(constellationMusic)
	}
	if c.Env.Sound != nil {
		c.Env.Sound(constellationSound)
	}
	c.World.StopWalk()
	c.Talk.Hide()
	c.constellation = run
}

func (r *constellationRun) duration() time.Duration {
	return r.rise + r.flight + constellationHold + 2*constellationFadeSteps*constellationFadeTick
}

func (c *Client) constellationTick(now time.Time) {
	if run := c.constellation; run != nil && now.Sub(run.started) >= run.duration() {
		c.constellations[run.id] = true
		c.stopConstellation()
	}
}

func (c *Client) stopConstellation() {
	run := c.constellation
	if run == nil {
		return
	}
	c.constellation = nil
	if c.Music != nil {
		if run.previousMusic != "" {
			c.Music.Play(run.previousMusic)
		} else {
			c.playMapMusic()
		}
	}
}

func (c *Client) drawConstellation(now time.Time) {
	r := c.constellation
	if r == nil {
		return
	}
	elapsed := max(now.Sub(r.started), 0)
	fadeAt := r.rise + r.flight + constellationHold
	alpha := 0
	if elapsed >= fadeAt {
		step := min(int((elapsed-fadeAt)/constellationFadeTick), 2*constellationFadeSteps)
		if step <= constellationFadeSteps {
			alpha = step * 255 / constellationFadeSteps
		} else {
			alpha = (2*constellationFadeSteps - step) * 255 / constellationFadeSteps
		}
		if step >= constellationFadeSteps {
			c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, alpha)
			return
		}
	}
	x, y := r.startX, r.startY
	row := 0
	if elapsed < r.rise {
		y -= int(elapsed/constellationRiseTick) * constellationRiseStep
	} else {
		if r.sky >= 0 {
			scroll := constellationSkyLeft + int((elapsed-r.rise)/constellationSkyTick)%constellationSkyWidth
			c.Pics.Draw(c.Screen, r.sky, scroll, constellationSkyTop, false)
			c.Pics.Draw(c.Screen, r.sky, scroll-constellationSkyWidth, constellationSkyTop, false)
		}
		tx, ty := constellationCenterX-r.width/2, constellationCenterY-r.height/2
		fraction := 1.0
		if r.flight > 0 {
			fraction = min(float64(elapsed-r.rise)/float64(r.flight), 1)
		}
		x = int(math.Round(float64(r.startX) + float64(tx-r.startX)*fraction))
		y = int(math.Round(float64(ScreenHeight) + float64(ty-ScreenHeight)*fraction))
		if elapsed >= r.rise+r.flight {
			row = r.height
		}
	}
	c.Pics.DrawRect(c.Screen, r.picture, x, y, image.Rect(0, row, r.width, row+r.height), true)
	if alpha > 0 {
		c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, alpha)
	}
}
