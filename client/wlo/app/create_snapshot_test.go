package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/protocol"
)

// fixedClock pins every animation clock of the creation forms.
func fixedClock(c *Client, now time.Time) {
	clock := func() time.Time { return now }
	c.Now = clock
	c.Create.Now = clock
	c.Create.Rand = func(int) int { return 0 }
	if p, ok := c.Create.Role.Painter.(interface{ SetNow(func() time.Time) }); ok {
		p.SetNow(clock)
	}
	c.Create.PrevChar.Now, c.Create.NextChar.Now = clock, clock
}

// TestCreateCharacterSnapshot walks the creation wizard and writes each
// step to SNAPSHOT_DIR for comparison with the reference screenshots.
func TestCreateCharacterSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	fixedClock(c, time.Now())
	c.Frame()
	c.Servers.Hide()
	c.Chars.Roster([]byte{1})
	c.dispatch([]byte{protocol.CommandHandshake, protocol.HandshakeWireCode3, 0})
	c.Frame()
	save(t, c, filepath.Join(dir, "create_1.png"))

	cr := c.Create
	cr.NameField.SetText([]byte("Dango2"))
	cr.RolePics[4].Click() // body 4, head 2, as in the reference screenshots
	c.Frame()
	save(t, c, filepath.Join(dir, "create_1_selected.png"))

	cr.Buttons[2].Click()
	if !cr.Waiting {
		t.Fatal("Next should wait for the name check")
	}
	c.dispatch([]byte{protocol.CommandCharacterCreation, protocol.CharacterCreationCheckName, 0})
	if cr.Step != 2 {
		t.Fatalf("step %d after the name check", cr.Step)
	}
	c.Frame()
	save(t, c, filepath.Join(dir, "create_2.png"))

	for range 5 {
		cr.StatArrows[6].Click() // INT +
	}
	cr.Buttons[2].Click()
	c.Frame()
	save(t, c, filepath.Join(dir, "create_3.png"))

	cr.Elements[3].OnClickTag(3)
	c.Frame()
	save(t, c, filepath.Join(dir, "create_3_fire.png"))

	cr.Buttons[2].Click()
	c.Frame()
	save(t, c, filepath.Join(dir, "create_4.png"))
	cr.Parts[2].OnClickTag(2)
	cr.ColorArrows[1].Click()
	cr.ColorArrows[1].Click()
	c.Frame()
	save(t, c, filepath.Join(dir, "create_4_skin.png"))

	cr.Buttons[2].Click()
	if cr.Step != 5 || !c.Password.Visible {
		t.Fatalf("step %d, dialog shown %v", cr.Step, c.Password.Visible)
	}
	c.Frame()
	save(t, c, filepath.Join(dir, "create_5.png"))
}

// TestCreatePacket checks 9/1's layout against the send case at 0x2c396c.
func TestCreatePacket(t *testing.T) {
	c := testClient(t)
	cr := c.Create
	cr.RolePics[3].OnClickTag(3)
	cr.Stats = [6]byte{0, 1, 2, 0, 0, 2}
	cr.Points = 0
	cr.Role.Body.Element = 3
	cr.Role.Body.Colors[0], cr.Role.Body.Colors[0x2c] = 1, 9
	for i, v := range []string{"abcdef", "abcdef", "secret", "secret"} {
		c.Password.Fields[i+1].SetText([]byte(v))
	}
	cr.EncodeColors()
	got := cr.CreatePacket()
	if got[0] != 9 || got[1] != 1 || got[2] != 1 || got[3] != 0 || got[4] != 0 || got[5] != 0 {
		t.Fatalf("header % x", got[:6])
	}
	if c1 := le32(got[6:]); c1 != 144444444 {
		t.Fatalf("colour1 %d", c1)
	}
	if c2 := le32(got[10:]); c2 != 444444449 {
		t.Fatalf("colour2 %d", c2)
	}
	if !bytes.Equal(got[14:20], []byte{3, 1, 2, 0, 0, 2}) {
		t.Fatalf("element and points % x", got[14:20])
	}
	if !bytes.Equal(got[20:], []byte("\x06abcdef\x06secret")) {
		t.Fatalf("passwords %q", got[20:])
	}
	if login.RoleIndex(1, 0) != 3 {
		t.Fatal("role index")
	}
}

func le32(b []byte) uint32 {
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

// TestNoticeSnapshot reproduces Login_Forced_Error_Message.png: two notices
// over the account form.
func TestNoticeSnapshot(t *testing.T) {
	dir := os.Getenv("SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("SNAPSHOT_DIR is not set")
	}
	c := testClient(t)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Frame()
	c.Servers.Hide()
	c.Login.Show()
	c.Notices.Show([]byte("Success. Use new name to login again"), 5*time.Second, now)
	c.Notices.Show([]byte("Password wrong"), 5*time.Second, now)
	c.Frame()
	save(t, c, filepath.Join(dir, "notice.png"))
}
