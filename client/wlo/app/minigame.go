package app

import (
	"encoding/binary"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// Minigames (TSportManage, PTR_DAT_004c9994). The server starts one with
// 57/1 (FUN_003bd398): the game type, a 16-bit parameter and a byte. The
// HUD is hidden, the game is created, and the start form (CH_GameStartForm,
// PTR_DAT_004ca720) explains it with Start and Leave. Each frame the main
// loop updates the game (FUN_003be358) and draws it after the map
// (FUN_003bdfc8); ground clicks go to the game (slot +4, FUN_003bd32c).
// The game's result goes back with 57/1 and the event step is marked done
// (FUN_003bdfa4); 57/2 (FUN_003bc120) brings the HUD back, frees the game
// and restores the map's music.
//
// Local games share the Start/Leave lifecycle. Native arcade games that
// require additional server protocol are listed in docs/MINIGAMES.md.
const (
	sportType      = 1 // offsets after the command byte
	sportParam     = 2 // u16
	sportByte      = 4
	sportStartSize = 5

	SportMole   = 3
	SportHunter = 4
	SportSheep  = 5
	SportLucky  = 13
	SportMemory = 15
)

// sportGame is a ported game as the manager drives it: the frame update
// (FUN_003be358), the draw (FUN_003bdfc8), the ground click (slot +4),
// Start, and Exit or Leave.
type sportGame interface {
	Pictures() []string
	Update(now time.Time)
	Draw(dst *surface.Surface, mx, my int, now time.Time)
	Click(x, y int, now time.Time)
	Begin(now time.Time)
	End(win bool)
	Running() bool
	Done() bool
	Cursor() cursor.Shape
}

// moleGame adapts minigame.Mole.
type moleGame struct{ *minigame.Mole }

func (g moleGame) Update(now time.Time)                               { g.Mole.Update(now) }
func (g moleGame) Draw(dst *surface.Surface, _, _ int, now time.Time) { g.Mole.Draw(dst, now) }
func (g moleGame) Begin(time.Time)                                    { g.Mole.Start() }
func (g moleGame) Running() bool                                      { return g.Started }
func (g moleGame) Cursor() cursor.Shape                               { return g.Mole.Cursor }

// hunterGame adapts minigame.Hunter; the sight follows the pointer and the
// cursor stays the normal one.
type hunterGame struct{ *minigame.Hunter }

func (g hunterGame) Update(now time.Time) { g.Step(now) }
func (g hunterGame) Draw(dst *surface.Surface, mx, my int, now time.Time) {
	g.Aim(mx, my)
	g.Hunter.Draw(dst, mx, my, now)
}
func (g hunterGame) Click(x, y int, _ time.Time) { g.Hunter.Click(x, y) }
func (g hunterGame) Begin(now time.Time)         { g.Start(now) }
func (g hunterGame) Running() bool               { return g.Started }
func (g hunterGame) Cursor() cursor.Shape        { return cursor.ShapeNormal }

// sportPlay is a running minigame.
type sportPlay struct {
	kind  byte
	game  sportGame
	start *sportStartForm
	exit  *sportExitForm
	hud   []seui.Control // the forms hidden for the game
}

// sportStartForm is CH_GameStartForm (constructor FUN_001b4d0c) laid out by
// FUN_001b5740 for the game, painted with its extra picture (FUN_001b6ab4).
type sportStartForm struct {
	seui.Form
	Start, Leave, Close *seui.FixedButton
	extra               int // a picture drawn on the form, -1 for none
	extraX, extraY      int
}

// Start form tags (the buttons' +0xb0, handled at 0x1b4f20).
const (
	sportTagStart = 1
	sportTagLeave = 2
	sportTagClose = 3
)

// sportLayout is one game's start form (FUN_001b5740).
type sportLayout struct {
	form                 string
	left, height, width  int
	startLeft, leaveLeft int
	buttonsTop           int
	closeLeft            int
	extra                string
	extraX, extraY       int
}

const (
	sportFormTop   = 0x3c
	sportButtonH   = 0x14
	sportButtonW   = 0x38
	sportCloseSide = 0x12
	sportCloseTop  = 0x1a
	// The games' Exit buttons (CH_GameExplain's button, FUN_001b20c0).
	moleExitButton   = "HitMouse_Exit_Btn"
	moleExitLeft     = 700
	moleExitTop      = 500
	hunterExitButton = "Hunting_Exit_Btn"
	hunterExitLeft   = 700
	hunterExitTop    = 0x28
	exitButtonRows   = 3
)

// moleLayouts are FUN_001b5740 case 3 per variant, with HitMouse_40s at
// (0x3c, 0x196) or (0x3e, 0x19e) (FUN_001b6ab4).
var moleLayouts = [...]sportLayout{
	minigame.VariantMouse:  {"HitMouse_Exp_Form", 0xf5, 0x1e3, 0x14b, 0x60, 0xaf, 0x1bd, 0x119, "HitMouse_40s", 0x3c, 0x196},
	minigame.VariantRabbit: {"HitRabit_Exp_Form", 0xf5, 0x1e3, 0x14b, 0x60, 0xaf, 0x1bd, 0x119, "HitMouse_40s", 0x3e, 0x19e},
	minigame.VariantTurtle: {"HitTurtle_Exp_Form", 0xf5, 0x1e3, 0x14b, 0x60, 0xaf, 0x1bd, 0x119, "HitMouse_40s", 0x3e, 0x19e},
}

// hunterLayout is FUN_001b5740 case 4; its paint adds nothing
// (FUN_001b6f24).
var sheepLayout = sportLayout{"Sheep_Exp_Form", 0xd7, 0x1fb, 0x199, 0x90, 0xd1, 0x1d1, 0x167, "", 0, 0}
var luckyLayout = sportLayout{"MiniGame_Lucky_Form", 0xd7, 0x20d, 0x199, 0x5f, 0xd2, 0x1ae, 0x127, "", 0, 0}
var memoryLayout = sportLayout{"MemGame_Exp", 0xeb, 0x20d, 0x199, 0x6b, 0xd8, 0x1b8, 0x167, "", 0, 0}

var hunterLayout = sportLayout{"Hunt_Exp_Form", 0xeb, 0x1e3, 0x159, 0x6b, 0xb8, 0x1b8, 0x127, "", 0, 0}

func newSportStartForm(env *seui.Env, l sportLayout, click func(tag int)) *sportStartForm {
	f := &sportStartForm{extra: -1}
	f.InitForm(f, env)
	f.Name = "CH_GameStartForm"
	f.Init(l.form, l.left, l.height, l.width, 0, 0, false, l.height, l.width, sportFormTop)
	button := func(name string, left, h, w, top, tag int, hint string) *seui.FixedButton {
		b := seui.NewFixedButton(env, f)
		b.Init(name, left, h, w, 0, 0, false, h, w, top)
		b.Tag = tag
		b.OnClickTag = click
		b.SetHint([]byte(hint))
		return b
	}
	f.Start = button("btn_Start_1", l.startLeft, sportButtonH, sportButtonW, l.buttonsTop, sportTagStart, "Start")
	f.Leave = button("btn_Leave_1", l.leaveLeft, sportButtonH, sportButtonW, l.buttonsTop, sportTagLeave, "Leave")
	closeName := "btn_close_s_1"
	if l.form == "MemGame_Exp" {
		closeName = "btn_close_s_3"
	}
	f.Close = button(closeName, l.closeLeft, sportCloseSide, sportCloseSide, sportCloseTop, sportTagClose, "Leave")
	if l.extra != "" {
		f.extra, f.extraX, f.extraY = env.Pics.Find(l.extra), l.extraX, l.extraY
	}
	return f
}

// Paint is FUN_001b6ab4: the form, then the game's picture on it.
func (f *sportStartForm) Paint() {
	f.Form.Paint()
	if f.extra >= 0 {
		a := f.Abs()
		f.Env.Pics.Draw(f.Env.Screen, f.extra, a.X+f.extraX, a.Y+f.extraY, true)
	}
}

// sportExitForm is CH_GameExplain (FUN_001b1fb8, PTR_DAT_004ca64c): a bare
// form whose button +0x130 is the game's Exit (FUN_001b20c0).
type sportExitForm struct {
	seui.Form
	Exit *seui.Button
}

func newSportExitForm(env *seui.Env, name string, left, top int, click func()) *sportExitForm {
	f := &sportExitForm{}
	f.InitForm(f, env)
	f.Name = "CH_GameExplain"
	f.Draggable = false
	f.Exit = seui.NewButton(env, f)
	if i := env.Pics.Find(name); i >= 0 {
		w, h := env.Pics.Size(i)
		f.Exit.Init(name, left, h/exitButtonRows, w, 0, 0, true, h/exitButtonRows, w, top)
	}
	f.Exit.OnClick = click
	return f
}

// startSport is 57/1 (FUN_003bd398).
func (c *Client) startSport(s []byte) {
	if len(s) < sportStartSize || c.sport != nil {
		return
	}
	kind := s[sportType]
	param := int(binary.LittleEndian.Uint16(s[sportParam:]))
	now := c.Now()
	sp := &sportPlay{kind: kind}
	result := func(win bool) {
		if c.sport != sp {
			return
		}
		sp.exit.Hide()
		c.sendSportResult(win)
	}
	var layout sportLayout
	var exit string
	var exitLeft, exitTop int
	switch kind {
	case minigame.ScotdKind:
		g := minigame.NewScotd(c.Pics, uint16(param), s[sportByte])
		g.NewMonster = c.newMonster
		g.Sound, g.Music, g.Result = c.playSound, c.playMusic, result
		sp.game = g
		layout = sportLayout{"MiniGame_Explan_Form", 215, 525, 409, 144, 209, 488, 359, "", 0, 0}
		exit, exitLeft, exitTop = "MiniGame_Exit_Btn", 720, 520
	case minigame.PumpkinKind:
		g := minigame.NewPumpkin(c.Pics)
		g.NewMonster = c.newMonster
		if c.World != nil && c.World.Body != nil {
			g.DrawPlayer = c.World.Body.DrawBody
		}
		g.Sound, g.Music, g.Result = c.playSound, c.playMusic, result
		sp.game = g
		layout = sportLayout{"MiniGame_Pumpkin_Form", 215, 525, 409, 95, 210, 430, 295, "", 0, 0}
		exit, exitLeft, exitTop = "MiniGame_Exit_Btn", 720, 520
	case SportMole:
		m := minigame.NewMole(c.Pics, param, now)
		m.Rand, m.Sound, m.Music, m.Result = rand.IntN, c.playSound, c.playMusic, result
		sp.game = moleGame{m}
		layout, exit, exitLeft, exitTop = moleLayouts[m.Variant], moleExitButton, moleExitLeft, moleExitTop
	case SportHunter:
		h := minigame.NewHunter(c.Pics, now)
		h.Rand, h.Sound, h.Music, h.Result = rand.IntN, c.playSound, c.playMusic, result
		h.NewMonster = c.newMonster
		if c.World != nil {
			h.Body = c.World.Player.Body
		}
		sp.game = hunterGame{h}
		layout, exit, exitLeft, exitTop = hunterLayout, hunterExitButton, hunterExitLeft, hunterExitTop
	case SportSheep:
		g := minigame.NewSheep(c.Pics, s[sportByte])
		g.NewMonster = c.newMonster
		g.Rand, g.Sound, g.Music, g.Result = rand.IntN, c.playSound, c.playMusic, result
		sp.game = g
		layout = sheepLayout
		if g.Variant != 1 {
			layout.form = "Pig_Exp_Form"
		}
		exit, exitLeft, exitTop = "MiniGame_Exit_Btn", 720, 520
	case SportLucky:
		g := minigame.NewLucky(c.Pics)
		if c.World != nil && c.World.Body != nil {
			g.DrawPlayer = c.World.Body.DrawBody
		}
		g.Rand, g.Sound, g.Music, g.Result = rand.IntN, c.playSound, c.playMusic, result
		sp.game, layout = g, luckyLayout
		exit, exitLeft, exitTop = "MiniGame_Exit_Btn", 720, 520
	case minigame.ArcadeEgg, minigame.ArcadeEgg2, minigame.ArcadeSlots, minigame.ArcadeSlots2, minigame.ArcadeSlots3:
		g := minigame.NewArcade(c.Pics, kind)
		g.Sound, g.Music, g.Result = c.playSound, c.playMusic, result
		g.Send = func(p []byte) {
			if c.sport == sp {
				c.Net.Send(p)
			}
		}
		for _, id := range g.Prizes() {
			if item, ok := c.items[id]; ok {
				c.loadPictures(strconv.Itoa(int(item.Icon)))
			}
		}
		g.Item = func(dst *surface.Surface, id uint16, x, y int) string {
			item, ok := c.items[id]
			if !ok {
				return ""
			}
			c.Pics.Draw(dst, c.Pics.Find(strconv.Itoa(int(item.Icon))), x, y, true)
			return item.Definition.Name
		}
		sp.game = g
		layout = sportLayout{"Slotmach_Exp_Form", 215, 525, 409, 119, 234, 458, 359, "", 0, 0}
		if kind == minigame.ArcadeEgg || kind == minigame.ArcadeEgg2 {
			layout.form = "TrunEgg_Exp_Form"
		}
		exit, exitLeft, exitTop = "MiniGame_Exit_Btn", 720, 520
	case SportMemory:
		g := minigame.NewMemory(c.Pics)
		g.Rand, g.Sound, g.Music, g.Result = rand.IntN, c.playSound, c.playMusic, result
		sp.game, layout = g, memoryLayout
		exit, exitLeft, exitTop = "Hunting_Exit_Btn", 700, 560
	default:
		log.Printf("minigame %d (parameter %d) is not ported; answering a loss", kind, param)
		c.sendSportResult(false)
		return
	}
	c.loadPictures(sp.game.Pictures()...)
	c.loadPictures(layout.form, layout.extra, exit, "btn_Start_1", "btn_Leave_1", "btn_close_s_1", "btn_close_s_3")
	sp.start = newSportStartForm(c.Env, layout, func(tag int) { c.sportStartClick(sp, tag) })
	sp.exit = newSportExitForm(c.Env, exit, exitLeft, exitTop, func() { sp.game.End(false) })
	c.UI.Add(sp.start)
	c.UI.Add(sp.exit)
	c.hideHUD(sp)
	if c.World != nil {
		c.World.StopWalk()
	}
	c.groundHeld, c.groundSince = false, time.Time{}
	c.pendingNPC = nil
	sp.start.Show()
	c.sport = sp
}

// newMonster is a hunted monster's role from its NPC template, drawn with
// its ground shadow.
func (c *Client) newMonster(id uint16) minigame.Monster {
	if c.lib == nil {
		return nil
	}
	if c.npcTemplates == nil {
		c.npcTemplates, _ = world.NPCTemplates(c.Assets)
	}
	t, ok := c.npcTemplates[uint32(id)]
	if !ok {
		return nil
	}
	return &monsterRole{NPC: role.NewNPC(c.lib, t.Look, t.Colors), pics: c.Pics, shadow: t.Shadow}
}

// monsterRole draws a role's shadow under its sprite (FUN_0030120c).
type monsterRole struct {
	*role.NPC
	pics   *picdb.DB
	shadow byte
}

func (m *monsterRole) Draw(dst *surface.Surface, x, y, action int) {
	world.DrawShadow(dst, m.pics, m.shadow, x, y)
	m.NPC.Draw(dst, x, y, action)
}

// sportStartClick is the start form's buttons (0x1b4f20): Start runs the
// game and shows its Exit; Leave and the close button give up (a loss).
func (c *Client) sportStartClick(sp *sportPlay, tag int) {
	if c.sport != sp || sp.game.Done() {
		return
	}
	sp.start.Hide()
	switch tag {
	case sportTagStart:
		sp.game.Begin(c.Now())
		sp.exit.Show()
	default:
		sp.game.End(false)
	}
}

// sendSportResult is FUN_003bdfa4: 57/1 with the result, and the event
// step is done.
func (c *Client) sendSportResult(win bool) {
	r := byte(0)
	if win {
		r = 1
	}
	c.Net.Send([]byte{protocol.CommandMinigame, protocol.MinigameResult, r})
	c.event.done = true
}

// endSport is 57/2 (FUN_003bc120).
func (c *Client) endSport() {
	sp := c.sport
	if sp == nil {
		return
	}
	c.sport = nil
	sp.start.Hide()
	sp.exit.Hide()
	c.UI.Remove(sp.start)
	c.UI.Remove(sp.exit)
	for _, f := range sp.hud {
		if s, ok := f.(interface{ Show() }); ok {
			s.Show()
		}
	}
	c.playMapMusic()
}

// hideHUD hides the HUD's forms for the game (FUN_003bd398's slot +0x24
// calls) and remembers them.
func (c *Client) hideHUD(sp *sportPlay) {
	if c.Inventory != nil && c.Inventory.Dialog != nil {
		c.Inventory.Dialog.Hide()
	}
	for _, f := range []seui.Control{c.MainStatus, c.FuncButtons, c.MainButtons, c.HotKeys, c.ChatBar, c.Chat, c.Inventory} {
		if f == nil || !f.Base().Visible {
			continue
		}
		if h, ok := f.(interface{ Hide() }); ok {
			h.Hide()
			sp.hud = append(sp.hud, f)
		}
	}
}

// sportFrame updates and draws the game after the map.
func (c *Client) sportFrame() {
	sp := c.sport
	if sp == nil {
		return
	}
	now := c.Now()
	if g, ok := sp.game.(interface{ Aim(int, int) }); ok {
		g.Aim(c.Input.X, c.Input.Y)
	}
	sp.game.Update(now)
	sp.game.Draw(c.Screen, c.Input.X, c.Input.Y, now)
}

// sportClick passes a ground click to the game; it reports whether a game
// runs (and so took the click).
func (c *Client) sportClick(x, y int) bool {
	if c.sport == nil {
		return false
	}
	c.sport.game.Click(x, y, c.Now())
	return true
}

// sportCursor is the game's cursor state (FUN_003ba9d8), 0 for none.
func (c *Client) sportCursor() cursor.Shape {
	if c.sport == nil || !c.sport.game.Running() || c.sport.game.Done() {
		return 0
	}
	return c.sport.game.Cursor()
}

// sportArchives are the picture archives a game's pictures come from: the
// image archives, and map.JMG for the full-screen backgrounds.
var sportArchives = append(append([]string{"Jpeg", "map", "map_c01", "map_d01"}, pictureArchives...),
	"item", "item1", "item2", "item3", "item_c01", "item_c02", "item1_d01", "item1_d02", "item1_d03", "item1_d04", "item2_c01", "item3_c01")

// loadPictures registers archive pictures in the picture database (the
// original's database holds every archive; the menu skins are loaded at
// startup).
func (c *Client) loadPictures(names ...string) {
	for _, n := range names {
		if n == "" || c.Pics.Find(n) >= 0 {
			continue
		}
		for _, arc := range sportArchives {
			if compiled, err := c.Assets.CompiledPicture(arc, n); err == nil {
				if err = c.Pics.AddCompiled(n, compiled); err == nil {
					break
				}
			}
			if m, err := c.Assets.LoadPicture(arc, n); err == nil {
				c.Pics.Add(n, m)
				break
			}
		}
	}
}

// playMusic plays a music file.
func (c *Client) playMusic(path string) {
	if c.Music != nil {
		c.Music.Play(path)
	}
}

// playSound plays a sound file once.
func (c *Client) playSound(path string) {
	if c.Env.Sound != nil {
		c.Env.Sound(path)
	}
}

// SportKey gives the running native game first use of its controls. Other keys
// remain available to the explanation/exit forms and text input.
func (c *Client) SportKey(key int, down bool) bool {
	if c.sport == nil || !c.sport.game.Running() || c.sport.game.Done() {
		return false
	}
	if g, ok := c.sport.game.(interface {
		Key(int, bool, time.Time) bool
	}); ok {
		return g.Key(key, down, c.Now())
	}
	return false
}
