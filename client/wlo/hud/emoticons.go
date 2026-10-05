package hud

import (
	"image"
	"strconv"
	"time"

	"wonderland-go/client/wlo/seui"
)

const (
	chatEmoticons     = 31
	emoticonSize      = 16
	emoticonColumns   = 10
	emoticonEvery     = 500 * time.Millisecond
	chatEmoticonEvery = 7 * 30 * time.Millisecond // TSe_CharMsg advances every seventh game frame
)

// Native text codes at PTR_DAT_004bdc18 in WLRI ca19ee087b60. These
// travel as ordinary chat text; they are separate from AC32 role expressions.
var emoticonCodes = [chatEmoticons]string{
	"@|", ":$", ">|", "-\"", "XD", "TT", ":D", ":S", "==", "8D",
	"%#", "8(", ":)", "^T", ">@", ":(", ":!", "JL", "8o", ":[",
	":m", ":o", "|T", "\"_", "*:", "j-", ":P", "|E", "zZ", "@#", "-P",
}

// initEmoticons ports FUN_0026348c's picker and FUN_0026a008's code
// insertion. Its 31st icon occupies the otherwise empty bottom-right cell.
func (b *InputBar) initEmoticons() {
	b.Message.TokenWidth = chatUnit
	b.Message.DrawText = b.drawMessageText
	b.EmoteClose = newBarButton(b.Env, b, "Btn_Close_s_1", 733, -66, "Close Chat Emotes")
	b.EmoteClose.OnClick = func() { b.ShowEmoticons(false) }
	for i, code := range emoticonCodes {
		x, y := 500+(i%emoticonColumns)*23, -62+(i/emoticonColumns)*20
		if i == chatEmoticons-1 {
			x, y = 730, -22
		}
		name := "icon_expre_" + strconv.Itoa(i+1)
		icon := seui.NewFixedButton(b.Env, b)
		icon.Init(name, x, emoticonSize, emoticonSize, 0, 0, true, emoticonSize, emoticonSize, y)
		icon.SetAnimation(name, 0, 2, 1, emoticonEvery, emoticonSize, emoticonSize, 0)
		icon.StartAnimation()
		icon.SetHint([]byte(code))
		icon.Tag, icon.OnClickTag = i, b.InsertEmoticon
		b.EmoteIcons[i] = icon
	}
	// Children paint in reverse creation order, matching TSe_GrBasic.
	// Register the backdrop last so it stays beneath the icons and close button.
	b.EmotePanel = seui.NewPanel(b.Env, b)
	b.EmotePanel.Init("panel_expression_1", 492, 69, 263, 0, 0, true, 69, 263, -69)
	b.EmotePanel.Hover = false
	b.Emotes.OnClick = func() {
		if !b.Blocked() {
			b.ShowEmoticons(!b.EmotePanel.Visible)
		}
	}
	b.ShowEmoticons(false)
}

func (b *InputBar) ShowEmoticons(visible bool) {
	b.EmotePanel.SetVisible(visible)
	b.EmoteClose.SetVisible(visible)
	for _, icon := range b.EmoteIcons {
		icon.SetVisible(visible)
	}
}

func (b *InputBar) InsertEmoticon(index int) {
	if index < 0 || index >= chatEmoticons || b.Blocked() || b.Message.ReadOnly {
		return
	}
	code := emoticonCodes[index]
	// The native picker reserves both bytes before insertion, so a full
	// field never receives half a code.
	if b.Message.Limit && len(b.Message.Text)+len(code) > b.Message.MaxLen {
		return
	}
	for _, c := range []byte(code) {
		b.Message.Char(c)
	}
	if b.Focus != nil {
		b.Focus(b.Message)
	}
}

func emoticonAt(s []byte) int {
	if len(s) < 2 || s[0] >= 0x80 {
		return -1
	}
	for i, code := range emoticonCodes {
		if s[0] == code[0] && s[1] == code[1] {
			return i
		}
	}
	return -1
}

// chatUnit keeps Big5 characters and native two-byte emoticon codes whole.
func chatUnit(s []byte) int {
	if len(s) > 1 && (s[0] >= 0x80 || emoticonAt(s) >= 0) {
		return 2
	}
	return 1
}

// drawChatText follows TSe_CharMsg's token scan: each emoticon uses the
// same 16 pixels as its two text bytes. Missing icons retain readable text.
func (l *ChatLog) drawChatText(x, y int, line ChatLine) {
	txt, pics, scr := l.Env.Text, l.Env.Pics, l.Env.Screen
	s := line.Text
	for len(s) > 0 {
		n := chatUnit(s)
		if icon := emoticonAt(s); icon >= 0 {
			pic := pics.Find("icon_expre_" + strconv.Itoa(icon+1))
			if pic >= 0 {
				_, h := pics.Size(pic)
				frame := int(l.Now().UnixMilli()/chatEmoticonEvery.Milliseconds()) % max(h/emoticonSize, 1)
				pics.DrawRect(scr, pic, x, y-2, image.Rect(0, frame*emoticonSize, emoticonSize, (frame+1)*emoticonSize), true)
				x += emoticonSize
				s = s[2:]
				continue
			}
		}
		txt.Draw(x, y, 0, false, true, scr, s[:n], 0, n*chatCharWidth+chatCharWidth, 0, line.Ink, chatTextStyle)
		x += n * chatCharWidth
		s = s[n:]
	}
}

// drawMessageText ports FUN_00269734's preview: native text codes remain
// in the buffer and on the wire, while each occupies a 16-pixel animated icon.
// The input bar advances its preview every seventh 30 ms frame, like the log.
func (b *InputBar) drawMessageText(x, y int, s []byte) {
	e := b.Message
	for len(s) > 0 {
		n := chatUnit(s)
		if icon := emoticonAt(s); icon >= 0 {
			pic := b.Env.Pics.Find("icon_expre_" + strconv.Itoa(icon+1))
			if pic >= 0 {
				_, h := b.Env.Pics.Size(pic)
				frame := int(e.Now().UnixMilli()/chatEmoticonEvery.Milliseconds()) % max(h/emoticonSize, 1)
				b.Env.Pics.DrawRect(b.Env.Screen, pic, x, y, image.Rect(0, frame*emoticonSize, emoticonSize, (frame+1)*emoticonSize), true)
				x += emoticonSize
				s = s[n:]
				continue
			}
		}
		b.Env.Text.Draw(x, y, 0, false, true, b.Env.Screen, s[:n], e.CharH, e.Width, e.Color2, e.Color, e.TextStyle)
		x += n * e.CharW
		s = s[n:]
	}
}
