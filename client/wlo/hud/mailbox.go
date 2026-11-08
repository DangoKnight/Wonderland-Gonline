package hud

import "wonderland-gonline/internal/localization"

import (
	"image"
	"math"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

// Mail is one 0xb9-byte mailbox record (FUN_001d495c, FUN_001d5128).
type Mail struct {
	Kind   byte    `json:"kind"`   // +0: MailKindFriend … MailKindPet
	Read   bool    `json:"read"`   // +1
	Sender uint32  `json:"sender"` // +2 (the recipient in the sent box)
	Name   []byte  `json:"name"`   // +6, a 16-byte short string
	Text   []byte  `json:"text"`   // +0x18, a 152-byte short string
	Date   float64 `json:"date"`   // +0xb1, a TDateTime
}

// Mailbox record kinds and their labels (FUN_001d1294).
const (
	MailKindFriend = 1 // "Friends"
	MailKindGuild  = 2
	MailKindSystem = 3 // senders 100 and 101
	MailKindPet    = 4 // the player's +0x3ca4 partner
)

// Mailbox limits (FUN_001d495c): 1000 letters, the oldest dropped when
// full, a warning past 900; ten rows a page.
const (
	MailboxCapacity = 1000
	mailboxWarnAt   = 900
	mailboxRows     = 10
	mailNameBytes   = 16
)

// MailDate formats a TDateTime as FUN_001d1294 and FUN_003c58f4 do.
func MailDate(d float64) []byte {
	t := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC).Add(time.Duration(math.Round(d*86400e3)) * time.Millisecond)
	return []byte(t.Format("2006/01/02 15:04:05"))
}

// MailboxDate is the TDateTime of a time's wall clock.
func MailboxDate(t time.Time) float64 {
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	return float64(wall.Sub(time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)).Milliseconds()) / 86400e3
}

// Mailbox is PTR_DAT_004ca0fc (constructor FUN_001d0ca8, paint
// FUN_001d1294): Form_MailBox_1, 0x25a × 0x187, centred in the main area,
// with the Inbox and Sent tabs (Btn_MailBoxTab_1/2, FUN_001d56ac), ten rows
// a page and Send Mail, Delete, page and close buttons. A row's click
// selects and opens it (FUN_001d4130/FUN_001d415c → FUN_001d41ac): an
// inbox letter in the letter form, a sent one in the compose form, which
// Send Mail (0x1d4750) also shows. Not ported: the guild and pet letter
// forms.
type Mailbox struct {
	seui.Form
	CloseCorner, Close, SendMail, Delete *seui.FixedButton
	PrevPage, NextPage                   *seui.FixedButton
	Tabs                                 [2]*seui.FixedButton
	Inbox, Sent                          []Mail
	Tab                                  int // +0x150: 0 inbox, 1 sent
	Page                                 int // +0x154, from 1
	Selected                             int // +0x158, from 1 in the tab's list
	Unread                               int // +0x164
	hover, pressed                       int // +0x160, +0x15c
	title                                int
	// Pointer reports the mouse position for the row highlight.
	Pointer func() (int, int)
	// Open is FUN_001d41ac's showing of a letter in the read form.
	Open func(index int, m Mail)
	// Opened reports whether the read form shows that inbox letter.
	Opened func(index int) bool
	// CloseRead hides the read form.
	CloseRead func()
	// OpenSent is FUN_001d41ac for the Sent tab: the letter opens in the
	// compose form.
	OpenSent func(index int, m Mail)
	// Compose is Send Mail (0x1d4750): the compose form, empty.
	Compose func()
	Notice  func(string)
	Changed func()
}

