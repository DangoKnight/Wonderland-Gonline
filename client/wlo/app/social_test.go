package app

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/protocol"
)

// friendRecord is one 14/5 record in raw bytes: ID, name, level, two
// bytes, element, body, head, two colours, nickname, guild length 0 and
// the VIP byte.
func friendRecord(id uint32, name string, level byte) []byte {
	b := binary.LittleEndian.AppendUint32(nil, id)
	b = append(b, byte(len(name)))
	b = append(b, name...)
	b = append(b, level, 2, 0, 3, 1, 0)
	b = binary.LittleEndian.AppendUint32(b, 0x11223344)
	b = binary.LittleEndian.AppendUint32(b, 0x55667788)
	return append(b, 2, 'h', 'i', 0, 1)
}

// TestFriendList: 14/5 fills the table after the GM entry, which the
// client keeps first; IDs 100 and 101 restart the list.
func TestFriendList(t *testing.T) {
	c, _, _ := enteredClient(t)
	p := []byte{protocol.CommandFriends, protocol.FriendsList}
	p = append(p, friendRecord(100, "GM", 0)...)
	for i := uint32(1); i <= 6; i++ {
		p = append(p, friendRecord(20000+i, "Pal", byte(i))...)
	}
	c.dispatch(p)
	s := c.Social
	if len(s.Friends) != 7 || s.Friends[0].ID != hud.GMFriendID || !bytes.Equal(s.Friends[0].Name, hud.GMFriendName) {
		t.Fatalf("friends %+v", s.Friends)
	}
	f := s.Friends[1]
	if f.ID != 20001 || string(f.Name) != "Pal" || f.Level != 1 || f.Job != 2 || f.Element != 3 || f.Body != 1 || f.Color1 != 0x11223344 || string(f.Nickname) != "hi" || f.VIP != 1 {
		t.Fatalf("record %+v", f)
	}
	// Two pages: the GM and four friends, then two.
	s.NextPage.OnClick()
	if s.Page != 2 {
		t.Fatalf("page %d", s.Page)
	}
	s.NextPage.OnClick()
	if s.Page != 2 {
		t.Fatal("turned past the last page")
	}
}

// TestFriendDelete: Del Friend without a tick asks for one; one tick asks
// "Delete this friend?" and OK sends 14/4 with the ID.
func TestFriendDelete(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.dispatch(append([]byte{protocol.CommandFriends, protocol.FriendsList}, friendRecord(20001, "Pal", 3)...))
	s := c.Social
	s.Toggle()
	s.DelFriend.OnClick()
	if c.Talk.Shown() {
		t.Fatal("asked without a tick")
	}
	s.TickRow(1) // row 0 is the GM entry
	s.DelFriend.OnClick()
	if !c.Talk.Shown() || !bytes.HasPrefix(c.Talk.Lines()[0], []byte("Delete this friend?")) {
		t.Fatal("no confirmation")
	}
	c.talkConfirm()
	*now = now.Add(time.Millisecond)
	want := []byte{protocol.CommandFriends, protocol.FriendsRemove, 0x21, 0x4e, 0, 0}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("sent %x, want %x", got, want)
	}
}

// TestSocialSnapshot renders the Friend tab with only the GM entry
// (Social/Social_Friend.png).
func TestSocialSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.Social.Toggle()
	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "social.png"))
}

// TestEmoteTab: an emoticon sends AC32:1 with 100 + n, an action AC32:2
// with its pose turned to the facing; a cell dragged onto an Alt slot
// fills it and is saved; Ctrl+1 puts its phrase in the chat bar.
func TestEmoteTab(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	sent := wire(t, c)
	s := c.Social
	s.Toggle()
	s.SelectTab(hud.SocialEmote)
	c.playEmote(103)
	c.World.Player.Direction = 8 // standing, facing up
	c.playPose(3)                // table entry 3: 16, a two-facing family
	want := [][]byte{{protocol.CommandPose, protocol.PoseEmote, 103}, {protocol.CommandPose, protocol.PoseBroadcast, 16}}
	got := sent()
	if len(got) != 2 || !bytes.Equal(got[0], want[0]) || !bytes.Equal(got[1], want[1]) {
		t.Fatalf("sent %x, want %x", got, want)
	}
	// Drag action 5 onto Alt+2 (slot left 71, top 0x194, within the window).
	at := s.Abs()
	c.Input.X, c.Input.Y = at.X+0xb4+5, at.Y+0xb4+5
	s.StartDrag(hud.EmoteSlot{Kind: hud.SlotAction, Value: 5})
	if !s.Drop(at.X+71+10, at.Y+0x194+10) {
		t.Fatal("no drag")
	}
	if sl := s.Slots()[1]; sl.Kind != hud.SlotAction || sl.Value != 5 {
		t.Fatalf("Alt+2 holds %+v", sl)
	}
	s.SetPhrases([5][]byte{[]byte("Test")})
	c.saveEmotes()
	s.SetSlots(hud.DefaultSlots())
	s.SetPhrases([5][]byte{})
	c.loadEmotes()
	if sl := s.Slots()[1]; sl.Value != 5 || string(s.Phrases()[0]) != "Test" {
		t.Fatalf("reloaded %+v %q", s.Slots(), s.Phrases()[0])
	}
	if !c.EmoteKey('1', shiftCtrl) || string(c.ChatBar.Message.Text) != "Test" {
		t.Fatalf("Ctrl+1 gave %q", c.ChatBar.Message.Text)
	}
	*now = now.Add(time.Millisecond)
}

