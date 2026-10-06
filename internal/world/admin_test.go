package world

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestAdminClearDroppedPreservesAuthoredGround(t *testing.T) {
	w := groundFixture()
	before := w.GroundPacket(12345)
	slots, ok := w.FreeGroundSlots(12345, 2)
	if !ok {
		t.Fatal("no slots")
	}
	w.Drop(12345, slots, game.Item{ID: 42, Count: 2}, 10, 20)
	packets := w.ClearDropped(12345)
	if len(packets) != 2 || !bytes.Equal(packets[0], []byte{23, 2, 1, 0, 0}) || !bytes.Equal(packets[1], []byte{23, 2, 3, 0, 0}) {
		t.Fatal(packets)
	}
	if !bytes.Equal(before, w.GroundPacket(12345)) {
		t.Fatal("authored ground changed")
	}
}
func TestAdminChestSyncCooldownAndVisibilityOverride(t *testing.T) {
	w := New(&assets.Catalog{Maps: map[uint16]assets.Map{10017: {ID: 10017, NPCs: []assets.MapNPC{{ClickID: 4, Flags: 1}}, Events: []assets.Event{{ClickID: 4}}}}})
	c := &game.Character{Map: 10017, Quests: map[uint32]game.Quest{}, ChestRespawns: map[uint32]time.Time{ChestKey(10017, 4): time.Now().Add(time.Hour)}}
	v := NewView()
	v.AdminActors = map[uint16]bool{4: false}
	if w.VisibleIn(c, v, 10017, 4) {
		t.Fatal("explicit hide ignored")
	}
	v.AdminActors[4] = true
	if !w.VisibleIn(c, v, 10017, 4) {
		t.Fatal("explicit show ignored")
	}
	contains := func(packets [][]byte, want []byte) bool {
		for _, p := range packets {
			if bytes.Equal(p, want) {
				return true
			}
		}
		return false
	}
	if !contains(w.Sync(c, v, true), []byte{22, 1, 4, 0, 1}) {
		t.Fatal("cooldown missing on sync")
	}
	clone := c.Clone()
	clone.ChestRespawns[ChestKey(10017, 4)] = time.Now().Add(-time.Hour)
	if !c.ChestRespawns[ChestKey(10017, 4)].After(time.Now()) {
		t.Fatal("clone aliases cooldown")
	}
	if !contains(w.Sync(&clone, v, true), []byte{22, 1, 4, 0, 0}) {
		t.Fatal("expired chest reopened snapshot")
	}
}