const (
	mailboxWidth   = 0x25a
	mailboxHeight  = 0x187
	mailboxTitleX  = 0x11a
	mailboxTitleY  = 0x46
	mailboxRowX    = 0x13
	mailboxRowY    = 0x5f
	mailboxRowW    = 0x233
	mailboxRowStep = 0x14
	mailboxTextY   = 0x62
	mailboxKindX   = 0x24
	mailboxTextX   = 0x5a
	mailboxNameX   = 0x10e
	mailboxDateX   = 400
	mailboxPageX   = 0x12a
	mailboxPageY   = 0x140
	mailboxTextH   = 0x10
	mailboxCharW   = 8
	// The preview keeps the first line, cut below 18 bytes with "..."
	// past 20.
	mailboxPreviewMax = 0x15
	mailboxPreviewCut = 0x12

	mailUnreadInk   = 0xffe0
	mailOpenedInk   = 0xad60
	mailPageInk     = 0x1f
	mailPageFullInk = 0xf800
	// The row highlight is RGB(0x9e, 0xfa, 0xfe): alpha 0x32 under the
	// pointer, 0x50 for the selected letter.
	mailRowHighlight = 0xfefa9e
	mailHoverAlpha   = 0x32
	mailSelectAlpha  = 0x50

	mailboxAtLastPage   = "ui.hud.at_last_page_bfbff4ec"
	mailboxNoneSelected = "ui.hud.no_mail_selected_fbe80b70"
	mailboxEmpty        = "ui.hud.mailbox_is_empty_b176cf94"
	mailboxRemoved      = "ui.hud.letter_removed_2254f1d7"
	mailboxEmptied      = "ui.hud.mailbox_emptied_01574f51"
	mailboxNearlyFull   = "ui.hud.mailbox_volume_at_90_when_full_old_lette_a41cfd52"
	mailUnknownName     = "ui.hud.unknown_b764cdc0"
	mailPreviewMore     = "..."
)

var mailKindLabels = map[byte]string{MailKindFriend: localization.UI("ui.hud.friends_bd104d1b"), MailKindGuild: localization.UI("ui.hud.guild_298ffc49"), MailKindSystem: localization.UI("ui.hud.system_6725e7bb"), MailKindPet: localization.UI("ui.hud.pet_8f0d1b30")}

// The legacy Sent tab has "Backup" baked into its English picture. Keep its
// native border/states and replace the interior label with localized text.
type mailSentTab struct{ seui.FixedButton }

func (b *mailSentTab) Paint() {
	b.FixedButton.Paint()
	at := b.Abs()
	scr := b.Env.Screen
	label := []byte(localization.UI("ui.hud.mail_sent"))
	for y := at.Y + 3; y < at.Y+b.Height-3; y++ {
		x := at.X + 3
		if x >= 0 && x < scr.W && y >= 0 && y < scr.H {
			scr.Fill(image.Rect(at.X+4, y, at.X+b.Width-4, y+1), scr.Pix[y*scr.W+x])
		}
	}
	b.Env.Text.Draw(at.X+(b.Width-len(label)*8)/2, at.Y+3, 0, false, true, scr, label, 15, b.Width, 0, labelColor, 0)
}

// NewMailbox is FUN_001d0ca8.
func NewMailbox(env *seui.Env) *Mailbox {
	m := &Mailbox{Page: 1, title: env.Pics.Find("Icon_MailBoxTitleMash_1")}
	m.InitForm(m, env)
	m.Name = "TSe_MailBox"
	m.Init("Form_MailBox_1", (seui.MainAreaW-mailboxWidth)/2, 0, 0, 0, 0, false, mailboxHeight, mailboxWidth, (seui.MainAreaH-mailboxHeight)/2)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, m)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	m.CloseCorner = button("Btn_Close_S_1", 0x22a, 0x19, 0x12, 0x12, m.Hide)
	m.Close = button("Btn_Close_1", 0x16d, 0x15e, 0x38, 0x14, m.Hide)
	m.SendMail = button("Btn_sendMail_2", 0xb9, 0x15e, 0x38, 0x14, func() { call(m.Compose) })
	m.Delete = button("Btn_deleteMail_1", 0x113, 0x15e, 0x38, 0x14, m.deleteSelected)
	m.PrevPage = button("Btn_PrePage_1", 0xb9, 0x140, 0x38, 0x14, m.prevPage)
	m.NextPage = button("Btn_NextPage_1", 0x16d, 0x140, 0x38, 0x14, m.nextPage)
	for i, left := range [2]int{0xf, 0x4f} {
		tab := i
		if i == 1 {
			b := &mailSentTab{}
			b.InitFixedButton(b, env, m)
			b.Init("Btn_MailBoxTab_2", left, 0x15, 0x40, 0, 0, true, 0x15, 0x40, 0x30)
			b.OnClick = func() { m.SelectTab(tab) }
			m.Tabs[i] = &b.FixedButton
		} else {
			m.Tabs[i] = button("Btn_MailBoxTab_"+strconv.Itoa(i+1), left, 0x30, 0x40, 0x15, func() { m.SelectTab(tab) })
		}
	}
	m.SelectTab(0)
	return m
}

