package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestInstalledNorthIslandManualCollection(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	s.Assets, s.World = catalog, world.New(catalog)
	c.character.Map, c.character.X, c.character.Y = 60000, 1282, 1315
	c.character.Bag = game.Inventory{}
	c.character.Quests = nil
	c.character.EventTimers = nil
	ctx := context.Background()
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	// Literal Legacy request for actor 33, whose EVE event chooses the grass pool.
	if err := s.worldCommand(ctx, c, []byte{20, 1, 33, 0}); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	var reward []byte
	for _, p := range packets {
		if len(p) == 33 && p[0] == 23 && p[1] == 6 {
			if reward != nil {
				t.Fatal("multiple collection awards")
			}
			reward = p
		}
	}
	if reward == nil || reward[4] != 1 || reward[32] != 0 {
		t.Fatal("missing native collection reward", packets)
	}
	id := uint16(reward[2]) | uint16(reward[3])<<8
	switch id {
	case 40014, 40007, 40008, 40011, 40012, 40020:
	default:
		t.Fatal("reward outside authored pool", id)
	}
	// Independent captured timer and completion bytes, not production builders.
	if !contains(packets, []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64}) || !contains(packets, []byte{20, 10}) || c.event == nil {
		t.Fatal("collection did not wait for acknowledgement", packets)
	}
	before := c.character.Clone()
	node := store.ResourceKey{Kind: store.ResourceCollectionActor, MapID: 60000, NodeID: 33}
	expiry, clockErr := s.Store.ResourceExpiry(ctx, node)
	if c.character.Bag[0].ID != id || c.character.Bag[0].Count != 1 || clockErr != nil || !expiry.After(time.Now()) {
		t.Fatal("reward or cooldown missing")
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].Bag != before.Bag || !stored[0].EventTimers[11020].Equal(before.EventTimers[11020]) {
		t.Fatal("reward did not commit before receipt", err)
	}
	for range 2 {
		if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
	}
	if c.event != nil || c.character.Bag != before.Bag {
		t.Fatal("acknowledgement replay changed reward")
	}
	wire.Reset()
	c.resumeAt = time.Time{}
	if err := s.worldCommand(ctx, c, []byte{20, 1, 33, 0}); err != nil {
		t.Fatal(err)
	}
	packets = wire.packets(t)
	if !contains(packets, []byte{20, 1, 0, 0, 0, 1, 1, 7, 0, 0, 1, 0, 0, 0, 0, 141, 195, 1}) {
		t.Fatal("missing captured cooldown dialogue", packets)
	}
	for _, p := range packets {
		if len(p) > 1 && ((p[0] == 23 && p[1] == 6) || p[0] == 66) {
			t.Fatal("cooldown click granted or reset timer", p)
		}
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil || c.event != nil {
		t.Fatal("cooldown dialogue did not release", err)
	}
	if c.character.Bag != before.Bag || !c.character.EventTimers[11020].Equal(before.EventTimers[11020]) {
		t.Fatal("cooldown click mutated resources")
	}
	// A duplicate completion cannot generate another acquisition.
	for _, p := range wire.packets(t) {
		if bytes.Equal(p, reward) {
			t.Fatal("completion duplicated acquisition")
		}
	}
	// The imported capacity branch must reject before starting a new timer.
	const siblingCollectionActor = 65000
	m := s.Assets.Maps[60000]
	for _, actor := range m.NPCs {
		if actor.ClickID == 33 {
			actor.ClickID = siblingCollectionActor
			m.NPCs = append(m.NPCs, actor)
			break
		}
	}
	s.Assets.Maps[60000] = m
	s.World = world.New(s.Assets)
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 40012, Count: 1}
	}
	c.character.EventTimers = nil
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	c.resumeAt = time.Time{}
	if err := s.worldCommand(ctx, c, []byte{20, 1, byte(siblingCollectionActor & 255), byte(siblingCollectionActor >> 8)}); err != nil {
		t.Fatal(err)
	}
	packets = wire.packets(t)
	if !contains(packets, []byte{20, 1, 0, 0, 0, 1, 1, 7, 0, 0, 1, 0, 0, 0, 0, 142, 195, 1}) {
		t.Fatal("missing inventory-capacity dialogue", packets)
	}
	if len(c.character.EventTimers) != 0 {
		t.Fatal("full inventory started cooldown")
	}
	for _, p := range packets {
		if len(p) > 1 && ((p[0] == 23 && p[1] == 6) || p[0] == 66) {
			t.Fatal("full inventory published reward or timer", p)
		}
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil || c.event != nil {
		t.Fatal("full inventory dialogue did not release", err)
	}
}
