package app

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"wonderland-gonline/client/wlo/hud"
)

// Letters (mailbox PTR_DAT_004ca0fc, read form PTR_DAT_004c9868). The
// original keeps the mailbox in user\Server<n>\MailBoxMsgLog<id ^ 0x79>
// \MailData.dat (FUN_001d36bc); the port keeps it in mail-<id>.json beside
// the settings.
const (
	// textMailHeader is 14/1's sub-command, sender and TDateTime; the text
	// is the rest of the packet (dispatcher case 0xe/1 at 0x2e29cc).
	textMailHeader = 1 + 4 + 8
	mailArrives    = `sound\wav0155.wav`
	// Senders 100 and 101 are the system (FUN_001d495c, FUN_001d4f40).
	mailSystemSender    = 100
	mailSystemSenderAlt = 101
	mailNotice          = 2000 // ms, the mailbox notices
)

type mailFile struct {
	Inbox []hud.Mail `json:"inbox"`
	Sent  []hud.Mail `json:"sent"`
}

func (c *Client) newMailbox() {
	b := hud.NewMailbox(c.Env)
	r := hud.NewMailRead(c.Env)
	b.Pointer = func() (int, int) { return c.Input.X, c.Input.Y }
	b.Notice = func(text string) { c.Notices.Show([]byte(text), mailNotice, c.Now()) }
	b.Changed = func() {
		c.syncMailIcon()
		c.saveMail()
	}
	b.Open = func(i int, m hud.Mail) {
		r.Load(i, m)
		r.Show()
		c.syncMailIcon()
	}
	b.Opened = func(i int) bool { return i != 0 && r.Visible && r.Index == i }
	b.CloseRead = r.Hide
	r.Count = func() int { return len(b.Inbox) }
	r.DrawPortrait = c.drawLetterPortrait
	// 0x3c146c and 0x3c5858: the next and previous letters.
	r.OnNext = func() {
		if r.Index+1 <= len(b.Inbox) {
			r.Hide()
			b.OpenInbox(r.Index + 1)
		}
	}
	r.OnPrev = func() {
		if r.Index-1 >= 1 {
			r.Hide()
			b.OpenInbox(r.Index - 1)
		}
	}
	// 0x3c5a80: the letter goes and the one now in its place opens.
	r.OnDelete = func() {
		b.RemoveInbox(r.Index, true)
		r.Hide()
		switch n := len(b.Inbox); {
		case n == 0:
		case n >= r.Index:
			b.OpenInbox(r.Index)
		default:
			b.OpenInbox(r.Index - 1)
		}
	}
	r.OnReply = func() { c.replyLetter(r) }
	c.Mailbox, c.MailRead = b, r
	c.UI.Add(b)
	c.UI.Add(r)
	w := hud.NewMailCompose(c.Env)
	w.Notice = b.Notice
	w.OldMail = func() {
		b.Show()
		w.Hide()
	}
	w.SendTo = func(name, text []byte) bool {
		f, ok := c.friendNamed(name)
		if ok && c.MailSend != nil {
			c.MailSend.Deliver(f, text)
		}
		return ok
	}
	b.Compose = func() { w.Open(nil, nil) }
	b.OpenSent = func(_ int, m hud.Mail) { w.Open(m.Name, m.Text) }
	c.MailCompose = w
	c.UI.Add(w)
	if c.MailSend != nil {
		// Old Mail (FUN_003c5b40) shows the mailbox and closes the form.
		c.MailSend.Old.OnClick = func() {
			b.Show()
			c.MailSend.Hide()
		}
		c.MailSend.File = func(to uint32, name, text []byte) {
			b.File(hud.Mail{Kind: hud.MailKindFriend, Sender: to, Name: hud.MailName(name), Text: append([]byte(nil), text...), Date: hud.MailboxDate(c.Now())})
		}
	}
}

// syncMailIcon is FUN_00267c4c / FUN_00267c54: the chat bar's icon
// blinks while letters are unread.
func (c *Client) syncMailIcon() {
	if c.ChatBar != nil && c.Mailbox != nil {
		c.ChatBar.MailUnread = c.Mailbox.Unread > 0
	}
}