// SelectTab is FUN_001d56ac.
func (m *Mailbox) SelectTab(tab int) {
	m.Tab, m.Selected = tab, 0
	for i, b := range m.Tabs {
		b.Sticky = i == tab
	}
}

// Toggle is the chat bar's mail icon (FUN_002695d8).
func (m *Mailbox) Toggle() {
	if m.Visible {
		m.Hide()
		return
	}
	m.Show()
}

func (m *Mailbox) list() []Mail {
	if m.Tab == 1 {
		return m.Sent
	}
	return m.Inbox
}

// Pages is FUN_00034b64's ceiling of the letters over ten.
func (m *Mailbox) Pages() int { return (len(m.list()) + mailboxRows - 1) / mailboxRows }

// Receive is FUN_001d495c: a letter is filed newest first unless the same
// letter is already there; a full box drops its oldest letter.
func (m *Mailbox) Receive(l Mail) bool {
	at := 0
	for i, o := range m.Inbox {
		if o.Date == l.Date && o.Sender == l.Sender && string(o.Text) == string(l.Text) && string(o.Name) == string(l.Name) && o.Kind == l.Kind {
			return false
		}
		if l.Date < o.Date {
			at = i + 1
		}
	}
	if len(m.Inbox)+1 > MailboxCapacity-1 {
		m.RemoveInbox(len(m.Inbox), false)
		at = min(at, len(m.Inbox))
	}
	m.Inbox = append(m.Inbox[:at], append([]Mail{l}, m.Inbox[at:]...)...)
	if !l.Read {
		m.Unread++
	}
	if m.Tab == 0 && m.Selected > at {
		m.Selected++
	}
	if len(m.Inbox) > mailboxWarnAt && m.Notice != nil {
		m.Notice(localization.UI(mailboxNearlyFull))
	}
	m.changed()
	return true
}

// File is FUN_001d5128 for a sent letter, filed newest first and read.
func (m *Mailbox) File(l Mail) {
	l.Read = true
	at := 0
	for i, o := range m.Sent {
		if l.Date < o.Date {
			at = i + 1
		}
	}
	if len(m.Sent)+1 > MailboxCapacity-1 {
		m.Sent = m.Sent[:len(m.Sent)-1]
		at = min(at, len(m.Sent))
	}
	m.Sent = append(m.Sent[:at], append([]Mail{l}, m.Sent[at:]...)...)
	if m.Tab == 1 && m.Selected > at {
		m.Selected++
	}
	m.changed()
}

// RemoveInbox is FUN_001d4d48.
func (m *Mailbox) RemoveInbox(index int, report bool) bool {
	if len(m.Inbox) == 0 {
		m.notice(localization.UI(mailboxEmpty))
		return false
	}
	if index < 1 || index > len(m.Inbox) {
		return false
	}
	if !m.Inbox[index-1].Read && m.Unread > 0 {
		m.Unread--
	}
	m.Inbox = append(m.Inbox[:index-1], m.Inbox[index:]...)
	if m.Tab == 0 && index < m.Selected {
		m.Selected--
	}
	m.clampPage()
	if report {
		m.notice(localization.UI(mailboxRemoved))
		if len(m.Inbox) == 0 {
			m.notice(localization.UI(mailboxEmptied))
		}
	}
	m.changed()
	return true
}

