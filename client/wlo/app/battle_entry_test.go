package app

import (
	"bytes"
	"testing"
	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/world"
)

func TestBattleEntryToolbarAndCancellation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	peer := &world.Peer{Player: world.Player{ID: 20002, X: c.World.Player.X + 50, Y: c.World.Player.Y}}
	c.World.Peers = map[uint32]*world.Peer{peer.ID: peer}
	c.World.ApplyBattlePresence([]byte{11, 4, 2, 34, 78, 0, 0, 0, 0, 1}, c.Now())
	for _, tc := range []struct {
		button int
		mode   byte
		shape  cursor.Shape
	}{{3, 5, cursor.ShapeSwordDown}, {4, 4, cursor.ShapeLook}} {
		button := c.FuncButtons.Buttons[tc.button]
		button.Click()
		c.Input.Hovered = nil
		c.updateCursor(c.Now())
		if c.battleEntryTarget != tc.mode || !button.IconDown || !c.World.JoinTeamSelection || c.Cursors.Current != tc.shape {
			t.Fatal("entry selection not armed")
		}
		cx, cy := c.World.Camera()
		c.GroundClick(peer.X-cx, peer.Y-cy-20)
		packets := sent()
		want := []byte{11, 2, tc.mode, 34, 78, 0, 0, 0, 0}
		if len(packets) == 0 || !bytes.Equal(packets[len(packets)-1], want) || c.battleEntryTarget != 0 || button.IconDown {
			t.Fatal("wrong entry request", packets)
		}
		before := len(packets)
		button.Click()
		c.GroundClick(0, 0)
		if c.battleEntryTarget != 0 || c.World.Walking() || len(sent()) != before {
			t.Fatal("invalid target leaked movement")
		}
		button.Click()
		(&Game{C: c}).rightPress(0, -10, -10)
		if c.battleEntryTarget != 0 || len(sent()) != before {
			t.Fatal("right click failed to cancel")
		}
		button.Click()
		c.setJoinTeamTarget(true)
		if c.battleEntryTarget != 0 || button.IconDown {
			t.Fatal("selection modes overlapped")
		}
		c.cancelFieldSelection()
	}
}

func TestSpectatorMenuOnlyExits(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.battle.state.Active, c.battle.state.Spectating = true, true
	c.battleMenu()
	if c.battle.menu == nil || !c.battle.menu.Visible {
		t.Fatal("missing spectator exit")
	}
	if _, ok := c.battleActor(); ok {
		t.Fatal("spectator has combat controls")
	}
	c.leaveSpectatedBattle()
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{11, 1, 3}) {
		t.Fatal("spectator submitted flee", got)
	}
	c.battle.state.Spectating = false
	c.leaveSpectatedBattle()
	if len(sent()) != 1 {
		t.Fatal("stale exit control affected combatant")
	}
}
