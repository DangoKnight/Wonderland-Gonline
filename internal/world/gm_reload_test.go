package world

import (
	"bytes"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
)

func TestReloadEventsPreservesMapAndRespawnState(t *testing.T) {
	w := fixture()
	old, _ := w.Map(12345)
	before := w.GroundPacket(12345)
	w.ground.taken[12345] = map[byte]time.Time{7: time.Now().Add(time.Hour)}
	w.ground.dropped[12345] = map[byte]droppedItem{99: {item: game.Item{ID: 32176, Count: 3}, x: 10, y: 20}}
	w.monsters.defeated[12345] = map[uint16]time.Time{3: time.Now().Add(time.Hour)}
	ground := w.GroundPacket(12345)
	updated := assets.Map{ID: 12345, Events: []assets.Event{{ClickID: 88}}, PreEvents: []assets.Event{{ClickID: 77}}}
	w.ReloadEvents(map[uint16]assets.Map{12345: updated})
	current, _ := w.Map(12345)
	if len(current.NPCs) != len(old.NPCs) || len(current.Items) != len(old.Items) || len(current.data.Events) != 1 || current.data.Events[0].ClickID != 88 || current.data.PreEvents[0].ClickID != 77 {
		t.Fatal("quest reload changed geometry or missed definitions")
	}
	if len(old.data.Events) != 1 || old.data.Events[0].ClickID != 4 {
		t.Fatal("mutated a retained map snapshot")
	}
	if !bytes.Equal(w.GroundPacket(12345), ground) || bytes.Equal(before, ground) || len(w.monsters.defeated[12345]) != 1 {
		t.Fatal("reload lost runtime ground/monster state")
	}
}
