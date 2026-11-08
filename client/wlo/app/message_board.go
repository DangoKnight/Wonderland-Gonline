package app

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/localization"
	"wonderland-gonline/internal/protocol"
)

const (
	boardWidth            = 654
	boardHeight           = 365
	boardRowHeight        = 20
	boardListLeft         = 15
	boardListTop          = 77
	boardListWidth        = 620
	boardGreen            = 0x0200 // RGB565 dark green, matching the native notice background.
	boardRequestTimeout   = 5 * time.Second
	boardMaxStoredContent = 2048 // Accept existing server records; new posts use the native 151-byte cap.
)

type boardEntry struct {
	id, author  uint32
	name, title []byte
	date        float64
}
type messageBoardForm struct {
	seui.Form
	c               *Client
	entries         []boardEntry
	page            byte
	total           uint32
	selected        boardEntry
	pendingRead     uint32
	pending         time.Time
	detail, compose *boardTextForm
}
type boardTextForm struct {
	seui.Form
	board                 *messageBoardForm
	title                 *seui.Editor
	body                  *seui.Memo
	remove, mail, confirm *seui.FixedButton
	editing               bool
}
type boardRow struct {
	seui.Component
	board *messageBoardForm
	index int
}

func (r *boardRow) Paint() {}
func (r *boardRow) LeftUp(_ byte, _, _ int) {
	b := r.board
	if r.index >= len(b.entries) || !b.pending.IsZero() {
		return
	}
	b.selected = b.entries[r.index]
	b.pendingRead = b.selected.id
	b.pending = b.c.Now()
	b.c.Net.Send(b.entryRequest(protocol.MessageBoardRead))
}
func boardButton(c *Client, parent seui.Control, pic string, x, y int, action func()) *seui.FixedButton {
	b := seui.NewFixedButton(c.Env, parent)
	b.Name = pic
	b.Init(pic, x, 20, 56, 0, 0, true, 20, 56, y)
	b.OnClick = action
	return b
}
func (c *Client) initMessageBoard() {
	b := &messageBoardForm{c: c, page: 1}
	b.InitForm(b, c.Env)
	b.Name = "PublicNoticeBoard"
	b.Init("Notice", (ScreenWidth-boardWidth)/2, 0, 0, 0, 0, false, boardHeight, boardWidth, (ScreenHeight-boardHeight)/2)
	b.Dockable = false
	c.messageBoard = b
	boardButton(c, b, "Btn_Close_S_1", 603, 24, c.closeMessageBoard).Init("Btn_Close_S_1", 603, 18, 18, 0, 0, true, 18, 18, 24)
	boardButton(c, b, "Btn_Close_1", 380, 320, c.closeMessageBoard)
	boardButton(c, b, "Btn_UPdata_1", 290, 320, func() { b.requestPage(b.page) })
	boardButton(c, b, "Btn_PrePage_1", 200, 290, func() {
		if b.page > 1 {
			b.requestPage(b.page - 1)
		}
	})
	boardButton(c, b, "Btn_NextPage_1", 380, 290, func() {
		if uint32(b.page) < b.pages() {
			b.requestPage(b.page + 1)
		}
	})
	boardButton(c, b, "Btn_AddNew_1", 200, 320, func() {
		if !b.pending.IsZero() {
			return
		}
		b.compose.title.SetText(nil)
		b.compose.body.SetText(nil)
		b.compose.Show()
		c.Input.Focused = b.compose.title
	})
	for i := 0; i < protocol.MessageBoardPageSize; i++ {
		r := &boardRow{board: b, index: i}
		r.InitComponent(r, c.Env, b)
		r.Left, r.Top, r.Width, r.Height = boardListLeft, boardListTop+i*boardRowHeight, boardListWidth, boardRowHeight
		r.Visible = true
	}
	c.UI.Add(b)
	b.detail = c.newBoardText(b, false)
	b.compose = c.newBoardText(b, true)
}
func (c *Client) newBoardText(b *messageBoardForm, editing bool) *boardTextForm {
	f := &boardTextForm{board: b, editing: editing}
	f.InitForm(f, c.Env)
	f.Dockable = false
	f.Init("notice2", (ScreenWidth-418)/2, 0, 0, 0, 0, false, 275, 418, (ScreenHeight-275)/2)
	f.title = seui.NewEditor(c.Env, f)
	f.title.Init("", 23, 0, 0, 0, 0, false, 24, 372, 62)
	f.title.ReadOnly = !editing
	f.title.Color = 0xffff
	f.title.Color3 = 0xffff
	f.title.Limit = true
	f.title.MaxLen = protocol.MessageBoardTitleMaxBytes
	f.title.BindLanguage("label", func() { f.title.SetLabel([]byte(localization.UI("ui.board.subject"))) })
	f.body = seui.NewMemo(c.Env, f)
	f.body.SetBounds(25, 101, 372, 100)
	f.body.Color = 0xffff
	f.body.ReadOnly = !editing
	f.body.MaxLen = protocol.MessageBoardContentMaxBytes
	if !editing {
		f.body.MaxLen = boardMaxStoredContent
	}
	boardButton(c, f, "Btn_Close_S_1", 368, 24, f.Hide).Init("Btn_Close_S_1", 368, 18, 18, 0, 0, true, 18, 18, 24)
	boardButton(c, f, "Btn_Close_1", 270, 228, f.Hide)
	if editing {
		f.confirm = boardButton(c, f, "Btn_confirm_1", 130, 228, func() {
			if !b.pending.IsZero() {
				return
			}
			if len(bytes.TrimSpace(f.title.Text)) == 0 || len(bytes.TrimSpace(f.body.Text)) == 0 {
				c.boardNotice("ui.board.enter_text")
				return
			}
			b.pending = c.Now()
			f.confirm.Enabled = false
			c.Net.Send(protocol.Builder{protocol.CommandMessageBoard, protocol.MessageBoardPost}.U32(uint32(len(f.title.Text))).Bytes(f.title.Text).U32(uint32(len(f.body.Text))).Bytes(f.body.Text).U8(protocol.MessageBoardWireContext1))
		})
	} else {
		f.remove = boardButton(c, f, "Btn_deleteMail_1", 90, 228, func() {
			if !b.pending.IsZero() || c.World == nil || b.selected.author != c.World.Player.ID {
				return
			}
			b.pending = c.Now()
			f.remove.Enabled = false
			c.Net.Send(b.entryRequest(protocol.MessageBoardDelete))
		})
		f.mail = boardButton(c, f, "Btn_sendMail_2", 180, 228, func() {
			if c.MailSend != nil && b.selected.author != 0 {
				c.MailSend.Open(hud.MailToFriend, hud.Friend{ID: b.selected.author, Name: b.selected.name}, nil)
			}
		})
	}
	c.UI.Add(f)
	return f
}
func (b *messageBoardForm) pages() uint32 {
	if b.total == 0 {
		return 1
	}
	return min(255, 1+(b.total-1)/protocol.MessageBoardPageSize)
}
func (b *messageBoardForm) requestPage(page byte) {
	if !b.pending.IsZero() {
		return
	}
	b.pendingRead = 0
	b.pending = b.c.Now()
	b.c.Net.Send([]byte{protocol.CommandMessageBoard, protocol.MessageBoardPage, page, protocol.MessageBoardWireContext1})
}

