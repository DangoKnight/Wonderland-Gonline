package world

import (
	"encoding/binary"
	"errors"
	"image"
	"slices"
	"sort"
	"strconv"
	"time"

	"wonderland-go/client/wlo/login"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/weather"
)

// Screen layout of the static view, measured from the original's capture
// (In-Game/Ship_Deck.png): the camera keeps the player at the screen
// centre, the name is centred above, and the location line ends at the
// right edge above the bottom bar.
const (
	screenW, screenH = 800, 600
	charW            = 8
	nameY            = 0xc8
	locationRight    = 0x316
	locationY        = 0x218
	nameInk          = 0xffe0
	locationInk      = 0xffff
	textStyle        = 2
	standingFront    = 0xb // the selection's facing (+0x121)
)

// Player is the character from packet AC3 (receive case 0x2dfdb3), as
// the server sends it: ID, body type, map, position, head and look, the
// colour values, the worn items and the name.
type Player struct {
	ID        uint32
	Body      byte
	Map       uint16
	X, Y      int
	Head      byte
	Look      byte
	Color1    uint32
	Color2    uint32
	Items     []uint16
	Name      []byte
	Direction int32
}

var errShort = errors.New("AC3: packet too short")

// ParseSelf reads AC3 after its command byte.
func ParseSelf(p []byte) (Player, error) {
	var pl Player
	r := reader{b: p}
	pl.ID = r.u32()
	pl.Body = r.u8()
	pl.Map = r.u16()
	pl.X, pl.Y = int(r.u16()), int(r.u16())
	r.u8() // +0xb2
	pl.Head, pl.Look = r.u8(), r.u8()
	pl.Color1, pl.Color2 = r.u32(), r.u32()
	for n := int(r.u8()); n > 0; n-- {
		pl.Items = append(pl.Items, r.u16())
	}
	r.u32()
	pl.Name = r.str()
	if r.bad {
		return pl, errShort
	}
	pl.Direction = standingFront
	return pl, nil
}

type reader struct {
	b   []byte
	bad bool
}

func (r *reader) take(n int) []byte {
	if len(r.b) < n {
		r.bad, r.b = true, nil
		return make([]byte, n)
	}
	v := r.b[:n]
	r.b = r.b[n:]
	return v
}
func (r *reader) u8() byte    { return r.take(1)[0] }
func (r *reader) u16() uint16 { return binary.LittleEndian.Uint16(r.take(2)) }
func (r *reader) u32() uint32 { return binary.LittleEndian.Uint32(r.take(4)) }
func (r *reader) str() []byte { return append([]byte(nil), r.take(int(r.u8()))...) }

// World is the in-game state.
type World struct {
	Env       *seui.Env
	Scene     *Scene
	SceneName string
	Player    Player
	Body      login.RoleView
	// NPCs are the map's NPC objects by click ID (MapNPCs).
	NPCs map[uint16]*NPC
	// Questions are the map's event questions by click ID.
	Questions map[uint16]Question
	// Areas are the map's event areas (MapAreas); lightsFrom starts the
	// door lights' frames.
	Areas      []Area
	lightsFrom time.Time
	// OnLeg is called as each leg of a walk starts, with the facing and
	// the waypoint (the client sends 6/1).
	OnLeg  func(facing, x, y int)
	walker Walker
	// Peers are the other players on the map, by ID (peers.go).
	Peers   map[uint32]*Peer
	hovered *NPC
	// CameraAt, when set, is the camera's top-left instead of the
	// player-centred one.
	CameraAt *image.Point
	// Cinematic leaves out the location line and the walk marker (a
	// movie's view).
	Cinematic bool
	// HideScene leaves out the background, HidePlayer the player (a
	// movie's stage effects and cast).
	HideScene, HidePlayer bool
	// Marker is the walk marker at a mouse walk's destination.
	Marker Marker
	// HideNames leaves out the characters' names, as during a
	// conversation (both talking captures show none).
	HideNames bool
	// Now is the clock for animated scene objects.
	Now func() time.Time
	// Weather is the weather layer and WeatherKind the kind it shows: the
	// scene's, or a movie's overlay.
	Weather     *weather.Layer
	WeatherKind weather.Kind
}

// New enters a map: the scene is loaded and the player dressed.
func New(env *seui.Env, a login.Assets, p Player, body login.RoleView, names map[uint16]string, sceneOf func(uint16) uint16) (*World, error) {
	s, err := LoadScene(a, p.Map)
	if err != nil {
		return nil, err
	}
	w := &World{Env: env, Scene: s, Player: p, Body: body, Now: time.Now}
	if sceneOf != nil {
		w.SceneName = names[sceneOf(p.Map)]
	}
	if body != nil {
		slot := &login.CharacterSlot{Name: p.Name, Level: 1, Body: p.Body, Head: p.Head,
			Color1: p.Color1, Color2: p.Color2, Direction: p.Direction}
		for i, id := range p.Items {
			if i+1 < len(slot.Equipment) {
				slot.Equipment[i+1] = id
			}
		}
		body.SetCharacter(slot)
	}
	return w, nil
}

// Camera is the scene position at the screen's top-left corner.
func (w *World) Camera() (int, int) {
	if w.CameraAt != nil {
		return w.CameraAt.X, w.CameraAt.Y
	}
	return w.Player.X - screenW/2, w.Player.Y - screenH/2
}

