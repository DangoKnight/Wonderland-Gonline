package app

import (
	"bytes"
	"testing"

	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestPKButtonNativeTargetingAndCancellation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	peer := &world.Peer{Player: world.Player{ID: 20002, X: c.World.Player.X + 50, Y: c.World.Player.Y}}
	c.World.Peers = map[uint32]*world.Peer{peer.ID: peer}
	button := c.FuncButtons.Buttons[pkButton]
	button.Click()
	c.Input.Hovered = nil
	c.updateCursor(c.Now())
	if !c.pkTarget || !button.IconDown || !c.World.JoinTeamSelection || c.Cursors.Current != cursor.ShapeSword {
		t.Fatal("PK button did not arm native sword targeting")
	}
	cx, cy := c.World.Camera()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	packets := sent()
	// Independent Legacy request: AC11:2:3, player 20002, zero NPC click ID.
	want := []byte{11, 2, 3, 34, 78, 0, 0, 0, 0}
	if c.pkTarget || button.IconDown || len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatal("incorrect PK request or selection was not released", packets)
	}
	for _, cancel := range []func(){
		func() { c.GroundClick(0, 0) },
		func() { (&Game{C: c}).rightPress(0, -10, -10) },
		func() { c.TeamKey(escapeKey, 0) },
		func() { button.Click() },
		func() { c.setJoinTeamTarget(true) },
		func() { c.setFriendTarget(true) },
		func() { c.setTradeTarget(true) },
	} {
		c.cancelFieldSelection()
		button.Click()
		cancel()
		if c.pkTarget || button.IconDown || len(sent()) != 1 || c.World.Walking() {
			t.Fatal("PK cancellation leaked a click/action")
		}
	}
	c.cancelFieldSelection()
	c.SettingsState.Synced, c.SettingsState.Permissions.PKAllowed = true, false
	button.Click()
	if c.pkTarget {
		t.Fatal("PK armed while disabled")
	}
	c.SettingsState.Permissions.PKAllowed = true
	c.TeamState.Members = []uint32{peer.ID, c.World.Player.ID}
	c.TeamState.Leader = peer.ID
	button.Click()
	if c.pkTarget {
		t.Fatal("team follower armed PK")
	}
	c.TeamState.ClearParty()
	c.TeamState.Members = []uint32{c.World.Player.ID, peer.ID}
	c.TeamState.Leader = c.World.Player.ID
	button.Click()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	if c.pkTarget || len(sent()) != 1 {
		t.Fatal("teammate was selected for PK")
	}
	c.TeamState.ClearParty()
	button.Click()
	c.World.ApplyBattlePresence([]byte{11, 4, 2, 34, 78, 0, 0, 0, 0, 1}, c.Now())
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	if c.pkTarget || len(sent()) != 1 {
		t.Fatal("fighting player was selected for PK")
	}
	c.World.ApplyBattlePresence([]byte{11, 4, 2, 34, 78, 0, 0, 0, 0, 0}, c.Now())
	button.Click()
	c.SettingsState.Permissions.PKAllowed = false
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	if c.pkTarget || len(sent()) != 1 {
		t.Fatal("stale PK permission was ignored")
	}
	c.SettingsState.Permissions.PKAllowed = true
	peer.X = c.World.Player.X + pkTargetDistance + 1
	button.Click()
	c.GroundClick(peer.X-cx, peer.Y-cy-20)
	if c.pkTarget || len(sent()) != 1 {
		t.Fatal("distant PK target was submitted")
	}
	button.Click()
	pl := c.World.Player
	c.warp(protocol.Builder{}.U32(pl.ID).U16(pl.Map).U16(uint16(pl.X)).U16(uint16(pl.Y)))
	if c.pkTarget || button.IconDown || c.World.JoinTeamSelection {
		t.Fatal("map placement retained PK selection")
	}
}
