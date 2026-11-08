package app

import (
	"bytes"
	"testing"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

// Incoming bytes copied independently from friends-20261108-01.pcapng.
// Request, acceptance, presence, text mail, then reciprocal removal need no
// replacement friend-list packet from the server.
func TestFriendsCapturedExchange(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.World.Player.ID = 4617671
	c.World.Peers = map[uint32]*world.Peer{4617590: {Player: world.Player{ID: 4617590, Name: []byte("Test0002"), Body: 1}, Level: 15}}
	sent := wire(t, c)
	c.dispatch([]byte{14, 2, 118, 117, 70, 0})
	if c.friendAsk == nil || c.UI.Modal == nil {
		t.Fatal("friend request not shown")
	}
	for _, child := range c.UI.Modal.Base().Children {
		if button, ok := child.(*seui.FixedButton); ok {
			button.Click()
			break
		}
	}
	if packets := sent(); len(packets) != 1 || !bytes.Equal(packets[0], []byte{14, 3, 118, 117, 70, 0, 1}) {
		t.Fatal(packets)
	}
	c.dispatch([]byte{14, 3, 118, 117, 70, 0, 1})
	c.dispatch([]byte{14, 9, 118, 117, 70, 0, 0})
	if f, ok := c.friend(4617590); !ok || string(f.Name) != "Test0002" || c.friendPresence(f.ID) != hud.PresenceOnline {
		t.Fatal("friend not added online", f)
	}
	c.World.RemovePeer(4617590)
	if c.friendPresence(4617590) != hud.PresenceOnline {
		t.Fatal("map departure marked friend offline")
	}
	c.dispatch([]byte{14, 1, 118, 117, 70, 0, 174, 91, 161, 86, 212, 159, 230, 64, 84, 101, 115, 116, 32, 50, 33})
	if len(c.Mailbox.Inbox) != 1 || string(c.Mailbox.Inbox[0].Text) != "Test 2!" || string(c.Mailbox.Inbox[0].Name) != "Test0002" {
		t.Fatal(c.Mailbox.Inbox)
	}
	c.dispatch([]byte{14, 4, 118, 117, 70, 0})
	if _, ok := c.friend(4617590); ok || len(c.Social.Friends) != 1 || len(c.Mailbox.Inbox) != 1 {
		t.Fatal("removal did not update friend list or destroyed mail")
	}
}

func TestFriendIncrementalProfilesAndPresence(t *testing.T) {
	c, _, _ := enteredClient(t)
	// The short AC14:11 record ends after nickname; it is not a full
	// AC14:5 record with guild and VIP bytes.
	record := friendRecord(20001, "Pal", 3)
	c.dispatch(append([]byte{14, 11}, record[:len(record)-2]...))
	c.dispatch([]byte{14, 9, 33, 78, 0, 0, 0})
	c.dispatch([]byte{14, 9, 33, 78, 0, 0, 3})
	if c.friendPresence(20001) != hud.PresenceOffline {
		t.Fatal("offline update ignored")
	}
	c.dispatch([]byte{14, 9, 33, 78, 0, 0, 1, 34, 78, 0, 0}) // Truncated batch must change nothing.
	if c.friendPresence(20001) != hud.PresenceOffline {
		t.Fatal("partial presence applied")
	}
	c.dispatch([]byte{14, 9, 33, 78, 0, 0, 0})
	record = friendRecord(20001, "New name", 9)
	c.dispatch(append([]byte{14, 8}, record...))
	if f, ok := c.friend(20001); !ok || f.Level != 9 || string(f.Name) != "New name" || c.friendPresence(f.ID) != hud.PresenceOnline {
		t.Fatal(f)
	}
	before := len(c.Social.Friends)
	c.dispatch(append([]byte{14, 11}, record[:len(record)-3]...))
	if len(c.Social.Friends) != before {
		t.Fatal("malformed profile changed list")
	}
}

// TestMakeFriends: the toolbar button arms mode 5 with cursor 5; a click
// on a player sends 14/2 and ends the mode; a friend is refused locally;
// map 10072 refuses the button.
func TestMakeFriends(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	capture := wire(t, c)
	peer := &world.Peer{Player: world.Player{ID: 20002, Name: []byte("Bud"), X: c.World.Player.X + 50, Y: c.World.Player.Y}, Level: 7}
	c.World.Peers = map[uint32]*world.Peer{peer.ID: peer}
	button := c.FuncButtons.Buttons[makeFriendsButton]
	button.Click()
	c.Input.Hovered = nil
	c.updateCursor(c.Now())
	if !c.friendTarget || c.Cursors.Current != cursor.ShapePointAlt {
		t.Fatal("Make Friends did not arm with cursor 5")
	}
	c.GroundClick(10, 400)
	if c.friendTarget || len(capture()) != 0 {
		t.Fatal("an empty click did not cancel cleanly")
	}
	button.Click()
	cx, cy := c.World.Camera()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	want := protocol.Builder{protocol.CommandFriends, protocol.FriendsRequest}.U32(20002)
	if p := capture(); c.friendTarget || len(p) != 1 || !bytes.Equal(p[0], want) {
		t.Fatalf("request %x armed %v", p, c.friendTarget)
	}
	// 14/3 result 1 adds the player from view.
	c.dispatch(append(protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(20002), 1))
	if f, ok := c.friend(20002); !ok || string(f.Name) != "Bud" || f.Level != 7 {
		t.Fatalf("friend not added: %+v", f)
	}
	button.Click()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	if p := capture(); len(p) != 1 {
		t.Fatalf("asked an existing friend: %x", p)
	}
	c.World.Player.Map = mapNoMakeFriends
	button.Click()
	if c.friendTarget {
		t.Fatal("armed on map 10072")
	}
}

// TestFriendQuestion: 14/2 asks; Yes answers 14/3 result 1, No result 2
// and silence result 3 after ten seconds.
func TestFriendQuestion(t *testing.T) {
	c, now, _ := enteredClient(t)
	capture := wire(t, c)
	c.World.Peers = map[uint32]*world.Peer{20003: {Player: world.Player{ID: 20003, Name: []byte("Kit")}}}
	ask := protocol.Builder{protocol.CommandFriends, protocol.FriendsRequest}.U32(20003)
	answer := func(result byte) []byte {
		return append(protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(20003), result)
	}
	c.dispatch(ask)
	if c.UI.Modal == nil || c.friendAsk == nil {
		t.Fatal("no question")
	}
	first := c.friendAsk
	c.dispatch(ask)
	if c.friendAsk != first || len(capture()) != 0 {
		t.Fatal("duplicate prompt cancelled the pending invitation")
	}
	press := func(i int) {
		var buttons []func()
		for _, k := range c.UI.Modal.Base().Children {
			if b, ok := k.(*seui.FixedButton); ok {
				buttons = append(buttons, b.Click)
			}
		}
		buttons[i]()
	}
	press(0)
	if p := capture(); len(p) != 1 || !bytes.Equal(p[0], answer(1)) {
		t.Fatalf("yes %x", p)
	}
	c.dispatch(ask)
	press(1)
	if p := capture(); len(p) != 2 || !bytes.Equal(p[1], answer(2)) {
		t.Fatalf("no %x", p)
	}
	c.dispatch(ask)
	*now = now.Add(friendAskTimeout)
	c.Frame()
	if p := capture(); len(p) != 3 || !bytes.Equal(p[2], answer(3)) || c.UI.Modal != nil {
		t.Fatalf("timeout %x", p)
	}
}
