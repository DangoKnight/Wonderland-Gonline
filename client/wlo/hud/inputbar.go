package hud

import (
	"image"
	"strconv"

	"wonderland-go/client/wlo/seui"
)

// InputBar is TSe_InputBar (constructor FUN_0026348c, paint FUN_00268194):
// the chat bar along the bottom: the channel button, the whisper name and
// message fields, the recent whisperers, and the emote button. Not ported
// yet: the mail animation,
// the battle buttons (Atk, Skill, Def, Catch, Flee, Help) and the viewer
// and auto-play buttons.
type InputBar struct {
	seui.FixedForm
	Channel    byte              // +0x140, 2 (Local) at start
	Switch     *seui.FixedButton // +0x1ac, the current channel's btn_channel_<n>_1
	Whisper    *seui.Editor      // +0x138, the whisper target
	Message    *seui.Editor      // +0x134, the message
	Emotes     *seui.FixedButton // +0x1ec Btn_expression_1
	EmotePanel *seui.Panel
	EmoteClose *seui.FixedButton
	EmoteIcons [chatEmoticons]*seui.FixedButton
	// Target is the whisper target's ID (+0x18c), 0 for none.
	Target uint32
	// The channel list (FUN_0026527c): a frame above the button with a
	// button for each other channel, shown while the pointer is over the
	// button or the list (FUN_00268194).
	Frame  *seui.Panel                             // +0x1e8 icon_Channelframe_1
	Others [channelListSlots + 1]*seui.FixedButton // +0x1b4..
	others [channelListSlots + 1]byte              // +0x1d0.. their channels
	// The recent whisperers (+0x174 in the panel +0x178): shown on the
	// Whisper channel while the pointer is over the whisper field or the
	// list (FUN_00268194); a pick fills the field and takes the name as
	// the target (FUN_00268c38 → OnWhisperName).
	Whisperers    *seui.SelectText
	WhispererBox  *seui.Panel
	OnWhisperName func()
	// Pointer is the pointer's screen position.
	Pointer func() (int, int)
	// InTeam and InGuild are the player's memberships (+0x1eff, the guild
	// object's +0xc); Notice adds a channel-10 line to the chat log; Focus
	// gives a control the keyboard.
	InTeam, InGuild func() bool
	Notice          func(text string)
	Focus           func(seui.Control)

	history  [][]byte // +0x144, the last ten messages sent
	recalled int      // +0x14c
	channels [inputChannels + 1]int
	mail     int
}

// Input bar channels (+0x140); each sends its own 2/x (FUN_002641c4).
const (
	InputWorld   = 1
	InputLocal   = 2
	InputWhisper = 3
	InputTeam    = 4
	InputGuild   = 5
	InputAlly    = 6
	InputGM      = 7
)

const (
	inputChannels     = 5
	channelListSlots  = 4
	inputWidth        = 0x209
	inputHeight       = 0x19
	inputTop          = 0x23f
	inputChannelX     = 6
	inputChannelY     = 2
	inputMailX        = 4
	inputMailY        = 2
	inputMailW        = 0x17
	inputMailH        = 0x15
	inputSwitchLeft   = 0x1f
	inputSwitchTop    = 2
	inputSwitchW      = 0x28
	inputSwitchH      = 0x15
	inputSwitchHover  = 0x1c // the hover rectangles start 3 pixels left of the button
	inputListHeight   = 0x7f
	inputFrameLeft    = 0x1c
	inputFrameW       = 0x2e
	inputFrameH       = 0x57
	inputFrameClipH   = 0x43
	inputListStep     = 0x14
	inputListTop      = 3
	inputWhisperLeft  = 0x49
	inputWhisperWidth = 0x58
	inputWhisperMax   = 0xf
	inputMessageLeft  = 0xa2
	inputMessageWidth = 0x148
	inputMessageMax   = 0x3c
	inputFieldTop     = 2
	inputFieldHeight  = 0x14
	inputFieldMargin  = 5
	inputEmoteLeft    = 0x1ec
	inputTextInk      = 0xffff
	inputHistory      = 10
	whisperersMax     = 10
	whispererRow      = 0x14
	whispererPad      = 0xc
	whispererBoxLeft  = 0x18
	whispererBoxW     = 0x8c
	whispererListLeft = 0x19
	whispererListTop  = 8
	whisperFieldHover = 0x46 // the hover rectangle: 0x56 × 0x16 from here, a pixel below the bar's top
	whisperFieldW     = 0x56
	whisperFieldH     = 0x16
	channelPicture    = "btn_channel_"
	channelPictureEnd = "_1"
	channelFrame      = "icon_Channelframe_1"
	// Team and Guild refuse without a team or guild (FUN_00267db0).
	notInTeam  = "(System):You are not in a Team"
	notInGuild = "(System):You are not in a Guild"
)