func (b *messageBoardForm) entryRequest(code byte) []byte {
	return protocol.Builder{protocol.CommandMessageBoard, code}.U32(b.selected.id).U32(uint32(b.page)).F64(b.selected.date).U8(protocol.MessageBoardWireContext1)
}
func (c *Client) closeMessageBoard() {
	if b := c.messageBoard; b != nil {
		b.Hide()
		b.detail.Hide()
		b.compose.Hide()
		b.pending = time.Time{}
		b.pendingRead = 0
		b.entries = nil
	}
}
func (c *Client) boardNotice(key string) {
	c.Notices.Show([]byte(localization.UI(key)), serviceNoticeDuration, c.Now())
}
func (c *Client) messageBoardTick() {
	if b := c.messageBoard; b != nil && !b.pending.IsZero() && c.Now().Sub(b.pending) >= boardRequestTimeout {
		b.pending = time.Time{}
		b.pendingRead = 0
		b.compose.confirm.Enabled = true
		c.boardNotice("ui.board.timeout")
	}
}
func (b *messageBoardForm) KeyDown(key uint16, shift byte) {
	if key == escapeKey {
		b.c.closeMessageBoard()
		return
	}
	b.Form.KeyDown(key, shift)
}
func (f *boardTextForm) KeyDown(key uint16, shift byte) {
	if key == escapeKey {
		f.Hide()
		return
	}
	f.Form.KeyDown(key, shift)
}
func (b *messageBoardForm) Paint() {
	b.Form.Paint()
	at := b.Abs()
	scr := b.c.Screen
	list := image.Rect(at.X+15, at.Y+77, at.X+635, at.Y+277)
	scr.Fill(list, boardGreen)
	for _, x := range []int{265, 387} {
		scr.Fill(image.Rect(at.X+x, list.Min.Y, at.X+x+1, list.Max.Y), 0xffff)
	}
	for i, e := range b.entries {
		y := at.Y + 79 + i*boardRowHeight
		for _, v := range []struct {
			x, w int
			text []byte
		}{{19, 242, e.title}, {269, 114, e.name}, {390, 240, []byte(boardDate(e.date))}} {
			b.c.Env.Text.Draw(at.X+v.x, y, 0, false, true, scr, v.text, 0, v.w, 0, 0xffe0, 2)
		}
	}
	text := []byte(fmt.Sprintf("%d / %d", b.page, b.pages()))
	b.c.Env.Text.Draw(at.X+293, at.Y+294, 0, false, true, scr, text, 0, 82, 0, 0x001f, 2)
}
func (f *boardTextForm) Paint() {
	f.Form.Paint()
	at := f.Abs()
	f.board.c.Screen.Fill(image.Rect(at.X+15, at.Y+54, at.X+402, at.Y+207), boardGreen)
}
func boardDate(date float64) string {
	if math.IsNaN(date) || math.IsInf(date, 0) || date < 0 || date > 3000000 {
		return ""
	}
	return time.UnixMilli(int64((date - 25569) * 86400000)).UTC().Format("2006/01/02 15:04:05")
}
func readBoardText(r *protocol.Reader, maxLen uint32) []byte {
	n := r.U32()
	if n > maxLen {
		r.Bytes(r.Remaining() + 1)
		return nil
	}
	return append([]byte(nil), r.Bytes(int(n))...)
}
func (c *Client) messageBoardPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandMessageBoard {
		return false
	}
	b := c.messageBoard
	if b == nil {
		return true
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.MessageBoardOpen:
		if len(p) != 2 || c.World == nil {
			return true
		}
		c.loadPictures("Notice", "notice2", "Btn_Close_S_1", "Btn_Close_1", "Btn_UPdata_1", "Btn_PrePage_1", "Btn_NextPage_1", "Btn_AddNew_1", "Btn_deleteMail_1", "Btn_sendMail_2", "Btn_Del_B", "btn_Send_B", "Btn_confirm_1")
		// The controls are created before these optional pictures are loaded.
		c.reloadBoardPictures()
		c.resetGroundHold()
		c.World.StopWalk()
		b.entries = nil
		b.total = 0
		b.page = 1
		b.pending = time.Time{}
		b.detail.Hide()
		b.compose.Hide()
		b.Show()
		b.requestPage(1)
	case protocol.MessageBoardEntries:
		var entries []boardEntry
		for r.Remaining() > 0 && len(entries) < protocol.MessageBoardPageSize {
			e := boardEntry{name: readBoardText(r, protocol.MessageBoardAuthorMaxBytes), title: readBoardText(r, protocol.MessageBoardTitleMaxBytes)}
			e.date = r.F64()
			e.author = r.U32()
			e.id = r.U32()
			entries = append(entries, e)
		}
		if r.Err() != nil || r.Remaining() != 0 {
			return true
		}
		if !b.Visible {
			return true
		}
		b.entries = entries
		b.pending = time.Time{}
		b.pendingRead = 0
		b.compose.confirm.Enabled = true
	case protocol.MessageBoardRead, protocol.MessageBoardPagination:
		count, page := r.U32(), r.U8()
		if r.Err() != nil || r.Remaining() != 0 || page == 0 || !b.Visible {
			return true
		}
		b.total, b.page = count, page
		b.compose.Hide()
		b.detail.Hide()
	case protocol.MessageBoardPosted:
		if len(p) != 3 || p[2] != protocol.MessageBoardWireContext1 || !b.Visible {
			return true
		}
		b.pending = time.Time{}
		b.compose.confirm.Enabled = true
		b.compose.Hide()
	case protocol.MessageBoardPage:
		e := boardEntry{name: readBoardText(r, protocol.MessageBoardAuthorMaxBytes), title: readBoardText(r, protocol.MessageBoardTitleMaxBytes)}
		e.date = r.F64()
		n := r.U32()
		e.author = r.U32()
		if n > boardMaxStoredContent {
			return true
		}
		content := r.Bytes(int(n))
		if r.Err() != nil || r.Remaining() != 0 || !b.Visible || b.pendingRead == 0 {
			return true
		}
		e.id = b.pendingRead
		b.selected = e
		b.pendingRead = 0
		b.pending = time.Time{}
		b.detail.title.SetText(e.title)
		b.detail.body.SetText(content)
		b.detail.remove.Enabled = c.World != nil && e.author == c.World.Player.ID
		b.detail.mail.Enabled = c.World != nil && e.author != c.World.Player.ID
		for _, v := range []struct {
			button   *seui.FixedButton
			disabled string
		}{{b.detail.remove, "Btn_Del_B"}, {b.detail.mail, "btn_Send_B"}} {
			pic := v.button.Name
			if !v.button.Enabled {
				pic = v.disabled
			}
			if index := c.Pics.Find(pic); index >= 0 {
				v.button.Image = index
			}
		}
		b.detail.Show()
	case protocol.MessageBoardAuthorLimit, protocol.MessageBoardFull, protocol.MessageBoardMissing:
		if len(p) != 2 || !b.Visible {
			return true
		}
		b.pending = time.Time{}
		b.pendingRead = 0
		b.compose.confirm.Enabled = true
		key := map[byte]string{5: "ui.board.author_limit", 6: "ui.board.full", 7: "ui.board.missing"}[p[1]]
		c.boardNotice(key)
	}
	return true
}
func (c *Client) reloadBoardPictures() {
	b := c.messageBoard
	for _, f := range []*seui.Form{&b.Form, &b.detail.Form, &b.compose.Form} {
		pic := "notice2"
		if f == &b.Form {
			pic = "Notice"
		}
		f.Image = c.Pics.Find(pic)
	}
	// Reload picture handles without rebuilding callbacks or form state.
	var walk func(seui.Control)
	walk = func(control seui.Control) {
		if button, ok := control.(*seui.FixedButton); ok {
			button.Image = c.Pics.Find(button.Name)
		}
		for _, child := range control.Base().Children {
			walk(child)
		}
	}
	walk(b)
	walk(b.detail)
	walk(b.compose)
}