// TestSocialEmoteSnapshot renders the Emote tab with Ctrl+1 "Test" and the
// default Alt+1 (Social/Social_Emote.png).
func TestSocialEmoteSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.Social.Toggle()
	c.Social.SelectTab(hud.SocialEmote)
	c.Social.SetPhrases([5][]byte{[]byte("Test")})
	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "social_emote.png"))
}

// TestSetUpSave: the profile loads from AC3; a changed nickname, a full
// date and a blood type save as AC10:1, AC10:2 (with the social code from
// the stored profile) and AC10:3 after the confirmation.
func TestSetUpSave(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	p := &c.World.Player
	p.Blood, p.BirthYear, p.BirthMonth, p.BirthDay = 1, 100, 3, 4 // 2000-03-04, A
	s := c.Social
	s.LoadProfile([]byte("old"), p.Blood, p.BirthYear, p.BirthMonth, p.BirthDay)
	s.Toggle()
	s.SelectTab(hud.SocialSetUp)
	nick, blood, y, m, d := s.Profile()
	if string(nick) != "old" || blood != 1 || y != 2000 || m != 3 || d != 4 {
		t.Fatalf("loaded %q %d %d-%d-%d", nick, blood, y, m, d)
	}
	s.SetProfile([]byte("new"), 3, 2001, 7, 9)
	s.SaveSetUp()
	// Code: 2000/(3+4+2)=222, ×sign(7/9 → 4)×blood 1 = 888, + ID%10 + len("Dango").
	code := byte((888 + int(p.ID%10) + 5) % 10)
	want := [][]byte{
		{protocol.CommandSocialRelations, protocol.SocialNicknameUpdate, 3, 'n', 'e', 'w'},
		{protocol.CommandSocialRelations, protocol.SocialProfileUpdate, code, 3, 101, 7, 9},
		{protocol.CommandSocialRelations, protocol.SocialFriendList},
	}
	got := sent()
	if len(got) != len(want) {
		t.Fatalf("sent %x", got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("packet %d %x, want %x", i, got[i], want[i])
		}
	}
	if p.BirthYear != 101 || p.Blood != 3 {
		t.Fatal("player profile not updated")
	}
}

// TestSocialSetUpSnapshot renders the Set Up tab of a player without a
// profile (Social/Social_SetUp.png).
func TestSocialSetUpSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.Social.LoadProfile(nil, 0, 0, 0, 0)
	c.Social.Toggle()
	c.Social.SelectTab(hud.SocialSetUp)
	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "social_setup.png"))
}

// TestMailSend: a row's mail icon opens the form for that friend; typed
// text goes out as 14/1 (kind 1, the ID, the text); ticked friends get
// one mail each.
func TestMailSend(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	p := []byte{protocol.CommandFriends, protocol.FriendsList}
	p = append(p, friendRecord(20001, "Pal", 3)...)
	p = append(p, friendRecord(20002, "Bud", 4)...)
	c.dispatch(p)
	s, m := c.Social, c.MailSend
	s.Toggle()
	s.MailRow(1)
	if !m.Visible || m.Mode != hud.MailToFriend || m.To.ID != 20001 {
		t.Fatalf("mail form %v mode %d to %d", m.Visible, m.Mode, m.To.ID)
	}
	for _, ch := range []byte("hey") {
		m.Memo.Char(ch)
	}
	m.Send.OnClick()
	want := append(binary.LittleEndian.AppendUint32([]byte{protocol.CommandFriends, protocol.FriendsTextMail, 1}, 20001), "hey"...)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatalf("sent %x, want %x", got, want)
	}
	s.TickRow(1)
	s.TickRow(2)
	s.SendMail.OnClick()
	if m.Mode != hud.MailToTicked || len(m.Recipients) != 2 || s.Ticked != 0 {
		t.Fatalf("mode %d recipients %d ticked %d", m.Mode, len(m.Recipients), s.Ticked)
	}
	m.Send.OnClick()
	if got := sent(); len(got) != 3 {
		t.Fatalf("sent %d packets", len(got))
	}
}

// TestMailSendSnapshot renders the mail form over the Friend tab, opened
// from the GM row (Social/Social_Mail.png).
func TestMailSendSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR not set")
	}
	c, now, _ := enteredClient(t)
	c.Social.Toggle()
	c.Social.MailRow(0)
	for i := 0; i < 10; i++ {
		*now = now.Add(20 * time.Millisecond)
		c.Frame()
	}
	save(t, c, filepath.Join(dir, "mailsend.png"))
}