// NewInputBar is FUN_0026348c's layout.
func NewInputBar(env *seui.Env) *InputBar {
	b := &InputBar{Channel: InputLocal, mail: env.Pics.Find("Anim_GotMailS_1")}
	b.InitFixedForm(b, env)
	b.Name = "TSe_InputBar"
	b.Draggable = false
	b.Init("main_input_1", 0, 0, 0, 0, 0, false, inputHeight, inputWidth, inputTop)
	for i := 1; i <= inputChannels; i++ {
		b.channels[i] = env.Pics.Find("btn_channel_" + strconv.Itoa(i))
	}
	b.Switch = seui.NewFixedButton(env, b)
	b.Switch.Init(channelButton(InputLocal), inputSwitchLeft, inputSwitchH, inputSwitchW, 0, 0, true, inputSwitchH, inputSwitchW, inputSwitchTop)
	b.Switch.SetHint([]byte("Switch Channel"))
	b.Switch.OnClick = b.nextChannel
	b.Switch.OnUp = b.toggleList
	for i := 1; i <= channelListSlots; i++ {
		o := seui.NewFixedButton(env, b)
		o.Init("", inputSwitchLeft, inputSwitchH, inputSwitchW, 0, 0, true, inputSwitchH, inputSwitchW, -inputListStep*i-inputListTop)
		o.Tag = i
		o.OnClickTag = b.pickChannel
		o.SetVisible(false)
		b.Others[i] = o
	}
	// Created after its buttons, the frame is drawn under them.
	b.Frame = seui.NewPanel(env, b)
	b.Frame.Init(channelFrame, inputFrameLeft, inputFrameClipH, inputFrameW, 0, 0, true, inputFrameH, inputFrameW, -inputFrameH)
	b.Frame.Hover = false
	b.Frame.SetVisible(false)
	// The whisper field is read-only outside the Whisper channel.
	b.Whisper = newField(env, b, inputWhisperLeft, inputWhisperWidth, inputWhisperMax, "")
	b.Whisper.ReadOnly = true
	b.Message = newField(env, b, inputMessageLeft, inputMessageWidth, inputMessageMax, "Press Ctrl+V to paste")
	b.Message.AllowPaste = true
	// The constructor clears the message field's pixel hit test (+0xa8);
	// it has no picture, so it is clicked by its rectangle. The whisper
	// field keeps it and takes the keyboard when Whisper is selected.
	b.Message.PixelHit = false
	b.Message.OnUp, b.Message.OnDown = b.recallOlder, b.recallNewer
	b.Emotes = newBarButton(env, b, "Btn_expression_1", inputEmoteLeft, 0, "Chat Emotes")
	b.WhispererBox = seui.NewPanel(env, b)
	b.WhispererBox.Init("", whispererBoxLeft, 0x32, 0x56, 0, 0, true, 0x24, whispererBoxW, -0x20)
	b.WhispererBox.SetMargins(7, 7, 7, 7)
	b.WhispererBox.Hover = false
	b.WhispererBox.SetVisible(false)
	b.Whisperers = seui.NewSelectText(env, b.WhispererBox)
	b.Whisperers.SetBounds(whispererListLeft, whispererListTop, whispererRow, whispererBoxW)
	b.Whisperers.SetColor(inputTextInk)
	b.Whisperers.OnSelect = b.pickWhisperer
	b.initEmoticons()
	return b
}