// removeSent is FUN_001d54cc.
func (m *Mailbox) removeSent(index int) {
	if index < 1 || index > len(m.Sent) {
		return
	}
	m.Sent = append(m.Sent[:index-1], m.Sent[index:]...)
	if m.Tab == 1 && index < m.Selected {
		m.Selected--
	}
	m.clampPage()
	m.notice(localization.UI(mailboxRemoved))
	m.changed()
}

func (m *Mailbox) clampPage() {
	if p := m.Pages(); p < m.Page {
		m.Page = p
	}
	if m.Page == 0 {
		m.Page = 1
	}
}

// OpenInbox is FUN_001d41ac for the inbox: the open letter closes again;
// another is marked read and shown.
func (m *Mailbox) OpenInbox(index int) {
	if m.Opened != nil && m.Opened(m.Selected) {
		if m.CloseRead != nil {
			m.CloseRead()
		}
		m.Selected = 0
		return
	}
	if index < 1 || index > len(m.Inbox) {
		return
	}
	l := &m.Inbox[index-1]
	if !l.Read {
		l.Read = true
		if m.Unread > 0 {
			m.Unread--
		}
		m.changed()
	}
	m.Selected = index
	if m.Open != nil {
		m.Open(index, *l)
	}
}

// deleteSelected is 0x1d4760: the selected letter on the shown page goes,
// the read form closes and the letter now in its place opens.
func (m *Mailbox) deleteSelected() {
	if m.Selected == 0 {
		m.notice(localization.UI(mailboxNoneSelected))
		return
	}
	if (m.Selected+mailboxRows-1)/mailboxRows != m.Page {
		return
	}
	if m.Tab == 1 {
		m.removeSent(m.Selected)
		m.Selected = min(m.Selected, len(m.Sent))
		return
	}
	m.RemoveInbox(m.Selected, true)
	if m.CloseRead != nil {
		m.CloseRead()
	}
	if m.Selected == 0 {
		return
	}
	m.Selected = min(m.Selected, len(m.Inbox))
	m.OpenInbox(m.Selected)
}

// nextPage is FUN_001d48a4; prevPage is 0x1d494c.
func (m *Mailbox) nextPage() {
	if m.Page < m.Pages() {
		m.Page++
		return
	}
	m.notice(localization.UI(mailboxAtLastPage))
}

func (m *Mailbox) prevPage() {
	if m.Page > 1 {
		m.Page--
	}
}

func (m *Mailbox) notice(s string) {
	if m.Notice != nil {
		m.Notice(s)
	}
}

func (m *Mailbox) changed() {
	if m.Changed != nil {
		m.Changed()
	}
}

// rowsShown is the paint's row count for the page.
func (m *Mailbox) rowsShown() int {
	pages := max(m.Pages(), 1)
	if m.Page < pages {
		return mailboxRows
	}
	return len(m.list()) - (m.Page-1)*mailboxRows
}

func (m *Mailbox) rowRect(row int) image.Rectangle {
	at := m.Abs()
	y := at.Y + mailboxRowY + (row-1)*mailboxRowStep
	return image.Rect(at.X+mailboxRowX, y, at.X+mailboxRowX+mailboxRowW, y+mailboxRowStep)
}

// LeftDown is FUN_001d4130: the row under the pointer is pressed.
func (m *Mailbox) LeftDown(shift byte, x, y int) {
	m.Form.LeftDown(shift, x, y)
	m.pressed = m.hover
}

// LeftUp is FUN_001d415c: releasing on the pressed row selects and opens
// its letter.
func (m *Mailbox) LeftUp(shift byte, x, y int) {
	m.Form.LeftUp(shift, x, y)
	if m.pressed != 0 && m.pressed == m.hover {
		m.Selected = (m.Page-1)*mailboxRows + m.hover
		if m.Tab == 0 {
			m.OpenInbox(m.Selected)
		} else if m.Selected <= len(m.Sent) && m.OpenSent != nil {
			m.OpenSent(m.Selected, m.Sent[m.Selected-1])
		}
	}
	m.pressed = 0
}

