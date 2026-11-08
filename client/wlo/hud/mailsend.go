package hud

import (
	"wonderland-gonline/client/wlo/seui"
)

// MailSend is TSe_SendMailForm (constructor FUN_003bfdec, paint
// FUN_003c2288, show FUN_003c1c3c; PTR_DAT_004ca5f8): Form_MailSend_1 at
// (10, 0x32), a memo for up to 152 bytes of text, Old Mail, clear, Send
// Mail, Close and a corner close. Mode 0 (+0x154) writes to one friend,
// whose name stands at (0x32, 0x43) and face at (0xbc, 0xf4) (FUN_003c4b4c:
// a player's face in action 2, the GM's in action 0); mode 2 writes to the
// friends ticked in the Social window and draws neither. Send (0x3c03c4)
// does nothing without text; the original sends one 14/1 for mode 0 and
// one listing every ticked friend for mode 2, which this port sends as
// one 14/1 each (the server takes one recipient a packet). Old Mail
// (FUN_003c5b40) opens the mailbox. Not ported: the "Banned to speak"
// check and the other modes (guild, invitations).
type MailSend struct {
	seui.Form
	Memo                    *seui.Memo        // +0x134
	CloseCorner             *seui.FixedButton // +0x13c
	Send, Old, Close, Clear *seui.FixedButton // +0x140, +0x14c, +0x144, +0x148
	Mode                    byte              // +0x154
	To                      Friend            // +0x138, +0x170
	Recipients              []Friend
	DrawPortrait            func(f Friend, x, y int)
	SendMail                func(to uint32, text []byte)
	// File keeps the sent letter in the mailbox's Sent tab (FUN_001d5128):
	// mode 0 with its recipient, mode 2 once without one.
	File  func(to uint32, name, text []byte)
	Sound func(name string)
}

const (
	MailToFriend  = 0
	MailToTicked  = 2
	MailMaxText   = 0x98
	mailTextColor = 0x0841 // Near-black RGB565; zero is the glyph transparency key.
	mailSendSound = `sound\wav0155.wav`
	mailNameX     = 0x32
	mailNameY     = 0x43
	mailNameH     = 0x10
	mailNameW     = 200
	mailFaceX     = 0xbc
	mailFaceY     = 0xf4
)

// NewMailSend is FUN_003bfdec.
func NewMailSend(env *seui.Env) *MailSend {
	m := &MailSend{}
	m.InitForm(m, env)
	m.Name = "TSe_SendMailForm"
	m.Init("Form_MailSend_1", 10, 0x163, 0xf9, 0, 0, true, 0x163, 0xf9, 0x32)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, m)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	m.CloseCorner = button("Btn_Close_s_1", 200, 0x17, 0x11, 0x11, m.Hide)
	m.Memo = seui.NewMemo(env, m)
	m.Memo.SetBounds(0x1e, 0x5a, 200, 0xbe)
	m.Memo.MaxLen = MailMaxText
	m.Memo.Color, m.Memo.Color2 = mailTextColor, 0
	m.Memo.TextStyle = 0
	m.Send = button("Btn_sendMail_2", 0xa7, 0x11a, 0x38, 0x14, m.send)
	m.Old = button("btn_PastMail_1", 0x2a, 0x11a, 0x38, 0x14, nil)
	m.Close = button("Btn_Close_1", 0x68, 0x13b, 0x38, 0x14, m.Hide)
	m.Clear = button("btn_Clear_1", 0x68, 0x11a, 0x38, 0x14, func() { m.Memo.SetText(nil) })
	return m
}

// Open shows the form for one friend (mode 0) or the ticked friends
// (mode 2), focusing the memo (FUN_003c1c3c).
func (m *MailSend) Open(mode byte, to Friend, recipients []Friend) {
	m.Mode, m.To, m.Recipients = mode, to, recipients
	m.Show()
	m.Env.UI.Focus(m.Memo)
}

// Paint is FUN_003c2288 for modes 0 and 2.
func (m *MailSend) Paint() {
	m.Form.Paint()
	if m.Mode != MailToFriend {
		return
	}
	at := m.Abs()
	m.Env.Text.Draw(at.X+mailNameX, at.Y+mailNameY, 0, false, true, m.Env.Screen, m.To.Name, mailNameH, mailNameW, 0, mailTextColor, 0)
	if m.DrawPortrait != nil {
		m.DrawPortrait(m.To, at.X+mailFaceX, at.Y+mailFaceY)
	}
}

// Deliver is 0x3c03c4 in mode 0 for another form's letter (the compose
// form, FUN_00193430).
func (m *MailSend) Deliver(to Friend, text []byte) {
	m.Mode, m.To = MailToFriend, to
	m.deliver(text)
}

// send is 0x3c03c4 for modes 0 and 2.
func (m *MailSend) send() {
	if len(m.Memo.Text) == 0 || m.SendMail == nil {
		return
	}
	m.deliver(m.Memo.Text)
	m.Hide()
}

func (m *MailSend) deliver(text []byte) {
	if len(text) == 0 || m.SendMail == nil {
		return
	}
	if m.Sound != nil {
		m.Sound(mailSendSound)
	}
	switch m.Mode {
	case MailToFriend:
		m.SendMail(m.To.ID, text)
		if m.File != nil {
			m.File(m.To.ID, m.To.Name, text)
		}
	case MailToTicked:
		for _, f := range m.Recipients {
			m.SendMail(f.ID, text)
		}
		m.Recipients = nil
		if m.File != nil {
			m.File(0, nil, text)
		}
	}
}