// NewView is a view of a map's scene with no player of its own, for a
// movie: its actors are set as NPCs (and Player/Body when the player takes
// part) and CameraAt places it.
func NewView(env *seui.Env, a login.Assets, mapID uint16) (*World, error) {
	s, err := LoadScene(a, mapID)
	if err != nil {
		return nil, err
	}
	return &World{Env: env, Scene: s, Now: time.Now, Cinematic: true, HideNames: true, CameraAt: &image.Point{}}, nil
}

// Draw draws the scene, the player and the texts.
func (w *World) Draw() {
	scr := w.Env.Screen
	cx, cy := w.Camera()
	if !w.HideScene {
		w.Scene.Draw(scr, cx, cy)
	}
	now := w.Now()
	// The weather's under pass (FUN_00334580) follows the ground.
	if w.Weather != nil {
		w.Weather.Under(scr, w.WeatherKind, image.Pt(cx, cy), now)
	}
	objects := w.Scene.Objects
	for i := range objects {
		if objects[i].Kind < ObjectSorted {
			objects[i].Draw(scr, cx, cy, now)
		}
	}
	px, py := w.Player.X-cx, w.Player.Y-cy
	// Characters are drawn back to front by their feet's Y (NPCs plus their
	// record's depth), with kind-3
	// objects at their depth lines after characters on the same row
	// (FUN_0041d13c's list order); the player goes after NPCs on the same
	// row.
	type figure struct {
		y    int
		draw func()
	}
	var figures []figure
	for _, n := range w.NPCs {
		if n.Shown && n.Painter != nil {
			figures = append(figures, figure{n.SortY(), func() {
				w.drawShadow(n, n.X-cx, n.Y-cy)
				if l, ok := n.Painter.(interface{ SetLit(bool) }); ok {
					l.SetLit(n == w.hovered)
				}
				n.paint()
				n.Painter.Draw(scr, n.X-cx, n.Y-cy+n.Info.SpriteDrop(), n.Action)
			}})
		}
	}
	for _, p := range w.Peers {
		if p.Role != nil {
			figures = append(figures, figure{p.Y, func() {
				w.drawSmallShadow(p.X-cx, p.Y-cy)
				p.Role.DrawBody(scr, p.X-cx, p.Y-cy, p.Direction)
			}})
		}
	}
	sort.SliceStable(figures, func(i, j int) bool { return figures[i].y < figures[j].y })
	if w.Body != nil && !w.HidePlayer {
		i := sort.Search(len(figures), func(i int) bool { return figures[i].y > w.Player.Y })
		figures = slices.Insert(figures, i, figure{w.Player.Y, func() {
			w.drawSmallShadow(px, py)
			w.Body.DrawBody(scr, px, py, w.Player.Direction)
		}})
	}
	for i := range objects {
		if o := &objects[i]; o.Kind == ObjectSorted {
			at := sort.Search(len(figures), func(j int) bool { return figures[j].y > o.SortY() })
			figures = slices.Insert(figures, at, figure{o.SortY(), func() { o.Draw(scr, cx, cy, now) }})
		}
	}
	for _, f := range figures {
		f.draw()
	}
	for i := range objects {
		if objects[i].Kind > ObjectSorted {
			objects[i].Draw(scr, cx, cy, now)
		}
	}
	if !w.HideNames {
		w.drawNames(cx, cy, px)
	}
	// The effects list (FUN_00403b8c) follows the map paint.
	if w.lightsFrom.IsZero() {
		w.lightsFrom = now
	}
	w.drawLights(cx, cy, now)
	// Its over pass (FUN_003346e8) follows the effects list.
	if w.Weather != nil {
		w.Weather.Over(scr, w.WeatherKind, image.Pt(cx, cy))
	}
	if !w.Cinematic {
		w.drawMarker(cx, cy, now)
		w.drawLocation()
	}
}

// drawNames draws the NPCs', other players' and the player's names.
func (w *World) drawNames(cx, cy, px int) {
	scr, txt := w.Env.Screen, w.Env.Text
	for _, n := range w.NPCs {
		if n.Shown && n.Painter != nil {
			w.drawName(n, n.X-cx, n.Y-cy)
		}
	}
	for _, p := range w.Peers {
		name := p.Name
		txt.Draw(p.X-cx-len(name)*charW/2, p.Y-cy-peerNameLift, 0, false, true, scr, name, 0, len(name)*charW+charW, 0, peerNameInk, textStyle)
	}
	name := w.Player.Name
	txt.Draw(px-len(name)*charW/2, nameY, 0, false, true, scr, name, 0, len(name)*charW+charW, 0, nameInk, textStyle)
}

// drawLocation draws the name, scene and position line.
func (w *World) drawLocation() {
	scr, txt := w.Env.Screen, w.Env.Text
	loc := []byte(string(w.Player.Name) + " " + w.SceneName + " X:" + strconv.Itoa(w.Player.X) + " Y:" + strconv.Itoa(w.Player.Y))
	txt.Draw(locationRight-len(loc)*charW, locationY, 0, false, true, scr, loc, 0, len(loc)*charW+charW, 0, locationInk, textStyle)
}