// Paint is FUN_001d1294.
func (m *Mailbox) Paint() {
	m.Form.Paint()
	at := m.Abs()
	pics, scr, txt := m.Env.Pics, m.Env.Screen, m.Env.Text
	m.hover = 0
	if m.Tab == 0 && m.title >= 0 {
		pics.Draw(scr, m.title, at.X+mailboxTitleX, at.Y+mailboxTitleY, true)
	}
	list := m.list()
	if len(list) == 0 {
		m.Selected = 0
	}
	if m.Pointer != nil {
		px, py := m.Pointer()
		for row := 1; row <= m.rowsShown(); row++ {
			if image.Pt(px, py).In(m.rowRect(row)) {
				m.hover = row
				break
			}
		}
	}
	pages := m.Pages()
	if pages == 0 {
		m.Page, pages = 1, 1
	}
	ink := uint16(mailPageInk)
	if len(list) > mailboxWarnAt {
		ink = mailPageFullInk
	}
	draw := func(x, y int, s []byte, ink uint16) {
		txt.Draw(x, y, 0, false, true, scr, s, mailboxTextH, len(s)*mailboxCharW, 0, ink, 0)
	}
	page := []byte(strconv.Itoa(m.Page) + " ")
	draw(at.X+mailboxPageX-len(page)*mailboxCharW, at.Y+mailboxPageY, page, ink)
	draw(at.X+mailboxPageX, at.Y+mailboxPageY, []byte("/ "+strconv.Itoa(pages)), ink)
	for row := 1; row <= mailboxRows; row++ {
		i := (m.Page-1)*mailboxRows + row
		if i > len(list) {
			break
		}
		l := list[i-1]
		ink := uint16(mailUnreadInk)
		if l.Read {
			ink = mailOpenedInk
		}
		y := at.Y + mailboxTextY + (row-1)*mailboxRowStep
		label, ok := mailKindLabels[l.Kind]
		if !ok {
			label = localization.UI("ui.hud.normal_02ef42b2")
		}
		draw(at.X+mailboxKindX, y, []byte(label), ink)
		draw(at.X+mailboxTextX, y, MailPreview(l.Text), ink)
		name := l.Name
		if len(name) == 0 {
			name = []byte(localization.UI(mailUnknownName))
		}
		draw(at.X+mailboxNameX, y, name, ink)
		draw(at.X+mailboxDateX, y, MailDate(l.Date), ink)
	}
	if m.hover != 0 {
		scr.FillAlpha(m.rowRect(m.hover), mailRowHighlight, mailHoverAlpha)
	}
	if m.Selected != 0 && (m.Selected+mailboxRows-1)/mailboxRows == m.Page {
		scr.FillAlpha(m.rowRect(m.Selected-(m.Page-1)*mailboxRows), mailRowHighlight, mailSelectAlpha)
	}
}

// MailPreview is the row's text: its first line, or when that is 21 bytes
// or longer the letter's first characters below 18 bytes and "...".
func MailPreview(text []byte) []byte {
	line := text
	for i, c := range text {
		if c == '\r' {
			line = text[:i]
			break
		}
	}
	if len(line) < mailboxPreviewMax {
		return line
	}
	pos, last := 1, 1
	for pos < mailboxPreviewCut {
		cur := pos
		if len(text) <= cur {
			break
		}
		if text[cur-1] < 0x81 {
			pos = cur + 1
		} else {
			pos = cur + 2
		}
		last = cur
	}
	return append(append([]byte(nil), text[:last-1]...), mailPreviewMore...)
}

// MailName trims a name to the record's 16 bytes.
func MailName(name []byte) []byte {
	if len(name) > mailNameBytes {
		name = name[:mailNameBytes]
	}
	return append([]byte(nil), name...)
}

func init() {
	localization.Register(func() {
		mailKindLabels = map[byte]string{MailKindFriend: localization.UI("ui.hud.friends_bd104d1b"), MailKindGuild: localization.UI("ui.hud.guild_298ffc49"), MailKindSystem: localization.UI("ui.hud.system_6725e7bb"), MailKindPet: localization.UI("ui.hud.pet_8f0d1b30")}
	})
}