// RememberWhisperer is FUN_00268cdc: a received whisper's speaker goes in
// before the last name (moved: false); a chosen target goes to the end,
// moving it if listed (moved: true). Past ten the oldest is dropped.
func (b *InputBar) RememberWhisperer(name []byte, moved bool) {
	l := b.Whisperers
	if i := l.IndexOf(name); i >= 0 {
		if !moved {
			return
		}
		l.Delete(i)
	}
	name = append([]byte(nil), name...)
	if !moved && l.Count() > 0 {
		l.Insert(name, l.Count()-1, "", "")
	} else {
		l.Add(name, "", "")
	}
	if l.Count() > whisperersMax {
		l.Delete(0)
	}
	h := l.Count()*whispererRow + whispererPad
	b.WhispererBox.Height, b.WhispererBox.Top = h, -h
	l.SetBounds(whispererListLeft, whispererListTop, l.Count()*whispererRow, whispererBoxW)
}

// pickWhisperer is FUN_00268c38.
func (b *InputBar) pickWhisperer(i int) {
	if i < 0 || i >= b.Whisperers.Count() {
		return
	}
	b.Whisper.SetText(b.Whisperers.Items[i])
	if b.OnWhisperName != nil {
		b.OnWhisperName()
	}
	b.WhispererBox.SetVisible(false)
}

// channelButton is a channel's button picture: "btn_channel_<n>_1".
func channelButton(n byte) string {
	return channelPicture + strconv.Itoa(int(n)) + channelPictureEnd
}

// newField is one of the bar's text fields: white text, five-pixel
// margins, a length limit and a hint.
func newField(env *seui.Env, owner seui.Control, left, width, maxLen int, hint string) *seui.Editor {
	e := seui.NewEditor(env, owner)
	e.Init("", left, inputFieldHeight, width, 0, 0, true, inputFieldHeight, width, inputFieldTop)
	e.SetMargins(inputFieldMargin, inputFieldMargin, inputFieldMargin, inputFieldMargin)
	e.SetColor(inputTextInk)
	e.Color2 = 0
	e.SetMaxLen(maxLen)
	if hint != "" {
		e.SetHint([]byte(hint))
	}
	return e
}

// Reset is FUN_00268980 (entering the world): the fields empty and the
// channel returns to Local.
func (b *InputBar) Reset() {
	b.ShowEmoticons(false)
	b.Message.Clear()
	b.Whisper.Clear()
	b.SelectChannel(InputLocal)
}

// nextChannel is the button's click (FUN_00269b30): World, Local, Whisper
// and Team in turn; from Guild it does nothing. It only acts while the
// channel list is closed, and the list opens as soon as the pointer is
// over the button, so a click leaves the channel alone.
func (b *InputBar) nextChannel() {
	if b.Frame.Visible {
		return
	}
	switch {
	case b.Channel >= InputWorld && b.Channel < InputTeam:
		b.SelectChannel(b.Channel + 1)
	case b.Channel == InputTeam:
		b.SelectChannel(InputWorld)
	}
}

// toggleList is the button's +0x60 handler (FUN_0026527c): the list
// opens or closes (the paint keeps it open under the pointer).
func (b *InputBar) toggleList() { b.showList(!b.Frame.Visible) }

// Presses on the bar itself (FUN_002695d8): the whisper field's area,
// whose read-only field takes no hits, selects Whisper.
var whisperArea = image.Rect(0x46, 0x240, 0x46+0x56, 0x240+0x16)

// LeftDown is the bar's press (FUN_002695d8). The mail icon's press
// (opening the mailbox) is not ported.
func (b *InputBar) LeftDown(shift byte, x, y int) {
	b.FixedForm.LeftDown(shift, x, y)
	if image.Pt(x, y).In(whisperArea) {
		b.SelectChannel(InputWhisper)
	}
}

// pickChannel is a list button's click (FUN_00267c5c).
func (b *InputBar) pickChannel(i int) {
	if i < 1 || i > channelListSlots || b.others[i] == 0 {
		return
	}
	b.SelectChannel(b.others[i])
	b.showList(false)
}

