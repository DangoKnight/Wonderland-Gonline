package app

import (
	"bytes"
	"os"
	"testing"
	"time"

	"wonderland-go/internal/game"
)

func TestNativePresentationAndAllocation(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	peer := game.Character{ID: 20002, Slot: 1, Name: "Ann", Nickname: "Traveler", Body: 1, Level: 3, Element: 2, Map: c.World.Player.Map, X: 900, Y: 1100}
	appearance, err := peer.AppearancePacket(true)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(appearance)
	if string(c.World.Peers[20002].Nickname) != "Traveler" {
		t.Fatal("appearance nickname missing")
	}
	c.dispatch([]byte{10, 1, 34, 78, 0, 0, 4, 'F', 'i', 's', 'h'})
	if string(c.World.Peers[20002].Nickname) != "Fish" {
		t.Fatal("nickname update ignored")
	}
	c.dispatch([]byte{10, 1, 34, 78, 0, 0, 8, 'B', 'a', 'd'})
	if string(c.World.Peers[20002].Nickname) != "Fish" {
		t.Fatal("malformed nickname mutated state")
	}
	c.dispatch([]byte{10, 5, 34, 78, 0, 0, 3, 'A', 'n', 'a'})
	if string(c.World.Peers[20002].Name) != "Ana" || string(c.playerName(20002)) != "Ana" {
		t.Fatal("name update missing")
	}
	c.dispatch([]byte{10, 3, 34, 78, 0, 0, 255})
	if c.World.Peers[20002].Presence != 255 {
		t.Fatal("presence update ignored")
	}
	c.dispatch([]byte{32, 2, 34, 78, 0, 0, 17})
	if c.World.Peers[20002].Direction != 17 {
		t.Fatal("native held pose missing")
	}
	c.dispatch([]byte{32, 1, 34, 78, 0, 0, 101})
	if c.World.Expressions[20002].Code != 101 {
		t.Fatal("native expression missing")
	}
	*now = now.Add(time.Second)
	c.dispatch([]byte{32, 1, 34, 78, 0, 0, 101})
	if !c.World.Expressions[20002].Started.Equal(*now) || c.World.Peers[20002].Direction != 17 {
		t.Fatal("repeated expression changed pose")
	}
	c.dispatch([]byte{6, 1, 34, 78, 0, 0, 26, 44, 0, 55, 0})
	if c.World.Peers[20002].Direction != 26 {
		t.Fatal("stop pose missing")
	}
	c.dispatch([]byte{5, 0, 34, 78, 0, 0, 0x39, 0x1b})
	if len(c.World.Peers[20002].Items) != 1 || c.World.Peers[20002].Items[0] != 6969 {
		t.Fatal("peer equipment ignored")
	}
	c.dispatch([]byte{5, 0, 34, 78, 0, 0})
	if len(c.World.Peers[20002].Items) != 0 {
		t.Fatal("empty equipment retained")
	}
	c.Stats.Points = 3
	c.Inventory.Show()
	var sent [][]byte
	c.Inventory.Send = func(p []byte) { sent = append(sent, append([]byte(nil), p...)) }
	c.Inventory.Increase[0].OnClickTag(0)
	c.Inventory.Increase[3].OnClickTag(3)
	c.Inventory.SubmitAllocation()
	if len(sent) != 1 || !bytes.Equal(sent[0], []byte{8, 1, 0, 2, 28, 1, 0, 33, 1, 0}) {
		t.Fatalf("allocation %x", sent)
	}
	c.dispatch([]byte{8, 1, 38, 1, 99, 0, 0, 0, 1, 0, 0, 0})
	if c.Stats.Points != 3 || !c.Inventory.AllocationWaiting {
		t.Fatal("pet reply changed player allocation")
	}
	c.dispatch([]byte{8, 1, 38, 1, 1, 0, 0, 0, 0, 0, 0, 0})
	if c.Stats.Points != 1 || c.Inventory.AllocationWaiting {
		t.Fatal("allocation reply did not clear waiting")
	}
	c.Inventory.AddPoint(1)
	c.Frame()
	if out := os.Getenv("ALLOCATION_SNAPSHOT"); out != "" {
		savePNG(t, out, c)
	}
	c.Inventory.Hide()
	if c.Inventory.PendingPoints != ([5]uint16{}) {
		t.Fatal("hidden inventory retained draft")
	}
	// Leaving the scene also drops transient expressions.
	c.World.RemovePeer(20002)
	if _, ok := c.World.Expressions[20002]; ok {
		t.Fatal("departed peer expression retained")
	}
}
