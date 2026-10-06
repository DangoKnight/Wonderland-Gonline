package hud

import (
	"image"

	"wonderland-gonline/client/wlo/seui"
)

const (
	chatMinimumWidth  = 224
	chatMinimumHeight = 72
	chatResizeRight   = 24
	chatResizeTop     = 2
	chatResizeBottom  = chatTop + chatHeight // preserve the HUD alignment shown in Chat_Background
	chatScreenWidth   = 800
	chatResizeSize    = 20
)

// rewrap rebuilds presentation rows from retained messages. A locked viewport
// stays on the same message/byte offset when its number of wrapped rows changes.
func (l *ChatLog) rewrap(anchor bool) {
	var entry, offset int
	anchored := anchor && l.Locked && l.firstRow < len(l.Lines)
	if anchored {
		entry, offset = l.Lines[l.firstRow].Entry, l.Lines[l.firstRow].Offset
	}
	l.Lines = nil
	per := max(l.listWidth()/chatCharWidth, 2)
	for i, original := range l.history {
		s := original.Text
		for start := 0; start < len(s); {
			end := start
			for end < len(s) {
				step := chatUnit(s[end:])
				if end+step-start > per {
					break
				}
				end += step
			}
			row := original
			row.Text = s[start:end]
			row.Entry, row.Offset, row.First = i, start, start == 0
			l.Lines = append(l.Lines, row)
			if anchored && i == entry && start <= offset && offset < end {
				l.firstRow = len(l.Lines) - 1
			}
			start = end
		}
	}
	if !l.Locked {
		l.firstRow = max(len(l.Lines)-l.rows(), 0)
	}
}

func (l *ChatLog) layoutChat() {
	if l.background != nil {
		l.background.Width, l.background.Height = l.Width, l.Height
	}
	if l.Resize != nil {
		l.Resize.Left, l.Resize.Top = l.Width-chatResizeRight, chatResizeTop
	}
	if l.Down != nil {
		l.Down.Top = l.Height - chatDownFromBottom
		l.Switch.Top = l.Height - chatSwitchFromBottom
		l.Scroll.Height = l.Height - (chatHeight - chatScrollH)
	}
}

// ResizeTo follows FUN_0048e004: even dimensions, native minima, bounded
// screen coordinates and the form's existing HUD-aligned bottom.
func (l *ChatLog) ResizeTo(width, height int) {
	if l.Mode != ChatOpaque {
		return
	}
	bottom := min(l.Top+l.Height, chatResizeBottom)
	bottom = max(bottom, chatMinimumHeight)
	l.Width = min(max(width/2*2, chatMinimumWidth), chatScreenWidth-l.Left)
	l.Height = min(max(height/2*2, chatMinimumHeight), bottom)
	l.Top = bottom - l.Height
	l.layoutChat()
	l.rewrap(true)
	l.syncScroll()
}

type chatResize struct {
	seui.Component
	log           *ChatLog
	dragging      bool
	start         image.Point
	width, height int
}

func newChatResize(env *seui.Env, l *ChatLog) *chatResize {
	r := &chatResize{log: l}
	r.InitComponent(r, env, l)
	r.Name = "TSe_SizeBtn"
	r.Init("Btn_Resize_1", l.Width-chatResizeRight, chatResizeSize, chatResizeSize, 0, 0, true, chatResizeSize, chatResizeSize, chatResizeTop)
	r.SetHint([]byte("Resize chat window"))
	return r
}
func (r *chatResize) LeftDown(shift byte, x, y int) {
	r.Component.LeftDown(shift, x, y)
	if r.Blocked() || r.log.Mode != ChatOpaque {
		return
	}
	r.dragging = true
	r.start, r.width, r.height = image.Pt(x, y), r.log.Width, r.log.Height
	r.Env.UI.Input.Captured = r
}
func (r *chatResize) MouseMove(_ byte, x, y int) {
	if r.dragging && !r.Blocked() {
		r.log.ResizeTo(r.width+x-r.start.X, r.height+r.start.Y-y)
	}
}
func (r *chatResize) CapturedMove(shift byte, x, y int) { r.MouseMove(shift, x, y) }
func (r *chatResize) LeftUp(shift byte, x, y int) {
	if r.dragging {
		r.MouseMove(shift, x, y)
		r.dragging = false
	}
	r.Component.LeftUp(shift, x, y)
}
func (r *chatResize) LeftUpOutside(shift byte, x, y int) { r.LeftUp(shift, x, y) }

func (r *chatResize) Paint() {
	row := 0
	if r.Env.UI.Input.Hovered == r {
		row = 1
	}
	if r.dragging {
		row = 2
	}
	r.Env.Pics.DrawRect(r.Env.Screen, r.Image, r.Rect().Min.X, r.Rect().Min.Y, image.Rect(0, row*chatResizeSize, chatResizeSize, (row+1)*chatResizeSize), true)
}

const chatDragBottom = 578 // native FUN_0048c32c's unlocked window clamp

// Update retains pointer capture while a drag crosses child controls.
func (l *ChatLog) Update(in *seui.Input) { l.Component.Update(in) }
func (l *ChatLog) LeftDown(shift byte, x, y int) {
	l.dragMoved = false
	l.FixedForm.LeftDown(shift, x, y)
	if l.Dragging {
		l.Env.UI.Input.Captured = l
	}
}
func (l *ChatLog) MouseMove(_ byte, x, y int) {
	if !l.Dragging || l.Locked || l.Mode != ChatOpaque || l.Blocked() {
		return
	}
	left := min(max(x-l.DragX, 0), max(chatScreenWidth-l.Width, 0))
	top := min(max(y-l.DragY, 0), max(chatDragBottom-l.Height, 0))
	if left != l.Left || top != l.Top {
		l.dragMoved = true
	}
	l.Left, l.Top = left, top
}
func (l *ChatLog) CapturedMove(shift byte, x, y int) { l.MouseMove(shift, x, y) }
func (l *ChatLog) LeftUp(shift byte, x, y int) {
	l.FixedForm.LeftUp(shift, x, y)
	l.dragMoved = false
}
func (l *ChatLog) LeftUpOutside(shift byte, x, y int) { l.LeftUp(shift, x, y) }

// drawBackgroundGrid ports FUN_0048d410's colour-keyed 32X32Grid tiling.
// The native fill starts at (25,8) and extends to width-15,height-1.
func (l *ChatLog) drawBackgroundGrid() {
	w, h := l.Env.Pics.Size(l.grid)
	if w <= 0 || h <= 0 {
		return
	}
	area := image.Rect(l.Left+25, l.Top+8, l.Left+l.Width-15, l.Top+l.Height-1)
	for y := area.Min.Y; y < area.Max.Y; y += h {
		for x := area.Min.X; x < area.Max.X; x += w {
			l.Env.Pics.DrawRect(l.Env.Screen, l.grid, x, y, image.Rect(0, 0, min(w, area.Max.X-x), min(h, area.Max.Y-y)), true)
		}
	}
}
func (l *ChatLog) tickerBytes() int {
	limit := min(len(l.ticker), chatTickerShown, max(l.listWidth()/chatCharWidth, 1))
	n := 0
	for n < limit {
		step := 1
		if l.ticker[n] >= 0x80 && n+1 < len(l.ticker) {
			step = 2
		}
		if n+step > limit {
			break
		}
		n += step
	}
	return n
}