// SelectChannel is FUN_00267db0. The whisper field is read-only and empty
// outside Whisper, which takes the keyboard; Team and Guild without a team
// or guild say so and fall back to Local.
func (b *InputBar) SelectChannel(n byte) {
	b.Whisper.ReadOnly = true
	if n != InputWhisper {
		b.Whisper.Clear()
		b.Target = 0
	}
	switch n {
	case InputWhisper:
		b.Whisper.ReadOnly = false
		if b.Focus != nil {
			b.Focus(b.Whisper)
		}
	case InputTeam:
		if b.InTeam == nil || !b.InTeam() {
			b.notice(notInTeam)
			b.SelectChannel(InputLocal)
			return
		}
	case InputGuild:
		if b.InGuild == nil || !b.InGuild() {
			b.notice(notInGuild)
			b.SelectChannel(InputLocal)
			return
		}
	}
	b.Channel = n
	b.Switch.Image = b.Env.Pics.Find(channelButton(n))
}

func (b *InputBar) notice(text string) {
	if b.Notice != nil {
		b.Notice(text)
	}
}

// Remember is the end of a sent message (FUN_002641c4): it joins the last
// ten, and the field empties.
func (b *InputBar) Remember() {
	if len(b.history) >= inputHistory {
		b.history = b.history[1:]
	}
	b.history = append(b.history, append([]byte(nil), b.Message.Text...))
	b.recalled = len(b.history)
	b.Message.Clear()
}

// recallOlder is the message field's Up (FUN_00265178).
func (b *InputBar) recallOlder() {
	if len(b.history) == 0 {
		return
	}
	b.Message.SetText(b.history[b.recalled-1])
	b.recalled--
	if b.recalled < 1 {
		b.recalled = len(b.history)
	}
}

// recallNewer is its Down (FUN_002651f8).
func (b *InputBar) recallNewer() {
	if len(b.history) == 0 {
		return
	}
	b.recalled++
	if b.recalled > len(b.history) {
		b.recalled = 1
	}
	b.Message.SetText(b.history[b.recalled-1])
}

// showList shows or hides the channel list, filling its buttons with the
// channels other than the current one (FUN_00268194).
func (b *InputBar) showList(on bool) {
	b.Frame.SetVisible(on)
	slot := 0
	for n := byte(1); n <= inputChannels; n++ {
		if n == b.Channel || slot == channelListSlots {
			continue
		}
		slot++
		b.others[slot] = n
		b.Others[slot].Image = b.Env.Pics.Find(channelButton(n))
	}
	for i := 1; i <= channelListSlots; i++ {
		b.Others[i].SetVisible(on)
	}
}

// Paint is FUN_00268194's drawing: the channel list follows the pointer,
// then the bar, the channel's icon and the mail icon's first frame.
func (b *InputBar) Paint() {
	if b.Pointer != nil {
		x, y := b.Pointer()
		pt := image.Pt(x, y)
		button := image.Rect(b.Left+inputSwitchHover, b.Top, b.Left+inputSwitchHover+inputSwitchW, b.Top+inputSwitchH)
		list := image.Rect(b.Left+inputSwitchHover, b.Top-inputListHeight, b.Left+inputSwitchHover+inputSwitchW, b.Top)
		b.showList(pt.In(button) || (b.Frame.Visible && pt.In(list)))
		if b.Whisperers.Count() > 0 && b.Channel == InputWhisper {
			field := image.Rect(b.Left+whisperFieldHover, b.Top+1, b.Left+whisperFieldHover+whisperFieldW, b.Top+1+whisperFieldH)
			box := b.WhispererBox.Rect()
			b.WhispererBox.SetVisible(pt.In(field) || (b.WhispererBox.Visible && pt.In(box)))
		} else {
			b.WhispererBox.SetVisible(false)
		}
	}
	b.FixedForm.Paint()
	pics, scr := b.Env.Pics, b.Env.Screen
	if b.Channel >= 1 && b.Channel <= inputChannels {
		pics.Draw(scr, b.channels[b.Channel], b.Left+inputChannelX, b.Top+inputChannelY, true)
	}
	pics.DrawRect(scr, b.mail, b.Left+inputMailX, b.Top+inputMailY, image.Rect(0, 0, inputMailW, inputMailH), true)
}
