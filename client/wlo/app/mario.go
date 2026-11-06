package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

// The fruit machines' explanation form (FUN_001b5740 cases 0xb, 0xc and
// 0xe) and its price, painted in blue at (0x186, 0x77) by FUN_001b7004,
// FUN_001b7084 and FUN_001b7104.
var marioLayout = sportLayout{"Mario_Exp_Form", 0xd7, 0x20d, 0x199, 0x77, 0xea, 0x1db, 0x167, "", 0, 0}

const (
	marioPriceX     = 0x186
	marioPriceY     = 0x77
	marioPriceLine  = 0xf
	marioPriceWidth = 0x18
	marioPriceInk   = 0x1f
	// The machine's form (FUN_001450d4, FUN_00145408, FUN_00145758).
	marioExitButton = "Hunting_Exit_Btn"
	marioExitLeft   = 700
	marioExitTop    = 0x23d
	marioStartName  = "Mario_Start"
	marioStartLeft  = 0x2ab
	marioStartTop   = 0x199
	marioStartSide  = 0x54
)

// newMario creates a fruit machine (57/1 kinds 11, 12 and 14) with its
// prompts, its balance query and its form: Leave, the machine's Start
// button and the prizes' hint buttons.
func (c *Client) newMario(sp *sportPlay, kind byte, result func(bool)) *minigame.Mario {
	g := minigame.NewMario(c.Pics, kind, c.Now())
	g.Sound, g.Music, g.Result = c.playSound, c.playMusic, result
	g.Send = func(p []byte) {
		if c.sport == sp {
			c.Net.Send(p)
		}
	}
	g.Notice = func(text string, d time.Duration) { c.Notices.Show([]byte(text), d, c.Now()) }
	g.Confirm = func(prompt string, ok func()) {
		c.sportAsk(sp, prompt, nil, func(row int) {
			if row == 0 {
				ok()
			}
		})
	}
	g.Choose = func(prompt string, options []string, pick func(int)) { c.sportAsk(sp, prompt, options, pick) }
	g.HasRoom = func() bool {
		if c.InventoryState == nil {
			return true
		}
		for _, it := range c.InventoryState.Bag {
			if it.Empty() {
				return true
			}
		}
		return false
	}
	g.Tokens = func(id uint16) int {
		n := 0
		if c.InventoryState != nil {
			for _, it := range c.InventoryState.Bag {
				if it.ID == id && !it.Empty() {
					n += int(it.Count)
				}
			}
		}
		return n
	}
	for _, id := range g.Prizes() {
		if item, ok := c.items[id]; ok {
			c.loadPictures(strconv.Itoa(int(item.Icon)))
		}
	}
	g.Item = func(dst *surface.Surface, id uint16, x, y int) {
		if item, ok := c.items[id]; ok {
			c.Pics.Draw(dst, c.Pics.Find(strconv.Itoa(int(item.Icon))), x, y, true)
		}
	}
	g.Name = func(id uint16) []byte {
		if item, ok := c.items[id]; ok {
			return clientassets.Big5Text(item.Definition.Name)
		}
		return nil
	}
	g.Text = func(dst *surface.Surface, x, y int, s []byte, lineH, width int, paper, ink uint16, style byte) {
		if c.Env.Text != nil {
			c.Env.Text.Draw(x, y, 0, false, true, dst, s, lineH, width, paper, ink, style)
		}
	}
	return g
}

// marioForm adds the machine's Start button and the prize hints to the
// form that holds its Leave button.
func (c *Client) marioForm(sp *sportPlay, g *minigame.Mario) {
	f := sp.exit
	f.Exit.OnClick = g.Exit
	start := seui.NewFixedButton(c.Env, f)
	start.Init(marioStartName, marioStartLeft, marioStartSide, marioStartSide, 0, 0, true, marioStartSide, marioStartSide, marioStartTop)
	start.BindLanguage("hint", func() { start.SetHint([]byte(localization.UI("ui.app.start_e4bb9f1e"))) })
	start.OnClick = func() {
		if sp.talk == nil || !sp.talk.Shown() {
			g.Press()
		}
	}
	for i, id := range g.Prizes()[1:] {
		item, ok := c.items[id]
		if !ok {
			continue
		}
		w, h := c.Pics.Size(c.Pics.Find(strconv.Itoa(int(item.Icon))))
		b := seui.NewFixedButton(c.Env, f)
		at := minigame.MarioPrizePlaces[i]
		b.Init(strconv.Itoa(int(id)), at.X, h, w, 0, 0, false, h, w, at.Y+minigame.MarioHintDrop)
		// Mario 1's hints are the descriptions (record +0x96), the others'
		// the names (+4).
		hint := item.Definition.Name
		if g.Kind == minigame.MarioKind {
			hint = item.Description
		}
		b.SetHint(clientassets.Big5Text(hint))
	}
}

// sportAsk shows the sport prompt (the talk window PTR_DAT_004c9ce4):
// OK and Cancel without options (FUN_0034d150, OK is row 0), else the
// option list (FUN_0034d00c).
func (c *Client) sportAsk(sp *sportPlay, prompt string, options []string, pick func(int)) {
	if sp.talk == nil {
		sp.talk = hud.NewTalk(c.Env)
		sp.talk.Now = c.Now
		sp.talk.Sound = c.playSound
	}
	rows := [][]byte{nil, nil}
	if options != nil {
		rows = rows[:0]
		for _, o := range options {
			rows = append(rows, []byte(o))
		}
	}
	sp.answer = pick
	sp.talk.Ask([][]byte{[]byte(prompt)}, hud.Speaker{}, nil, rows, options == nil)
}

// sportAnswer is a click while the sport prompt shows; it reports whether
// the prompt took it.
func (c *Client) sportAnswer(sp *sportPlay, x, y int) bool {
	if sp.talk == nil || !sp.talk.Shown() {
		return false
	}
	row, cancel := sp.talk.Pick(x, y)
	if row < 0 && !cancel {
		return true
	}
	sp.talk.Hide()
	pick := sp.answer
	sp.answer = nil
	if pick != nil && !cancel {
		pick(row)
	}
	return true
}

// sportBalance is 35/4 (0x2eaded) while a fruit machine runs: the game's
// +0xac becomes the IM points plus the bonus points.
func (c *Client) sportBalance(s []byte) {
	if c.sport == nil || len(s) < 9 {
		return
	}
	if g, ok := c.sport.game.(*minigame.Mario); ok {
		g.Points = int(int32(binary.LittleEndian.Uint32(s[1:]))) + int(int32(binary.LittleEndian.Uint32(s[5:])))
	}
}
