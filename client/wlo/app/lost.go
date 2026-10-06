package app

import (
	"image"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

// LostForm is the message form PTR_DAT_004ca12c (constructor FUN_003a5184):
// panel15, 200 × 150 at (300, 0x50), with a text field (+0x140) and four
// btn_module_1 buttons. A lost connection shows it with Leave and Prev.
type LostForm struct {
	seui.Form
	Text         *seui.Editor // +0x140 editorBG
	Leave        *seui.Button // +0x130, closes the client (0x3a54f0)
	Prev         *seui.Button // +0x134, back to the server list (0x3a547c)
	Finish       *seui.Button // +0x138, hidden here
	UpdateButton *seui.Button // +0x13c "Update", hidden here
}

func newLostForm(env *seui.Env) *LostForm {
	f := &LostForm{}
	f.InitForm(f, env)
	f.Name = "TRe_MsgForm"
	f.Init("panel15", 300, 0x32, 0x32, 0, 0, true, 0x96, 200, 0x50)
	f.SetMargins(0xf, 0xf, 0xf, 0xf)
	f.Text = seui.NewEditor(env, f)
	f.Text.Init("editorBG", 10, 0x14, 0x32, 0, 0, true, 0x16, 0xb4, 0x19)
	f.Text.SetColor(0xffff)
	f.Text.Color2 = 0
	button := func(left int, caption string) *seui.Button {
		b := seui.NewButton(env, f)
		b.Init("btn_module_1", left, 0x14, 0x38, 0, 0, true, 0x14, 0x38, 0x6e)
		b.SetColor(0xffff)
		b.SetCaption([]byte(caption))
		return b
	}
	f.Prev = button(0x66, "Prev")
	f.UpdateButton = button(0x66, "Update")
	f.Leave = button(0x20, "Leave")
	f.Finish = button(0x3c, "Finish")
	f.Finish.SetVisible(false)
	f.UpdateButton.SetVisible(false)
	return f
}

// The lost connection's screen transition (FUN_0049a2e8 with mode 1, run
// by FUN_0049a484 each game frame): black at alpha 12 × step over the
// scene for steps 1..15; at 15 the darkened frame is kept (FUN_0049ba08)
// and the scene is no longer drawn (+0x565), while the forms still are.
const (
	fadeSteps     = 15
	fadeAlphaStep = 12
	fadeFrame     = 30 * time.Millisecond
)

type fadeState struct {
	step   int // +0x531, 0 when no transition runs
	at     time.Time
	frozen *surface.Surface // DAT_00828160
}

// drawFade runs the transition over the scene just drawn.
func (c *Client) drawFade(now time.Time) {
	f := &c.fade
	if f.frozen != nil {
		c.Screen.Draw(0, 0, f.frozen, false)
		return
	}
	if f.step == 0 {
		return
	}
	for now.Sub(f.at) >= fadeFrame && f.step < fadeSteps {
		f.at = f.at.Add(fadeFrame)
		f.step++
	}
	c.Screen.FillAlpha(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0, f.step*fadeAlphaStep)
	if f.step == fadeSteps {
		f.frozen = c.Screen.Clone()
		f.step = 0
	}
}

// sceneFrozen reports whether the scene stopped (a lost connection).
func (c *Client) sceneFrozen() bool { return c.fade.step > 0 || c.fade.frozen != nil }

// disconnected is ClientSocket1Disconnect (0x49871c). With a login form
// open or in the game, the login forms close, the message form shows the
// reason ("Connection lost" unless the server left one) with Leave and
// Prev, and the screen darkens and freezes behind it.
func (c *Client) disconnected() {
	c.Login.LoginTime = time.Time{}
	c.hotbar.drag = nil
	c.closeHotbarTarget()
	if !(c.Login.Visible || c.Chars.Visible || c.Create.Visible || c.Password.Visible || c.G.InGame) {
		return
	}
	if c.Skills != nil {
		c.Skills.Hide()
	}
	if c.Settings != nil {
		c.Settings.Hide()
	}
	if c.Inventory != nil {
		c.Inventory.Hide()
		c.Inventory.ResetRemote()
	}
	c.Login.Hide()
	c.Chars.Hide()
	c.Create.Hide()
	c.Password.Hide()
	text := c.disconnectText
	if len(text) == 0 {
		text = noticeConnection
	}
	c.disconnectText = nil
	c.Lost.Text.SetText(text)
	c.Lost.Show()
	c.Lost.Prev.Show()
	c.Lost.Leave.Show()
	if c.fade.frozen != nil {
		c.fade.frozen.Close()
	}
	c.fade = fadeState{step: 1, at: c.Now()}
}

// lostPrev is Prev (0x3a547c): the form closes, the game state resets
// (FUN_00314bf4) and the server list returns.
func (c *Client) lostPrev() {
	c.Lost.Hide()
	if c.fade.frozen != nil {
		c.fade.frozen.Close()
	}
	c.fade = fadeState{}
	c.G.InGame = false
	c.World = nil
	c.movie = nil
	c.players = nil
	c.stopAmbience()
	c.Chat.Clear()
	c.UI.HideAll()
	c.Servers.Show()
	if c.Music != nil {
		c.Music.Play(loginMusic) // back in the login phase: CheckStartMusic
	}
}
