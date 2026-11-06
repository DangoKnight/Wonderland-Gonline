package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestCollectionGlobalCooldownAndIndependentDurations(t *testing.T) {
	s, c, _, ev := waterGatheringFixture(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	s.Assets.ResourceRespawns = []assets.ResourceRespawn{{Kind: game.ResourceCollectionActor, MapID: 60001, NodeID: 88, CooldownSeconds: 5}, {Kind: game.ResourceCollectionActor, MapID: 60001, NodeID: 89, CooldownSeconds: 20}}
	// Legacy character timer history must not block another source sharing its ID.
	c.character.EventTimers = map[uint16]time.Time{11009: now.Add(time.Hour)}
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
	if handled, err := s.tryWaterGathering(ctx, c, 88, ev, 1, now); !handled || err != nil {
		t.Fatal(handled, err)
	}
	account, err := s.Store.Register(ctx, "collectorb", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	second := c.character.Clone()
	second.ID, second.Name = account.CharacterID(1), "CollectorB"
	second.Bag, second.Quests = game.Inventory{}, nil
	if err := s.Store.CreateCharacter(ctx, account, second); err != nil {
		t.Fatal(err)
	}
	b := &Session{account: account, character: &second, view: world.NewView(), conn: &captureConn{}, pets: newPetRoster()}
	// A different account still sees the same server-wide claim.
	key := store.ResourceKey{Kind: store.ResourceCollectionActor, MapID: 60001, NodeID: 88}
	if branch, err := s.findCollectionBranch(ctx, b, key, ev, now); err != nil || branch != 0 {
		t.Fatal("another session bypassed cooldown", branch, err)
	}
	sibling := key
	sibling.NodeID = 89
	if branch, err := s.findCollectionBranch(ctx, b, sibling, ev, now); err != nil || branch != 1 {
		t.Fatal("shared legacy timer blocked another spot", branch, err)
	}
	if handled, err := s.tryWaterGathering(ctx, b, 89, ev, 1, now); !handled || err != nil {
		t.Fatal(handled, err)
	}
	if branch, err := s.findCollectionBranch(ctx, b, key, ev, now.Add(6*time.Second)); err != nil || branch != 1 {
		t.Fatal("short timer did not expire", branch, err)
	}
	if branch, err := s.findCollectionBranch(ctx, b, sibling, ev, now.Add(6*time.Second)); err != nil || branch != 0 {
		t.Fatal("short timer reset long timer", branch, err)
	}
}

func TestGroundCooldownDurationsAndRestore(t *testing.T) {
	s, players, _ := worldFixture(t)
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Items: []assets.GroundItem{{ClickID: 5, ItemID: 32176, X: 1042, Y: 1075}, {ClickID: 6, ItemID: 32176, X: 1042, Y: 1075}}}
	s.Assets.ResourceRespawns = []assets.ResourceRespawn{{Kind: game.ResourceGroundItem, MapID: 10017, NodeID: 5, CooldownSeconds: 10}, {Kind: game.ResourceGroundItem, MapID: 10017, NodeID: 6, CooldownSeconds: 30}}
	s.World = world.New(s.Assets)
	ctx := context.Background()
	a, b := players[0], players[1]
	for _, c := range []*Session{a, b} {
		if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.worldCommand(ctx, a, []byte{23, 2, 5}); err != nil {
		t.Fatal(err)
	}
	before := b.character.Bag
	if err := s.worldCommand(ctx, b, []byte{23, 2, 5}); err != nil || b.character.Bag != before {
		t.Fatal("second player picked consumed item", err)
	}
	if err := s.worldCommand(ctx, b, []byte{23, 2, 6}); err != nil {
		t.Fatal("neighboring item blocked", err)
	}
	// A fresh world (server restart) restores both per-item clocks before map info.
	s.World = world.New(s.Assets)
	if _, err := s.arrivalPackets(a.character.Clone(), world.NewView(), newPetRoster(), 0); err != nil {
		t.Fatal(err)
	}
	for _, slot := range []byte{5, 6} {
		if _, ok := s.World.GroundAt(10017, slot); ok {
			t.Fatal("fresh world restored consumed item early", slot)
		}
	}
	s.respawnGround(time.Now().Add(15 * time.Second))
	if _, ok := s.World.GroundAt(10017, 5); !ok {
		t.Fatal("short item did not respawn")
	}
	if _, ok := s.World.GroundAt(10017, 6); ok {
		t.Fatal("short item respawn reset longer item")
	}
}
