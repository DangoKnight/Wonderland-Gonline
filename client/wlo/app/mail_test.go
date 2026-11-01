package app

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/protocol"
)

// letterPacket is 14/1 as aLogin reads it: sender, TDateTime, then text.
func letterPacket(sender uint32, date float64, text string) []byte {
	p := binary.LittleEndian.AppendUint32([]byte{protocol.CommandFriends, protocol.FriendsTextMail}, sender)
	p = binary.LittleEndian.AppendUint64(p, math.Float64bits(date))
	return append(p, text...)
}

func mailClient(t *testing.T) (*Client, *time.Time) {
	t.Helper()
	c, now, _ := enteredClient(t)
	c.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	p := []byte{protocol.CommandFriends, protocol.FriendsList}
	p = append(p, friendRecord(20001, "Pal", 3)...)
	c.dispatch(p)
	return c, now
}

// TestLetterArrives: a friend's letter is filed and saved, the mail icon
// blinks, the icon opens the mailbox, a row opens the letter (read, the
// blinking stops) and Reply addresses the send form to the sender.
func TestLetterArrives(t *testing.T) {
	c, now := mailClient(t)
	date := hud.MailboxDate(time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC))
	c.dispatch(letterPacket(20001, date, "hello\rthere"))
	b, r := c.Mailbox, c.MailRead
	if len(b.Inbox) != 1 || b.Unread != 1 || !c.ChatBar.MailUnread {
		t.Fatalf("inbox %v unread %d blink %v", b.Inbox, b.Unread, c.ChatBar.MailUnread)
	}
	l := b.Inbox[0]
	if l.Kind != hud.MailKindFriend || string(l.Name) != "Pal" || string(l.Text) != "hello\rthere" || string(hud.MailDate(l.Date)) != "2026/10/12 09:30:00" {
		t.Fatalf("letter %+v", l)
	}
	if _, err := os.Stat(c.mailPath()); err != nil {
		t.Fatal("mailbox not saved", err)
	}
	at := c.ChatBar.Abs()
	c.ChatBar.LeftDown(0, at.X+10, at.Y+5)
	if !b.Visible {
		t.Fatal("mail icon did not open the mailbox")
	}
	row := b.Abs()
	c.Input.X, c.Input.Y = row.X+0x40, row.Y+0x5f+5
	*now = now.Add(time.Second)
	c.Frame()
	b.LeftDown(0, c.Input.X, c.Input.Y)
	b.LeftUp(0, c.Input.X, c.Input.Y)
	if !r.Visible || r.Index != 1 || b.Unread != 0 || c.ChatBar.MailUnread || !b.Inbox[0].Read {
		t.Fatalf("read form %v index %d unread %d", r.Visible, r.Index, b.Unread)
	}
	if string(r.Memo.Text) != "hello\rthere" {
		t.Fatalf("memo %q", r.Memo.Text)
	}
	c.MailSend.Old.OnClick()
	if !b.Visible || c.MailSend.Visible {
		t.Fatal("Old Mail did not open the mailbox")
	}
	r.Show()
	r.Reply.OnClick()
	if r.Visible || !c.MailSend.Visible || c.MailSend.To.ID != 20001 || c.MailSend.Left != r.Left {
		t.Fatal("reply did not address the sender")
	}
	// The saved mailbox returns at the next world entry.
	b.Inbox, b.Unread = nil, 0
	c.loadMail()
	if len(b.Inbox) != 1 || !b.Inbox[0].Read {
		t.Fatal("mailbox not reloaded", b.Inbox)
	}
}

