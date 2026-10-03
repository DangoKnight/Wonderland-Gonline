package hud

import (
	"bytes"
	"image"
	"time"

	"wonderland-go/client/wlo/seui"
)

// ChatLog is TTalkMsgForm (constructor FUN_0048bbf4, adding FUN_0048ac28,
// ticker FUN_0048d688): the chat log over the bottom left of the map, in
// its transparent mode. Lines go into the message list (TSe_CharMsg at
// 0x32, 0xe, 405 × 60), newest at the bottom, wrapped to its width and
// coloured by channel; system lines also run through the ticker below it.
// Not ported yet: scrolling, the lock, resizing and the other modes, the
// list's emoticons and VIP marks, and clicking a line to whisper.
type ChatLog struct {
	seui.FixedForm
	Lock, Up, Down, Switch *seui.FixedButton
	Now                    func() time.Time
	Lines                  []ChatLine

	ticker    []byte   // +0x170, the shown ticker text
	queue     [][]byte // +0x178, system lines waiting for the ticker
	message   []byte   // +0x174, the line being typed in
	next      int      // +0x188, its next character (from 1)
	plays     int      // +0x18a, how often it has played
	tickAt    time.Time
	scrollBar int
	strip     int // Panel_TalkForm2_1, the strip in this mode
}

// ChatLine is one wrapped row of the list.
type ChatLine struct {
	Text []byte
	Ink  uint16
}

const (
	chatLeft, chatTop       = 0, 0x1db
	chatWidth, chatHeight   = 0x208, 100
	chatListLeft            = 0x32
	chatListTop             = 0xe
	chatListWidth           = 0x208 - 0x73
	chatListHeight          = 100 - 0x28
	chatRowHeight           = 0x12
	chatCharWidth           = 8
	chatMaxLines            = 100 // FUN_0048e37c keeps 100 entries (20 in mode 2)
	chatTextStyle           = 2
	chatTickerX             = 0x32
	chatTickerY             = 0x22b
	chatTickerLead          = 60   // spaces before a message
	chatTickerShown         = 0x36 // characters the ticker shows
	chatTickerPlays         = 3
	chatTickerEvery         = 100 * time.Millisecond // +0x184; its value is not traced
	chatScrollLeft          = 10
	chatScrollTop           = 0x20
	chatScrollWidth         = 0xb
	chatArrowLeft           = 8
	chatUpTop               = 0xd
	chatDownFromBottom      = 0x17
	chatSwitchLeft          = 0x18
	chatSwitchFromBottom    = 0x15
	chatLockLeft, chatLockY = 7, -8
	// The strip is stretched to the window's height by repeating a row of
	// its scroll track; the thumb (bar_H4's first 9 columns) likewise.
	chatStripSplit  = 44
	chatScrollH     = 100 - 0x39
	chatThumbWidth  = 9
	chatThumbLeft   = chatScrollLeft + 1
	chatThumbTop    = 7
	chatThumbBottom = 7
)

// drawStretched draws a picture's columns [0, w) at (x, y), h tall, by
// keeping its rows above split and the rows below it at the bottom, and
// repeating row split in between.
func drawStretched(env *seui.Env, pic, x, y, w, h, split int) {
	pics, scr := env.Pics, env.Screen
	_, ph := pics.Size(pic)
	if pic < 0 || ph == 0 {
		return
	}
	split = min(split, ph-1)
	bottom := ph - split
	pics.DrawRect(scr, pic, x, y, image.Rect(0, 0, w, split), true)
	for row := split; row < h-bottom; row++ {
		pics.DrawRect(scr, pic, x, y+row, image.Rect(0, split, w, split+1), true)
	}
	pics.DrawRect(scr, pic, x, y+max(h-bottom, split), image.Rect(0, split, w, ph), true)
}

// Chat channels (TTalkMsgForm's list colours, FUN_0048d250, and the input
// bar's channels): system lines are 0.
const (
	ChannelSystem = 0
	ChannelWorld  = 1
	ChannelLocal  = 2
	ChannelWhisp  = 3
	ChannelTeam   = 4
	ChannelGuild  = 5
)

// chatPalette is the colour table FUN_003beb14 fills (0x72a360), and
// channelColor the entry each channel uses (FUN_0048bbf4).
var (
	chatPalette  = [...]uint16{0xfe31, 0xffb3, 0xc6f3, 0x8653, 0x6e7e, 0x8e27, 0xffff, 0xf800, 0xf4a3, 0xff80, 0xfc00}
	channelColor = map[int]int{0: 7, 1: 1, 2: 9, 3: 0, 4: 10, 5: 6, 6: 4, 7: 5, 8: 2, 10: 7, 11: 7}
)

// ChannelInk is a channel's line colour.
func ChannelInk(channel int) uint16 {
	if i, ok := channelColor[channel]; ok {
		return chatPalette[i]
	}
	return chatPalette[6]
}

// Line prefixes of FUN_0048ac28 (strings at 0x48b2c8…).
var (
	localTag  = []byte("(Local)")
	nameColon = []byte(":")
)

