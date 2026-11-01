package hud

import "wonderland-gonline/client/wlo/seui"

// MailCompose is the letter form addressed by name, PTR_DAT_004ca6c8
// (constructor FUN_001947d8, send FUN_00193430): Form_MailBox at (0xe6,
// 0x1e) with a blue name field (10 bytes), a 152-byte memo, Send Mail,
// Old Mail, Clear, Close and a corner close. The mailbox's Send Mail shows
// it empty; a sent letter opens in it (FUN_001d41ac for the Sent tab).
// Send checks the name ("No target player") and text ("No mail content"),
// closes the form and, for a friend's name, sends through the friend form
// (0x3c03c4, mode 0). Letters to other players go as 14/6 with a Mailbox
// item; that path is not ported and answers "Need Mailbox to send", as the
// original does without the item.
type MailCompose struct {
	seui.Form
	To                      *seui.Editor      // +0x13c
	Memo                    *seui.Memo        // +0x130
	CloseCorner             *seui.FixedButton // +0x140
	Send, Old, Close, Clear *seui.FixedButton // +0x144, +0x150, +0x148, +0x14c
	// SendTo sends to a friend by name; false when the name is no friend.
	SendTo  func(name, text []byte) bool
	OldMail func()
	Notice  func(string)
}

const (
	mailComposeNameInk = 0x1f
	mailComposeNameMax = 10
	mailComposeInk     = 0x0841
	mailNoTarget       = "No target player"
	mailNoContent      = "No mail content"
	mailNeedMailbox    = "Need Mailbox to send"
)

// NewMailCompose is FUN_001947d8.
func NewMailCompose(env *seui.Env) *MailCompose {
	m := &MailCompose{}
	m.InitForm(m, env)
	m.Name = "TSe_MailComposeForm"
	m.Init("Form_MailBox", 0xe6, 0x173, 0xfd, 0, 0, true, 0x173, 0xfd, 0x1e)
	m.SetMargins(0x4b, 0x5a, 0x35, 0x50)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, m)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	m.CloseCorner = button("Btn_Close_s_1", 0xcb, 0x19, 0x11, 0x11, m.Hide)
	m.To = seui.NewEditor(env, m)
	m.To.Init("", 0x33, 0x19, 0xa0, 0, 0, true, 0x19, 0xa0, 0x3d)
	m.To.SetColor(mailComposeNameInk)
	m.To.SetMaxLen(mailComposeNameMax)
	m.To.AllowPaste = true
	m.Memo = seui.NewMemo(env, m)
	m.Memo.SetBounds(0x1e, 0x5d, 200, 200)
	// The constructor sets ink 0, which equals the paper and vanishes in
	// this renderer's transparent glyphs; compatibility: the memo's own
	// near-black default stands in (no capture shows this form).
	m.Memo.Color, m.Memo.Color2 = mailComposeInk, 0
	m.Memo.TextStyle = 0
	m.Memo.MaxLen = MailMaxText
	m.Send = button("Btn_sendMail_2", 0xa2, 0x12f, 0x38, 0x14, m.send)
	m.Old = button("btn_PastMail_1", 0x23, 0x12f, 0x38, 0x14, func() { call(m.OldMail) })
	m.Close = button("Btn_Close_1", 0x61, 0x14d, 0x38, 0x14, m.Hide)
	m.Clear = button("btn_Clear_1", 0x61, 0x12f, 0x38, 0x14, func() { m.Memo.SetText(nil) })
	return m
}

// Open shows the form with a name and text (empty for a new letter).
func (m *MailCompose) Open(name, text []byte) {
	m.To.SetText(name)
	m.Memo.SetText(text)
	m.Show()
	m.Env.UI.Focus(m.To)
}

// send is FUN_00193430.
func (m *MailCompose) send() {
	notice := func(s string) {
		if m.Notice != nil {
			m.Notice(s)
		}
	}
	switch {
	case len(m.To.Text) == 0:
		notice(mailNoTarget)
	case len(m.Memo.Text) == 0:
		notice(mailNoContent)
	case m.SendTo == nil || !m.SendTo(m.To.Text, m.Memo.Text):
		notice(mailNeedMailbox)
	default:
		m.Hide()
		m.Memo.SetText(nil)
	}
}