// TestMailboxOrder: letters file newest first, repeats are dropped, the
// system sender is named, and deleting from the read form opens the letter
// that takes its place.
func TestMailboxOrder(t *testing.T) {
	c, _ := mailClient(t)
	b, r := c.Mailbox, c.MailRead
	c.dispatch(letterPacket(20001, 46000, "old"))
	c.dispatch(letterPacket(101, 46002, "news"))
	c.dispatch(letterPacket(20001, 46001, "mid"))
	c.dispatch(letterPacket(20001, 46001, "mid"))
	var got []string
	for _, l := range b.Inbox {
		got = append(got, string(l.Text))
	}
	if len(got) != 3 || got[0] != "news" || got[1] != "mid" || got[2] != "old" || b.Unread != 3 {
		t.Fatalf("order %v unread %d", got, b.Unread)
	}
	if s := b.Inbox[0]; s.Kind != hud.MailKindSystem || s.Sender != 100 || !bytes.Equal(s.Name, hud.GMFriendName) {
		t.Fatalf("system letter %+v", s)
	}
	b.OpenInbox(2)
	r.Delete.OnClick()
	if len(b.Inbox) != 2 || !r.Visible || r.Index != 2 || string(r.Mail.Text) != "old" || b.Unread != 1 {
		t.Fatalf("after delete: %d letters, index %d %q, unread %d", len(b.Inbox), r.Index, r.Mail.Text, b.Unread)
	}
	r.Prev.OnClick()
	if r.Index != 1 || string(r.Mail.Text) != "news" || b.Unread != 0 {
		t.Fatal("previous letter", r.Index, b.Unread)
	}
}

func TestMailPreview(t *testing.T) {
	for in, want := range map[string]string{
		"short":                             "short",
		"line one\rline two":                "line one",
		"abcdefghijklmnopqrstuvwxyz":        "abcdefghijklmnop...",
		"\xa4\xa4\xa4\xe5abcdefghijklmnopq": "\xa4\xa4\xa4\xe5abcdefghijkl...",
	} {
		if got := string(hud.MailPreview([]byte(in))); got != want {
			t.Errorf("%q: %q, want %q", in, got, want)
		}
	}
}

// TestMailboxSnapshot renders the mailbox and an open letter.
func TestMailboxSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, _ := mailClient(t)
	c.dispatch(letterPacket(20001, 46000.5, "Hello! Are you coming to the beach tomorrow?"))
	c.dispatch(letterPacket(100, 46001.25, "Welcome to Wonderland."))
	c.Mailbox.Show()
	save(t, c, filepath.Join(dir, "mailbox.png"))
	c.Mailbox.OpenInbox(2)
	save(t, c, filepath.Join(dir, "mailread.png"))
	c.MailRead.Hide()
	c.MailCompose.Open([]byte("Pal"), []byte("See you there!"))
	save(t, c, filepath.Join(dir, "mailcompose.png"))
}

// TestMailCompose: Send Mail opens the compose form; a friend's name sends
// 14/1 and files the letter under Sent, where a click reopens it; another
// name needs a Mailbox.
func TestMailCompose(t *testing.T) {
	c, _ := mailClient(t)
	sent := wire(t, c)
	b, w := c.Mailbox, c.MailCompose
	b.SendMail.OnClick()
	if !w.Visible {
		t.Fatal("compose form not shown")
	}
	w.To.SetText([]byte("Stranger"))
	for _, ch := range []byte("hi") {
		w.Memo.Char(ch)
	}
	w.Send.OnClick()
	if !w.Visible || len(sent()) != 0 {
		t.Fatal("stranger letter sent")
	}
	w.To.SetText([]byte("Pal"))
	w.Send.OnClick()
	want := append(binary.LittleEndian.AppendUint32([]byte{protocol.CommandFriends, protocol.FriendsTextMail, 1}, 20001), "hi"...)
	if p := sent(); w.Visible || len(p) != 1 || !bytes.Equal(p[0], want) {
		t.Fatalf("friend letter %x", p)
	}
	if len(b.Sent) != 1 || string(b.Sent[0].Name) != "Pal" || string(b.Sent[0].Text) != "hi" {
		t.Fatal("letter not filed", b.Sent)
	}
	b.SelectTab(1)
	b.OpenSent(1, b.Sent[0])
	if !w.Visible || string(w.To.Text) != "Pal" || string(w.Memo.Text) != "hi" {
		t.Fatal("sent letter not reopened")
	}
}
