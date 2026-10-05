package hud

import (
	"bytes"
	"image"
	"time"

	"wonderland-go/client/wlo/settings"
	"wonderland-go/client/wlo/seui"
	"wonderland-go/client/wlo/surface"
)

// ChatLog is TTalkMsgForm (constructor FUN_0048bbf4, adding FUN_0048ac28,
// ticker FUN_0048d688): the chat log over the bottom left of the map, in
// its transparent mode. Lines go into the message list (TSe_CharMsg at
// 0x32, 0xe, 405 × 60), newest at the bottom, wrapped to its width and
// coloured by channel; system lines also run through the ticker below it.
// The scroll position and lock follow FUN_0048ac28/0048c9d4. Alternate
// backgrounds and VIP marks remain pending.
type ChatLog struct {
	seui.FixedForm
	Lock, Up, Down, Switch *seui.FixedButton
	Now                    func() time.Time
	Lines                  []ChatLine
	Scroll                 *seui.ScrollButton
	Locked                 bool
	Mode                   ChatMode
	background             *seui.Panel
	SelfID                 func() uint32
	OnSpeaker              func(uint32)
	firstRow               int
	history                []ChatLine
	dragMoved              bool
	grid                   int
	Resize                 *chatResize
	Colors                 map[int]uint16 // local channel palette overrides
	// Faces finds a speaker's look by ID (the player, then the map's
	// players); nil leaves the row without an icon.
	Faces func(id uint32) SpeakerFace
	// Pointer is the pointer's screen position; Hovered is the speaker
	// whose icon is under it (+0x500, cleared by every paint), which a
	// press whispers to (FUN_00496ca4).
	Pointer func() (int, int)
	Hovered uint32

	ticker  []byte   // +0x170, the shown ticker text
	queue   [][]byte // +0x178, system lines waiting for the ticker
	message []byte   // +0x174, the line being typed in
	next    int      // +0x188, its next character (from 1)
	plays   int      // +0x18a, how often it has played
	tickAt  time.Time
	strip   int // Panel_TalkForm2_1, the strip in this mode
}

// ChatLine is one wrapped row of the list.
type ChatLine struct {
	Text    []byte
	Ink     uint16
	Channel int    // the row's channel (+0x4cc)
	Speaker uint32 // the speaker's ID (+0x4d4), 0 for none
	Entry   int    // retained message index, used to preserve a locked viewport on rewrap
	Offset  int    // byte offset within that message
	First   bool   // the entry's first row
}

// SpeakerFace draws a frame of a speaker's face sprite
// (Human.DrawFaceFrame).
type SpeakerFace interface {
	DrawFaceFrame(dst *surface.Surface, x, y, action, frame int)
	// FaceHit tests a point against the frame's opaque pixels.
	FaceHit(x, y, action, frame, px, py int) bool
}

