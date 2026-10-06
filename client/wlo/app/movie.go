package app

import (
	"encoding/binary"
	"image"
	"log"
	"strconv"
	"strings"
	"wonderland-gonline/internal/clientimage"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/movie"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/weather"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/clientassets"
)

// Movies (event kind 5, FUN_00304fd0 case 5): the frame's value names the
// movie (<value>.sty) and its mode the game mode while it plays (1 and 3
// set PTR_DAT_004ca2f8 +0x1d to 1, 2 and 4 to 4), which hides the HUD.
// The movie draws its own view: the scene of its background map, its
// actors and camera (TMovie, FUN_00340404). Its lines go to the talk
// window; a click closes a line. When the movie ends the game mode returns
// to 0 (or 2 after mode 4) and the step is acknowledged with 20/6, as the
// interpreter marked it done when the movie started (+0x7108) and the
// acknowledgement waits for the movie.
//
// The timeline's effects, music and keyframe sounds are in movie/effects.go
// and soundtable.go, the pictures in movie/pictures.go. Not ported yet: the
// weather overlays (1, 2, 3, 10), looping keyframe sounds, pictures' light
// blit and their place in the depth order (drawn over the scene here), the
// actors' trails and draw modes (keyframe +0x1d), and party-member
// speakers.
const (
	eventFrameValue  = 10 // u32
	movieScenePrefix = "Map"
	// movieModeNormal and movieModeWide are the game modes a movie sets;
	// both hide the HUD and lower the wide talk form.
	movieModeNormal = 1
	movieModeWide   = 4
)

// moviePlay is a running movie.
type moviePlay struct {
	p    *movie.Player
	view *world.World
	npcs []*world.NPC // the actors after the player, in order
	mode byte
	pics map[string]*surface.Surface // the pictures' images, keyed
}

// pictureArchives are the image archives (pic\*.BMg) a picture's name is
// looked up in; the original registers them all in one picture database.
var pictureArchives = []string{"images3", "images", "images1", "images4", "images3_01", "images3_c01", "images1_d01", "images4_c01", "images_c01"}

// picture loads a movie picture's image with its colour key.
func (c *Client) picture(mp *moviePlay, pic *movie.Picture) *surface.Surface {
	if s, ok := mp.pics[pic.Name]; ok {
		return s
	}
	var s *surface.Surface
	for _, arc := range pictureArchives {
		if compiled, err := c.Assets.CompiledPicture(arc, pic.Name); err == nil {
			if pixels, err := compiled.Pixels(compiled.Bounds(), clientimage.Plain); err == nil {
				b := compiled.Bounds()
				s = &surface.Surface{W: b.Dx(), H: b.Dy(), Pix: pixels, Key: pic.Key()}
				break
			}
		}
		if m, err := c.Assets.LoadPicture(arc, pic.Name); err == nil {
			s = surface.FromImage(m)
			s.Key = pic.Key()
			break
		}
	}
	mp.pics[pic.Name] = s
	return s
}

// drawPictures is FUN_00342508 for each picture drawn in the stage: one
// frame of its strip, centred on its point with its bottom on it. Pictures
// below the full light level (+0x2164 < 255) use the light blit in the
// original; the movies seen so far use 255, so all are drawn keyed.
func (c *Client) drawPictures(mp *moviePlay) {
	p := mp.p
	cam := p.Camera.Add(p.ShakeOffset())
	for _, ps := range p.Pictures {
		if !ps.Drawn(p.Stage) {
			continue
		}
		img := c.picture(mp, ps.Pic)
		if img == nil {
			continue
		}
		rows, cols := ps.Pic.Grid()
		w, h := img.W/cols, img.H/rows
		pt := ps.Point()
		c.Screen.DrawRect(pt.X-w/2-cam.X, pt.Y-h-cam.Y, image.Rect(0, (ps.Frame-1)*h, w, ps.Frame*h), img, true)
	}
}

// startMovie is kind 5.
func (c *Client) startMovie(s []byte) {
	id := int(binary.LittleEndian.Uint32(s[eventFrameValue:]))
	m, err := movie.Load(c.Assets, id)
	if err != nil {
		log.Printf("movie %d: %v", id, err)
		c.event.done = true
		return
	}
	view, err := c.movieView(m)
	if err != nil {
		log.Printf("movie %d: %v", id, err)
		c.event.done = true
		return
	}
	mp := &moviePlay{view: view, mode: movieModeNormal, pics: map[string]*surface.Surface{}}
	if mode := s[eventFrameMode]; mode == 2 || mode == 4 {
		mp.mode = movieModeWide
	}
	if c.npcTemplates == nil {
		c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	}
	for _, a := range m.NPCs {
		n := &world.NPC{Template: a.Template, Shown: true, Action: movie.NormalPose}
		if t, ok := c.npcTemplates[a.Template]; ok {
			n.Info = t
			if c.lib != nil {
				n.Painter = role.NewNPC(c.lib, t.Look, t.Colors)
			}
		}
		mp.npcs = append(mp.npcs, n)
	}
	view.NPCs = map[uint16]*world.NPC{}
	for i, n := range mp.npcs {
		view.NPCs[uint16(i+1)] = n
	}
	if m.Player != nil && c.World != nil && c.lib != nil {
		view.Player = c.World.Player
		body := role.NewHuman(c.lib, c.items)
		world.Dress(body, view.Player)
		view.Body = body
	}
	view.Now = c.World.Now
	mp.p = &movie.Player{M: m, Now: c.Now, Facing: world.Facing,
		Say: func(l movie.Line) { c.movieSay(mp, l) }, Talking: c.Talk.Shown,
		Music:    func(i int) { c.playTableMusic(i) },
		MapMusic: c.playMapMusic,
		Sound:    func(i int, _ bool) { c.playTableSound(i) }}
	mp.p.Start()
	c.movie = mp
	c.Talk.Lowered = true
	c.sync(mp)
}