// NewChatLog is FUN_0048bbf4's resting layout.
func NewChatLog(env *seui.Env) *ChatLog {
	l := &ChatLog{Now: time.Now, scrollBar: env.Pics.Find("bar_H4"), strip: env.Pics.Find("panel_TalkForm2_1")}
	l.InitFixedForm(l, env)
	l.Name = "TTalkMsgForm"
	l.Draggable = false
	l.Init("", chatLeft, 0, 0, 0, 0, false, chatHeight, chatWidth, chatTop)
	l.Lock = newBarButton(env, l, "Btn_MsgLock_1", chatLockLeft, chatLockY, "Chatbox Lock")
	l.Up = newBarButton(env, l, "Btn_ArrowUp_1", chatArrowLeft, chatUpTop, "")
	l.Down = newBarButton(env, l, "Btn_ArrowDn_1", chatArrowLeft, chatHeight-chatDownFromBottom, "")
	l.Switch = newBarButton(env, l, "Btn_Transition_1", chatSwitchLeft, chatHeight-chatSwitchFromBottom, "Chat Box Switch")
	return l
}

// Add is FUN_0048ac28 for an already formatted line: it is wrapped into
// the list, and a system line also joins the ticker's queue.
func (l *ChatLog) Add(text []byte, channel int) {
	if channel == ChannelSystem {
		l.queue = append(l.queue, append([]byte(nil), text...))
	}
	ink := ChannelInk(channel)
	per := chatListWidth / chatCharWidth
	for len(text) > 0 {
		n := min(len(text), per)
		// Keep a double-byte character whole.
		if n < len(text) {
			for i := 0; i < n; i++ {
				if text[i] >= 0x80 {
					if i+1 == n {
						n--
						break
					}
					i++
				}
			}
		}
		l.Lines = append(l.Lines, ChatLine{append([]byte(nil), text[:n]...), ink})
		text = text[n:]
	}
	if over := len(l.Lines) - chatMaxLines; over > 0 {
		l.Lines = l.Lines[over:]
	}
}

// AddLocal is a Local channel line: "(Local)" + name + ":" + text.
func (l *ChatLog) AddLocal(name, text []byte) {
	line := bytes.Join([][]byte{localTag, name, nameColon, text}, nil)
	l.Add(line, ChannelLocal)
}

// Clear is FUN_0048cab4: the list and the ticker empty.
func (l *ChatLog) Clear() {
	l.Lines, l.queue, l.ticker, l.message, l.plays = nil, nil, nil, nil, 0
}

// tick is FUN_0048d688's text: a message enters from the right one
// character per interval behind 60 spaces, and plays three times.
func (l *ChatLog) tick() {
	if len(l.queue) == 0 {
		return
	}
	now := l.Now()
	if l.ticker == nil {
		l.plays++
		if l.plays > chatTickerPlays {
			l.plays = 0
			l.queue = l.queue[1:]
			if len(l.queue) == 0 {
				return
			}
			l.plays = 1
		}
		l.ticker = bytes.Repeat([]byte{' '}, chatTickerLead)
		l.message, l.next = l.queue[0], 0
		l.tickAt = now
	}
	for !now.Before(l.tickAt.Add(chatTickerEvery)) && l.ticker != nil {
		l.tickAt = l.tickAt.Add(chatTickerEvery)
		step := 1
		if l.ticker[0] >= 0x80 {
			step = 2
		}
		l.ticker = l.ticker[min(step, len(l.ticker)):]
		if len(l.ticker) < chatTickerShown && l.next < len(l.message) {
			n := 1
			if l.message[l.next] >= 0x80 && l.next+1 < len(l.message) {
				n = 2
			}
			l.ticker = append(l.ticker, l.message[l.next:l.next+n]...)
			l.next += n
		}
		if len(l.ticker) == 0 {
			l.ticker = nil
		}
	}
}

// Paint draws the scroll bar, the list's rows (newest at the bottom) and
// the ticker.
func (l *ChatLog) Paint() {
	scr, pics, txt := l.Env.Screen, l.Env.Pics, l.Env.Text
	sw, _ := pics.Size(l.strip)
	drawStretched(l.Env, l.strip, l.Left, l.Top, sw, chatHeight, chatStripSplit)
	// With every line in view the thumb fills the track.
	_, barH := pics.Size(l.scrollBar)
	drawStretched(l.Env, l.scrollBar, l.Left+chatThumbLeft, l.Top+chatScrollTop, chatThumbWidth, chatScrollH, barH/2)
	rows := chatListHeight / chatRowHeight
	first := max(len(l.Lines)-rows, 0)
	y := l.Top + chatListTop + (rows-(len(l.Lines)-first))*chatRowHeight
	for _, line := range l.Lines[first:] {
		w := len(line.Text)*chatCharWidth + chatCharWidth
		txt.Draw(l.Left+chatListLeft, y, 0, false, true, scr, line.Text, 0, w, 0, line.Ink, chatTextStyle)
		y += chatRowHeight
	}
	l.tick()
	if l.ticker != nil {
		shown := l.ticker[:min(len(l.ticker), chatTickerShown)]
		txt.Draw(chatTickerX, chatTickerY, 0, false, true, scr, shown, 0, len(shown)*chatCharWidth+chatCharWidth, 0, ChannelInk(ChannelSystem), chatTextStyle)
	}
}