const (
	chatLeft, chatTop       = 0, 0x1db
	chatWidth, chatHeight   = 0x208, 100
	chatListLeft            = 0x32
	chatListTop             = 0xe
	chatListWidth           = 0x208 - 0x73
	chatListHeight          = 100 - 0x28
	chatRowHeight           = 0x14
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
	chatStripSplit = 44
	chatScrollH    = 100 - 0x39
	chatThumbWidth = 9
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

// Chat channels of the list (FUN_0048ac28's last argument, also the
// colour entry): 2/n arrives on channel n for n up to 7.
const (
	ChannelSystem  = 0  // no prefix ("(SystemPromp):" without a speaker); joins the ticker
	ChannelWorld   = 1  // "(World)"
	ChannelLocal   = 2  // "(Local)"
	ChannelWhisper = 3  // "(Whisp)"
	ChannelGM      = 4  // "(GM)"
	ChannelTeam    = 5  // "(Team)"
	ChannelGuild   = 6  // "(Guild)"
	ChannelAlly    = 7  // "(Ally)"
	ChannelNotice  = 10 // the client's own messages, as given
	ChannelPrompt  = 11 // "(System):"
	ChannelBouquet = 12 // "(Bouquet):"
)

// chatPalette is the colour table FUN_003beb14 fills (0x72a360), and
// channelColor the entry each channel uses. The list starts with
// FUN_0048bbf4's entries, but every sent and received line first copies
// the player's ChannelColor1..5 setting into its channel's slot (+0x4e0 +
// channel·2); without a save those are Local 9, Whisper 10, Team 6, Guild
// 4 and World 1 (0x284d05), so Whisper is orange (0xfc00), as
// Chat_02/Connection_Lost.png shows.
var (
	chatPalette  = settings.Palette
	channelColor = map[int]int{0: 7, 1: 1, 2: 9, 3: 10, 4: 10, 5: 6, 6: 4, 7: 5, 8: 2, 10: 7, 11: 7}
)

// ChannelInk is a channel's line colour.
func ChannelInk(channel int) uint16 {
	if i, ok := channelColor[channel]; ok {
		return chatPalette[i]
	}
	return chatPalette[6]
}

// Line prefixes of FUN_0048ac28 (strings at 0x48b284…): a speaker's
// line reads prefix + name + ":" + text.
var (
	channelTag = map[int][]byte{
		ChannelWorld:   []byte("(World)"),
		ChannelLocal:   []byte("(Local)"),
		ChannelWhisper: []byte("(Whisp)"),
		ChannelGM:      []byte("(GM)"),
		ChannelTeam:    []byte("(Team)"),
		ChannelGuild:   []byte("(Guild)"),
		ChannelAlly:    []byte("(Ally)"),
	}
	systemPromptTag = []byte("(SystemPromp):")
	promptTag       = []byte("(System):")
	bouquetTag      = []byte("(Bouquet):")
	nameColon       = []byte(":")
)

// Speaker icons (FUN_004905e8): rows of channels 1..7 whose speaker is
// known show the speaker's face sprite in action 4 (+0x121 = 4) at its
// second frame (+0x11e = 1, the head turned three-quarters, as
// Chat_Icons_02.png shows), 10 pixels left of the list and 10 below the
// row's top.
const (
	chatFaceAction = 4
	chatFaceFrame  = 1
	chatFaceLeft   = -10
	chatFaceDown   = 10
)

// NewChatLog is FUN_0048bbf4's resting layout.
func NewChatLog(env *seui.Env) *ChatLog {
	l := &ChatLog{Mode: ChatTransparent, Now: time.Now, grid: env.Pics.Find("32X32Grid"), strip: env.Pics.Find("panel_TalkForm2_1")}
	l.InitFixedForm(l, env)
	l.Name = "TTalkMsgForm"
	l.Draggable = false
	l.Init("", chatLeft, 0, 0, 0, 0, false, chatHeight, chatWidth, chatTop)
	l.Lock = newBarButton(env, l, "Btn_MsgLock_1", chatLockLeft, chatLockY, "Chatbox Lock")
	l.Up = newBarButton(env, l, "Btn_ArrowUp_1", chatArrowLeft, chatUpTop, "")
	l.Down = newBarButton(env, l, "Btn_ArrowDn_1", chatArrowLeft, chatHeight-chatDownFromBottom, "")
	l.Switch = newBarButton(env, l, "Btn_Transition_1", chatSwitchLeft, chatHeight-chatSwitchFromBottom, "Chat Box Switch")
	l.Scroll = seui.NewScrollButton(env, l)
	l.Scroll.Init("", chatScrollLeft, 0, 0, 0, 0, false, chatScrollH, chatScrollWidth, chatScrollTop)
	l.Scroll.SetThumb("bar_H4", 0, 21, chatThumbWidth, 0)
	l.Scroll.SetCaps(7, 16)
	l.Scroll.OnChange = func() { l.firstRow = l.Scroll.Pos }
	l.Scroll.OnPageUp, l.Scroll.OnPageDown = l.Scroll.OnChange, l.Scroll.OnChange
	l.Up.OnClick = l.WheelUp
	l.Down.OnClick = l.WheelDown
	l.Lock.OnClick = func() { l.SetLocked(!l.Locked) }
	l.OnClick = func() {
		if !l.dragMoved && !l.Blocked() && l.OnSpeaker != nil && l.Pointer != nil {
			x, y := l.Pointer()
			if id := l.SpeakerAt(x, y); id != 0 {
				l.OnSpeaker(id)
			}
		}
	}
	l.background = seui.NewPanel(env, nil)
	l.background.Init("panel_TalkForm", chatLeft, 86, 524, 0, 0, true, chatHeight, chatWidth, chatTop)
	l.background.SetMargins(66, 38, 36, 90)
	l.Resize = newChatResize(env, l)
	l.Switch.OnClick = func() { l.SetMode((l.Mode + 1) % chatModes) }
	l.SetMode(ChatTransparent)
	l.syncScroll()
	return l
}

// Say is FUN_0048ac28: the line is formatted for its channel, with the
// speaker's name, and added.
func (l *ChatLog) Say(speaker uint32, name, text []byte, channel int) {
	var line []byte
	switch channel {
	case ChannelSystem:
		if speaker == 0 {
			line = append(append([]byte(nil), systemPromptTag...), text...)
		} else {
			line = text
		}
	case ChannelPrompt:
		line = append(append([]byte(nil), promptTag...), text...)
	case ChannelBouquet:
		line = append(append([]byte(nil), bouquetTag...), text...)
	default:
		if tag, ok := channelTag[channel]; ok {
			line = bytes.Join([][]byte{tag, name, nameColon, text}, nil)
		} else {
			line = text
		}
	}
	// Native self messages release the lock and return to the newest entry.
	if l.Locked && l.SelfID != nil && speaker != 0 && speaker == l.SelfID() {
		l.SetLocked(false)
	}
	l.add(line, channel, speaker)
}

// Add adds an already formatted line without a speaker; a system line
// also joins the ticker's queue.
func (l *ChatLog) Add(text []byte, channel int) { l.add(text, channel, 0) }

// Notice is a channel-10 line: the client's own messages ("No target"),
// red and without a prefix.
func (l *ChatLog) Notice(text string) { l.add([]byte(text), ChannelNotice, 0) }

// add wraps a line into the list (FUN_0048e37c).
func (l *ChatLog) add(text []byte, channel int, speaker uint32) {
	if channel == ChannelSystem {
		l.queue = append(l.queue, append([]byte(nil), text...))
	}
	ink := ChannelInk(channel)
	if v, ok := l.Colors[channel]; ok {
		ink = v
	}
	l.history = append(l.history, ChatLine{Text: append([]byte(nil), text...), Ink: ink, Channel: channel, Speaker: speaker})
	if over := len(l.history) - chatMaxLines; over > 0 {
		l.history = l.history[over:]
		// Shift the viewport by the rows belonging to discarded messages.
		dropped := 0
		for _, row := range l.Lines {
			if row.Entry < over {
				dropped++
			}
		}
		l.firstRow = max(l.firstRow-dropped, 0)
	}
	l.rewrap(false)
	if !l.Locked {
		l.firstRow = max(len(l.Lines)-l.rows(), 0)
	}

	l.syncScroll()
}

// Clear is FUN_0048cab4: the list and the ticker empty.
func (l *ChatLog) Clear() {
	l.history = nil
	l.Lines, l.queue, l.ticker, l.message, l.plays = nil, nil, nil, nil, 0
	l.firstRow, l.Hovered = 0, 0
	l.syncScroll()
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
	if l.Mode == ChatOpaque && l.background != nil {
		l.drawBackgroundGrid()
		l.background.Left, l.background.Top = l.Left, l.Top
		l.background.Paint()
	} else if l.Mode == ChatTransparent {
		sw, _ := pics.Size(l.strip)
		drawStretched(l.Env, l.strip, l.Left, l.Top, sw, l.Height, chatStripSplit)
	}
	l.Hovered = 0
	first, end, y := l.visibleRows()
	if l.Mode == ChatTickerOnly {
		end = first
	}
	for _, line := range l.Lines[first:end] {
		if line.First && line.Speaker != 0 && line.Channel >= ChannelWorld && line.Channel <= ChannelAlly && l.Faces != nil {
			if f := l.Faces(line.Speaker); f != nil {
				fx, fy := l.Left+chatListLeft+chatFaceLeft, y+chatFaceDown
				f.DrawFaceFrame(scr, fx, fy, chatFaceAction, chatFaceFrame)
				if l.Pointer != nil {
					if px, py := l.Pointer(); f.FaceHit(fx, fy, chatFaceAction, chatFaceFrame, px, py) {
						l.Hovered = line.Speaker
					}
				}
			}
		}
		l.drawChatText(l.Left+chatListLeft, y, line)
		y += chatRowHeight
	}
	l.tick()
	if l.ticker != nil {
		shown := l.ticker[:l.tickerBytes()]
		txt.Draw(l.Left+chatTickerX, l.Top+l.Height-(chatTop+chatHeight-chatTickerY), 0, false, true, scr, shown, 0, len(shown)*chatCharWidth+chatCharWidth, 0, ChannelInk(ChannelSystem), chatTextStyle)
	}
}

// SetLocked freezes the current viewport while other players add messages.
// Unlocking follows the tail again (FUN_0048c9d4).
func (l *ChatLog) SetLocked(locked bool) {
	l.Locked = locked
	l.Draggable = l.Mode == ChatOpaque && !locked
	if locked {
		l.Dragging = false
	}
	if !locked {
		l.firstRow = max(len(l.Lines)-l.rows(), 0)
	}
	if l.Lock != nil {
		picture, hint := "Btn_MsgLock_1", "Chatbox: Unlocked"
		if locked {
			picture, hint = "Btn_MsgLock_2", "Chatbox: Locked"
		}
		l.Lock.Image = l.Env.Pics.Find(picture)
		l.Lock.SetHint([]byte(hint))
	}
	l.syncScroll()
}

func (l *ChatLog) listWidth() int {
	width := l.Width
	if width == 0 {
		width = chatWidth
	}
	return width - (chatWidth - chatListWidth)
}
func (l *ChatLog) rows() int {
	height := l.Height
	if height == 0 {
		height = chatHeight
	}
	return max((height-(chatHeight-chatListHeight))/chatRowHeight, 1)
}

func (l *ChatLog) syncScroll() {
	l.firstRow = max(0, min(l.firstRow, max(len(l.Lines)-l.rows(), 0)))
	if l.Scroll != nil {
		l.Scroll.SetRange(len(l.Lines), l.rows())
		l.Scroll.SetPos(l.firstRow)
	}
}

func (l *ChatLog) WheelUp() {
	if l.Env != nil && l.Blocked() {
		return
	}
	l.firstRow = max(l.firstRow-1, 0)
	l.syncScroll()
}
func (l *ChatLog) WheelDown() {
	if l.Env != nil && l.Blocked() {
		return
	}
	l.firstRow++
	l.syncScroll()
}

func (l *ChatLog) visibleRows() (first, end, y int) {
	first = max(0, min(l.firstRow, max(len(l.Lines)-l.rows(), 0)))
	end = min(first+l.rows(), len(l.Lines))
	y = l.Top + chatListTop + (l.rows()-(end-first))*chatRowHeight
	return
}

// SpeakerAt supports the native list's "Click to Whisper" affordance,
// including continuation rows rather than just portrait pixels.
func (l *ChatLog) SpeakerAt(x, y int) uint32 {
	first, end, top := l.visibleRows()
	if x < l.Left+chatListLeft || x >= l.Left+chatListLeft+l.listWidth() || y < top {
		return 0
	}
	row := first + (y-top)/chatRowHeight
	if row >= end {
		return 0
	}
	line := l.Lines[row]
	if line.Channel < ChannelWorld || line.Channel > ChannelAlly {
		return 0
	}
	return line.Speaker
}

// Chat modes at +0x169 (FUN_0048d2e4, switch callback at 0x48dd78).
type ChatMode byte

const (
	ChatOpaque ChatMode = iota
	ChatTransparent
	ChatTickerOnly
	chatModes
)

func (l *ChatLog) SetMode(mode ChatMode) {
	if mode >= chatModes {
		return
	}
	l.Mode = mode
	l.Draggable = mode == ChatOpaque && !l.Locked
	l.Dragging = false
	// All three modes share the same geometry, as Chat_Resizing shows.
	if l.Width == 0 || l.Height == 0 {
		l.Left, l.Top, l.Width, l.Height = chatLeft, chatTop, chatWidth, chatHeight
	}
	l.layoutChat()
	l.rewrap(true)
	l.syncScroll()
	show := mode != ChatTickerOnly
	if l.Lock != nil {
		l.Resize.SetVisible(mode == ChatOpaque)
		l.Lock.SetVisible(show)
		l.Up.SetVisible(show)
		l.Down.SetVisible(show)
		l.Scroll.SetVisible(show)
		l.Switch.Left = chatSwitchLeft
		if !show {
			l.Switch.Left = 0
		}
	}
}

// The transparent form has no background under the messages: clicks there
// reach the map (FUN_0046aadc). Portrait clicks are handled before UI dispatch.
func (l *ChatLog) HitTest(x, y int) bool {
	if l.Mode == ChatOpaque {
		return image.Pt(x, y).In(l.Rect())
	}
	if l.Mode == ChatTickerOnly {
		return false
	}
	return image.Pt(x, y).In(image.Rect(l.Left, l.Top, l.Left+chatListLeft, l.Top+l.Height))
}