// movieView is the view of the movie's background map ("Map10001").
func (c *Client) movieView(m *movie.Movie) (*world.World, error) {
	id, err := strconv.Atoi(strings.TrimPrefix(m.Camera.Scene, movieScenePrefix))
	if err != nil {
		id = int(c.World.Player.Map)
	}
	return world.NewView(c.Env, c.Assets, uint16(id))
}

// movieSay is FUN_003430c0: the line in the talk window, said by the
// player (the wide form) or an NPC template (its face or body).
func (c *Client) movieSay(mp *moviePlay, l movie.Line) {
	c.loadTalks()
	text := clientassets.Big5Text(c.talks[l.Talk])
	var who hud.Speaker
	if l.Speaker == movie.PlayerTemplate {
		who = c.speaker(eventSpeakerSelf, 0)
	} else if t, ok := c.npcTemplates[l.Speaker]; ok {
		n := &world.NPC{Template: l.Speaker, Info: t}
		for _, a := range mp.npcs {
			if a.Template == l.Speaker {
				n = a
				break
			}
		}
		if n.Painter == nil && c.lib != nil {
			n.Painter = role.NewNPC(c.lib, t.Look, t.Colors)
		}
		who = npcSpeaker(n)
	}
	c.Talk.Say(text, who, c.World.Player.Name)
}

// sync copies the movie's actors and camera into its view.
// The shake moves the camera (+4000, +0xfa4), only the actors of the
// current stage are drawn (FUN_00340404), and each shows its keyframe's
// fixed frame or its own animation (FUN_00339930).
func (c *Client) sync(mp *moviePlay) {
	p := mp.p
	*mp.view.CameraAt = p.Camera.Add(p.ShakeOffset())
	mp.view.HideScene = p.HideScene
	i := 0
	for _, a := range p.Actors {
		pt := a.Point()
		action := a.Action()
		frame, fixed := a.Frame()
		if a.Actor == p.M.Player {
			mp.view.Player.X, mp.view.Player.Y = pt.X, pt.Y
			mp.view.Player.Direction = int32(action)
			mp.view.HidePlayer = !p.Shown(a)
			if h, ok := mp.view.Body.(interface{ Hold(int, bool) }); ok {
				h.Hold(frame, !fixed)
			}
			continue
		}
		n := mp.npcs[i]
		i++
		n.X, n.Y, n.Action, n.Shown = pt.X, pt.Y, action, p.Shown(a)
		n.Fixed, n.Wrap, n.Frame = true, !fixed, frame
	}
}

// drawEffects paints the stage's overlay fill (FUN_003346e8: ro_ARGB
// through ro_Clipper_Alpha_Fill) and the scene's light (DAT_0072a090).
func (c *Client) drawEffects(p *movie.Player) {
	full := image.Rect(0, 0, ScreenWidth, ScreenHeight)
	if a, r, g, b, ok := p.Fill(); ok {
		c.Screen.FillAlpha(full, uint32(r)|uint32(g)<<8|uint32(b)<<16, int(a))
	}
	if p.Light < lightFull {
		c.Screen.FillAlpha(full, 0, lightFull-p.Light)
	}
}

// zoomScreen is the main form's view rectangle (+0x584): each zoom step
// trims 20 by 15 pixels from every side, and the rest fills the window.
func (c *Client) zoomScreen(zoom int) {
	if zoom <= 0 {
		return
	}
	r := image.Rect(zoom*movie.ZoomStepW, zoom*movie.ZoomStepH, ScreenWidth-zoom*movie.ZoomStepW, ScreenHeight-zoom*movie.ZoomStepH)
	src := c.Screen.NewCompatible(r.Dx(), r.Dy())
	defer src.Close()
	src.DrawRect(0, 0, r, c.Screen, false)
	c.Screen.DrawStretch(image.Rect(0, 0, ScreenWidth, ScreenHeight), src, false)
}

// lightFull is DAT_0072a090's full light.
const lightFull = 0xff

// movieFrame runs and draws the movie; it reports false when none plays.
func (c *Client) movieFrame() bool {
	mp := c.movie
	if mp == nil {
		return false
	}
	mp.p.Tick()
	c.sync(mp)
	// The movie's draw runs the weather passes with its overlay (+0xf9a)
	// rather than the map's weather.
	mp.view.Weather, mp.view.WeatherKind = c.weatherLayer(), weather.Kind(mp.p.Overlay)
	mp.view.Draw()
	c.drawPictures(mp)
	c.drawEffects(mp.p)
	c.Talk.Draw()
	c.zoomScreen(mp.p.Zoom)
	if mp.p.Ended() {
		c.endMovie()
	}
	return true
}

// endMovie restores the game mode and acknowledges the step.
func (c *Client) endMovie() {
	c.movie = nil
	c.Talk.Hide()
	c.Talk.Lowered = false
	c.event.done = true
}

// MoviePlaying reports whether a movie is running.
func (c *Client) MoviePlaying() bool { return c.movie != nil }
