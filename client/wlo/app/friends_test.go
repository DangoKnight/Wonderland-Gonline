package app

import (
	"bytes"
	"testing"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

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
	if !c.friendTarget || len(capture()) != 0 {
		t.Fatal("an empty click ended the mode or sent")
	}
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
