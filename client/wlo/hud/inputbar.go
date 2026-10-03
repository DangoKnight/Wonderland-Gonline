package hud

import (
	"image"
	"strconv"

	"wonderland-go/client/wlo/seui"
)

// InputBar is TSe_InputBar (constructor FUN_0026348c, paint FUN_00268194):
// the chat bar along the bottom. Ported is its resting layout: the bar,
// the current channel's icon with the mail icon over it, the channel
// button, the whisper name and message fields, and the emote button. Not
// ported yet: sending, the channel list (btn_channel_1..5 in
// icon_Channelframe_1), the emote panel (icon_expre_1..31), the mail
// animation, the battle buttons (Atk, Skill, Def, Catch, Flee, Help) and
// the viewer and auto-play buttons.
type InputBar struct {
	seui.FixedForm
	Channel  byte              // +0x140, 2 (Local) at start
	Switch   *seui.FixedButton // +0x1ac btn_channel_2_1, "Switch Channel"
	Whisper  *seui.Editor      // +0x138, the whisper target
	Message  *seui.Editor      // +0x134, the message
	Emotes   *seui.FixedButton // +0x1ec Btn_expression_1
	channels [inputChannels + 1]int
	mail     int
}

const (
	inputChannels     = 5
	inputLocal        = 2
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
)

// NewInputBar is FUN_0026348c's resting layout.
func NewInputBar(env *seui.Env) *InputBar {
	b := &InputBar{Channel: inputLocal, mail: env.Pics.Find("Anim_GotMailS_1")}
	b.InitFixedForm(b, env)
	b.Name = "TSe_InputBar"
	b.Draggable = false
	b.Init("main_input_1", 0, 0, 0, 0, 0, false, inputHeight, inputWidth, inputTop)
	for i := 1; i <= inputChannels; i++ {
		b.channels[i] = env.Pics.Find("btn_channel_" + strconv.Itoa(i))
	}
	b.Switch = newBarButton(env, b, "btn_channel_2_1", inputSwitchLeft, inputSwitchTop, "Switch Channel")
	b.Whisper = newField(env, b, inputWhisperLeft, inputWhisperWidth, inputWhisperMax, "Whisper")
	b.Message = newField(env, b, inputMessageLeft, inputMessageWidth, inputMessageMax, "Press Ctrl+V to paste")
	b.Emotes = newBarButton(env, b, "Btn_expression_1", inputEmoteLeft, 0, "Chat Emotes")
	return b
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
	e.SetHint([]byte(hint))
	return e
}

// Paint is FUN_00268194's drawing: the bar, the channel's icon and the
// mail icon's first frame.
func (b *InputBar) Paint() {
	b.FixedForm.Paint()
	pics, scr := b.Env.Pics, b.Env.Screen
	if b.Channel >= 1 && b.Channel <= inputChannels {
		pics.Draw(scr, b.channels[b.Channel], b.Left+inputChannelX, b.Top+inputChannelY, true)
	}
	pics.DrawRect(scr, b.mail, b.Left+inputMailX, b.Top+inputMailY, image.Rect(0, 0, inputMailW, inputMailH), true)
}