// receiveLetter is 14/1 (case 0xe/1 at 0x2e29cc, FUN_003c2360 and
// FUN_001d4f40): the letter is filed under Friends, or System for senders
// 100 and 101, named from the friend list or the players around; the
// arrival sound plays and the mail icon blinks.
func (c *Client) receiveLetter(p []byte) bool {
	if len(p) < textMailHeader || c.Mailbox == nil {
		return false
	}
	sender := binary.LittleEndian.Uint32(p[1:])
	date := math.Float64frombits(binary.LittleEndian.Uint64(p[5:]))
	if sender == mailSystemSenderAlt {
		sender = mailSystemSender
	}
	l := hud.Mail{Kind: hud.MailKindFriend, Sender: sender, Text: append([]byte(nil), p[textMailHeader:]...), Date: date}
	if len(l.Text) > hud.MailMaxText {
		l.Text = l.Text[:hud.MailMaxText]
	}
	if sender == mailSystemSender {
		l.Kind, l.Name = hud.MailKindSystem, hud.MailName(hud.GMFriendName)
	} else if f, ok := c.friend(sender); ok {
		l.Name = hud.MailName(f.Name)
	} else if c.World != nil {
		l.Name = hud.MailName(c.World.PeerName(sender))
	}
	c.Mailbox.Receive(l)
	c.playSound(mailArrives)
	c.syncMailIcon()
	return true
}

// friendNamed is FUN_00193430's search of the friend list by name, the
// GM entry excepted.
func (c *Client) friendNamed(name []byte) (hud.Friend, bool) {
	if c.Social != nil {
		for _, f := range c.Social.Friends {
			if f.ID != hud.GMFriendID && string(f.Name) == string(name) {
				return f, true
			}
		}
	}
	return hud.Friend{}, false
}

func (c *Client) friend(id uint32) (hud.Friend, bool) {
	if c.Social != nil {
		for _, f := range c.Social.Friends {
			if f.ID == id {
				return f, true
			}
		}
	}
	return hud.Friend{}, false
}

// drawLetterPortrait is FUN_003c4370's face: the system's cupid, or the
// sender's face when they are a friend.
func (c *Client) drawLetterPortrait(l hud.Mail, x, y int) {
	if f, ok := c.friend(l.Sender); ok {
		c.drawMailPortrait(f, x, y)
	}
}

// replyLetter is FUN_003c12a4: the send form opens where the letter was,
// addressed to its sender, and the letter closes. The original sends
// replies to non-friends through the compose-by-name form, which is not
// ported; those use the same form.
func (c *Client) replyLetter(r *hud.MailRead) {
	if c.MailSend == nil {
		return
	}
	to, ok := c.friend(r.Mail.Sender)
	if !ok {
		to = hud.Friend{ID: r.Mail.Sender, Name: r.Mail.Name}
	}
	c.MailSend.Left, c.MailSend.Top = r.Left, r.Top
	c.MailSend.Open(hud.MailToFriend, to, nil)
	r.Hide()
}

func (c *Client) mailPath() string {
	if c.settingsPath == "" || c.World == nil {
		return ""
	}
	return filepath.Join(filepath.Dir(c.settingsPath), fmt.Sprintf("mail-%d.json", c.World.Player.ID))
}

// loadMail reads the character's mailbox at world entry.
func (c *Client) loadMail() {
	if c.Mailbox == nil {
		return
	}
	var f mailFile
	if p := c.mailPath(); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			if err := json.Unmarshal(b, &f); err != nil {
				c.Chat.Notice("Could not load mail: " + err.Error())
				f = mailFile{}
			}
		}
	}
	b := c.Mailbox
	b.Inbox, b.Sent, b.Unread, b.Page = f.Inbox, f.Sent, 0, 1
	b.SelectTab(0)
	for _, l := range b.Inbox {
		if !l.Read {
			b.Unread++
		}
	}
	c.syncMailIcon()
}

func (c *Client) saveMail() {
	p := c.mailPath()
	if p == "" || c.Mailbox == nil {
		return
	}
	b, err := json.Marshal(mailFile{Inbox: c.Mailbox.Inbox, Sent: c.Mailbox.Sent})
	if err == nil {
		err = os.MkdirAll(filepath.Dir(p), 0o700)
	}
	if err == nil {
		err = os.WriteFile(p, b, 0o600)
	}
	if err != nil {
		c.Chat.Notice("Could not save mail: " + err.Error())
	}
}
