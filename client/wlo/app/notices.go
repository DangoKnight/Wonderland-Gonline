package app

import (
	"bytes"
	"time"

	"wonderland-go/client/wlo/seui"
)

// Notices is the notice board (PTR_DAT_004c9e40, TRe_TalkMsgFormPlus_1,
// constructor FUN_0034e50c), a TSe_TalkMsgFormPlus talk window used for
// timed messages. Its slot +0x9c (FUN_0034e70c) adds a message:
//
//   - at most five entries are kept (+0xf1c), each with its own duration
//     (+0x398 + i·0x10c) and start (+0x39c + i·0x10c);
//   - a message containing line breaks ("\r") replaces all entries and
//     keeps its last five lines;
//   - the entries are joined with "\r" and shown through the talk window's
//     text setter (FUN_0046fe50), whose timer is the newest message's
//     duration. When that timer runs out, every entry is cleared
//     (FUN_0034e3c8); with more than one entry, one expired entry is
//     removed per frame (FUN_0034e614).
//
// The talk window itself (buttons, portraits, typing effects) is not
// ported. Its layout here was measured from the original's capture
// (Login_Forced_Error_Message.png) with the margins of the window's
// FUN_00477d2c(0x32, 0x1f, 0x1f, 0x32) and Init("panel22", …): the width
// is the longest line's 8-pixel cells plus 110, the height 18 pixels a
// line plus 62, lines are centred and wrap at word boundaries.
const (
	noticeEntries   = 5
	noticeBreak     = '\r'
	noticeWrapChars = 32
	noticeCharW     = 8
	noticeLineH     = 18
	noticeSideW     = 0x32 + 0x32 + 10
	noticeEdgeH     = 0x1f + 0x1f
	noticeTop       = 0x1b
	noticeTextY     = 0x21 // first line below the panel's top
	noticeTextShift = -3   // the capture's text sits 3 pixels left of centre
	noticeInk       = 0xffe0
	noticeStyle     = 2
)

type notice struct {
	text  []byte
	d     time.Duration
	start time.Time
}

type Notices struct {
	items []notice
	shown bool      // +0x196, the talk window's timer is running
	until time.Time // its end; zero for no timeout
	panel *seui.Panel
}

// Show is FUN_0034e70c with a duration of d (0 or less never times out).
func (n *Notices) Show(text []byte, d time.Duration, now time.Time) {
	parts := bytes.Split(text, []byte{noticeBreak})
	if len(parts) > 1 && len(parts[len(parts)-1]) == 0 {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > 1 {
		n.items = n.items[:0]
	}
	if len(n.items)+1 > noticeEntries {
		n.items = n.items[1:]
	}
	if len(parts) > noticeEntries {
		parts = parts[len(parts)-noticeEntries:]
	}
	for _, p := range parts {
		if len(n.items) < noticeEntries {
			n.items = append(n.items, notice{append([]byte(nil), p...), d, now})
		}
	}
	n.shown = true
	n.until = time.Time{}
	if d > 0 {
		n.until = now.Add(d)
	}
}

// update is FUN_0034e614's bookkeeping.
func (n *Notices) update(now time.Time) {
	if n.shown && !n.until.IsZero() && !now.Before(n.until) {
		n.shown = false
	}
	if !n.shown {
		n.items = n.items[:0]
		return
	}
	if len(n.items) > 1 {
		for i, it := range n.items {
			if it.d > 0 && !now.Before(it.start.Add(it.d)) {
				n.items = append(n.items[:i], n.items[i+1:]...)
				break
			}
		}
	}
}

// wrap splits the joined text into display lines of at most
// noticeWrapChars bytes, breaking after a space when it can. Big5 pairs are
// kept together.
func wrap(text []byte) [][]byte {
	var lines [][]byte
	for _, para := range bytes.Split(text, []byte{noticeBreak}) {
		for len(para) > noticeWrapChars {
			cut := noticeWrapChars
			for i := 0; i < noticeWrapChars; {
				if para[i] > 0x80 && i+1 < len(para) {
					if i+2 > noticeWrapChars {
						cut = i
						break
					}
					i += 2
					continue
				}
				i++
			}
			if sp := bytes.LastIndexByte(para[:cut], ' '); sp > 0 {
				cut = sp + 1
			}
			lines = append(lines, para[:cut])
			para = para[cut:]
		}
		lines = append(lines, para)
	}
	return lines
}

// Draw updates and draws the board.
func (n *Notices) Draw(env *seui.Env, now time.Time) {
	n.update(now)
	if len(n.items) == 0 {
		return
	}
	texts := make([][]byte, len(n.items))
	for i, it := range n.items {
		texts[i] = it.text
	}
	lines := wrap(bytes.Join(texts, []byte{noticeBreak}))
	longest := 0
	for _, l := range lines {
		longest = max(longest, len(l))
	}
	w := longest*noticeCharW + noticeSideW
	h := len(lines)*noticeLineH + noticeEdgeH
	if n.panel == nil {
		n.panel = seui.NewPanel(env, nil)
		n.panel.Init("panel22", 0, 0xa8, 0x14c, 0, 0, true, 0, 0, 0)
		n.panel.SetMargins(0x32, 0x20, 0x20, 0x3c)
		n.panel.Hover = false
	}
	p := n.panel
	p.Left, p.Top, p.Width, p.Height = (ScreenWidth-w)/2, noticeTop, w, h
	p.Paint()
	for i, l := range lines {
		x := (ScreenWidth-len(l)*noticeCharW)/2 + noticeTextShift
		y := noticeTop + noticeTextY + i*noticeLineH
		env.Text.Draw(x, y, 0, false, true, env.Screen, l, noticeLineH, len(l)*noticeCharW+noticeCharW, 0, noticeInk, noticeStyle)
	}
}
