package app

import (
	"encoding/binary"
	"log"
	"strconv"
	"strings"

	"wonderland-go/client/wlo/hud"
	"wonderland-go/client/wlo/movie"
	"wonderland-go/client/wlo/role"
	"wonderland-go/client/wlo/world"
	"wonderland-go/internal/clientassets"
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
// Not ported yet: the screen effects of the timeline (overlay, flashes,
// shaking), its sounds and music, the pictures (images), the actors'
// party-member speakers, and the music change at the end.
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
	mp := &moviePlay{view: view, mode: movieModeNormal}
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
		Say: func(l movie.Line) { c.movieSay(mp, l) }, Talking: c.Talk.Shown}
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
func (c *Client) sync(mp *moviePlay) {
	p := mp.p
	cam := p.Camera
	*mp.view.CameraAt = cam
	i := 0
	for _, a := range p.Actors {
		pt := a.Point()
		action := a.Action()
		if a.Actor == p.M.Player {
			mp.view.Player.X, mp.view.Player.Y = pt.X, pt.Y
			mp.view.Player.Direction = int32(action)
			continue
		}
		n := mp.npcs[i]
		i++
		n.X, n.Y, n.Action, n.Shown = pt.X, pt.Y, action, a.Active(p.Stage) || a.Key > 0
	}
}

// movieFrame runs and draws the movie; it reports false when none plays.
func (c *Client) movieFrame() bool {
	mp := c.movie
	if mp == nil {
		return false
	}
	mp.p.Tick()
	c.sync(mp)
	mp.view.Draw()
	c.Talk.Draw()
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
