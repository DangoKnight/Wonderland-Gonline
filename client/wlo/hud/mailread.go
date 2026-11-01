package hud

import (
	"bytes"

	"wonderland-gonline/client/wlo/seui"
)

// MailRead is the friend letter form PTR_DAT_004c9868 (TSe_ReceiveMailForm,
// constructor FUN_003c0dc0 with +0x28651 = 0, paint FUN_003c151c and
// FUN_003c4370, load FUN_003c58f4): form_MailReceive_1 at (10, 0x32) with
// the letter's date at (0x1d, 0x32), the sender's name at (0x4c, 0x43),
// their face at (0xbc, 0xf4), the text in a read-only green memo, Reply,
// Delete, Close, a corner close and the previous and next letters (their
// grey panels and icons stand in while the buttons are hidden; the icons
// are missing from the client's pictures). The guild form
// (PTR_DAT_004ca8e8, +0x28651 = 1) is not ported.
type MailRead struct {
	seui.FixedForm
	Memo                              *seui.Memo        // +0x2861c
	CloseCorner, Reply, Close         *seui.FixedButton // +0x28628, +0x28624, +0x2862c
	Delete, Next, Prev                *seui.FixedButton // +0x28638, +0x28630, +0x28634
	nextIcon, prevIcon, invitation    int               // +0x286fc, +0x28700, +0x28708
	Index                             int               // +0x2871c, the inbox letter
	Mail                              Mail
	date                              []byte // +0x2864c
	Invitation                        bool   // +0x28705
	Count                             func() int
	DrawPortrait                      func(m Mail, x, y int)
	OnReply, OnDelete, OnNext, OnPrev func()
}

const (
	mailReadWidth  = 0xfd
	mailReadHeight = 0x173
	mailReadDateX  = 0x1d
	mailReadDateY  = 0x32
	mailReadNameX  = 0x4c
	mailReadNameY  = 0x43
	mailReadFaceX  = 0xbc
	mailReadFaceY  = 0xf4
	mailReadBadgeX = 0x13
	mailReadBadgeY = 0x38
	mailReadInk    = 0x7e0
	// mailInvitation marks an invitation letter (FUN_003c58f4, the
	// short string at 0x3c5a28); it is cut from the text shown.
	mailInvitation = "^inviation^"
)

// NewMailRead is FUN_003c0dc0.
func NewMailRead(env *seui.Env) *MailRead {
	m := &MailRead{
		nextIcon:   env.Pics.Find("icon_NextMail_1"),
		prevIcon:   env.Pics.Find("icon_PrevMail_1"),
		invitation: env.Pics.Find("form_invitation2"),
	}
	m.InitFixedForm(m, env)
	m.Name = "TSe_ReceiveMailForm"
	m.Init("form_MailReceive_1", 10, mailReadHeight, mailReadWidth, 0, 0, true, mailReadHeight, mailReadWidth, 0x32)
	button := func(picture string, left, top, w, h int, click func()) *seui.FixedButton {
		b := seui.NewFixedButton(env, m)
		b.Init(picture, left, h, w, 0, 0, true, h, w, top)
		b.OnClick = click
		return b
	}
	m.CloseCorner = button("Btn_Close_s_1", 0xcb, 0x17, 0x11, 0x11, m.Hide)
	m.Memo = seui.NewMemo(env, m)
	m.Memo.SetBounds(0x1c, 0x58, 200, 0xbe)
	m.Memo.MaxLen = MailMaxText
	m.Memo.Color, m.Memo.Color2 = mailReadInk, 0
	m.Memo.TextStyle = 0
	m.Memo.ReadOnly = true
	m.Reply = button("btn_reply_1", 0x65, 0x11d, 0x38, 0x14, func() { call(m.OnReply) })
	m.Close = button("Btn_Close_1", 0x65, 0x14b, 0x38, 0x14, m.Hide)
	m.Delete = button("btn_deleteMail_1", 0xa4, 0x11d, 0x38, 0x14, func() { call(m.OnDelete) })
	m.Next = button("btn_NextMail_1", 0x24, 0x136, 0x38, 0x14, func() { call(m.OnNext) })
	m.Prev = button("btn_PrevMail_1", 0x24, 0x11d, 0x38, 0x14, func() { call(m.OnPrev) })
	// The grey panels lie under the buttons and show while they hide
	// (btn_PrevMail_Gray_1 is missing from the client's pictures).
	for _, g := range []struct {
		picture string
		top     int
	}{{"btn_NextMail_Gray_1", 0x136}, {"btn_PrevMail_Gray_1", 0x11d}} {
		p := seui.NewPanel(env, m)
		p.Init(g.picture, 0x24, 0x14, 0x38, 0, 0, true, 0x14, 0x38, g.top)
		p.Hover = false
	}
	return m
}

func call(f func()) {
	if f != nil {
		f()
	}
}

// Load is FUN_003c58f4.
func (m *MailRead) Load(index int, l Mail) {
	m.Index, m.Mail = index, l
	text := l.Text
	m.Invitation = bytes.HasPrefix(text, []byte(mailInvitation))
	if m.Invitation {
		text = text[len(mailInvitation):]
	}
	m.Memo.SetText(text)
	m.date = MailDate(l.Date)
}

// Paint is FUN_003c151c with FUN_003c4370.
func (m *MailRead) Paint() {
	count := 0
	if m.Count != nil {
		count = m.Count()
	}
	m.Next.SetVisible(m.Index < count)
	m.Prev.SetVisible(m.Index >= 2)
	m.FixedForm.Paint()
	at := m.Abs()
	pics, scr, txt := m.Env.Pics, m.Env.Screen, m.Env.Text
	txt.Draw(at.X+mailReadDateX, at.Y+mailReadDateY, 0, false, true, scr, m.date, mailNameH, mailNameW, 0, m.Color, 0)
	name := m.Mail.Name
	if len(name) == 0 {
		name = []byte(mailUnknownName)
	}
	txt.Draw(at.X+mailReadNameX, at.Y+mailReadNameY, 0, false, true, scr, name, mailNameH, mailNameW, 0, m.Color, 0)
	if m.DrawPortrait != nil {
		m.DrawPortrait(m.Mail, at.X+mailReadFaceX, at.Y+mailReadFaceY)
	}
	if m.Invitation && m.invitation >= 0 {
		pics.Draw(scr, m.invitation, at.X+mailReadBadgeX, at.Y+mailReadBadgeY, true)
	}
	if !m.Next.Visible && m.nextIcon >= 0 {
		pics.Draw(scr, m.nextIcon, at.X+m.Next.Left, at.Y+m.Next.Top, true)
	}
	if !m.Prev.Visible && m.prevIcon >= 0 {
		pics.Draw(scr, m.prevIcon, at.X+m.Prev.Left, at.Y+m.Prev.Top, true)
	}
}
