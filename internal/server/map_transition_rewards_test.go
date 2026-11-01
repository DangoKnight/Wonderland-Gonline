package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestLauraExitRewardAtomicReplayAndCapacity(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	s.Assets.MapTransitions = []assets.MapTransition{{FromMap: 10001, Entry: 1, Destination: assets.TravelDestination{Map: 10000, X: 1892, Y: 1655}, Quest: 10035, Talk: 30126, Items: []assets.BoxedItem{{ItemID: 36002, Quantity: 1}, {ItemID: 34058, Quantity: 1}}, SkipIfOwned: []uint16{36002}}}
	s.Assets.Items[36002] = game.ItemDefinition{ID: 36002}
	s.Assets.Items[34058] = game.ItemDefinition{ID: 34058}
	s.Assets.Maps[10000] = assets.Map{ID: 10000}
	s.Assets.Maps[10001] = assets.Map{ID: 10001}
	s.World = world.New(s.Assets)
	c.character.Map = 10001
	c.character.Bag = game.Inventory{}
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	c.character.Bag[0] = game.Item{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.teleport(ctx, c, world.Destination{Map: 10000, X: 1892, Y: 1655}, 1); err != nil || c.character.Map != 10001 || c.character.Quests[10035].ID != 0 || !c.character.Bag[0].Empty() {
		t.Fatal("partial/full-bag gift", err)
	}
	c.character.Bag[1] = game.Item{}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.teleport(ctx, c, world.Destination{Map: 10000, X: 1892, Y: 1655}, 1); err != nil || c.character.Quests[10035].State != game.Completed || c.character.Bag[0].ID != 36002 || c.character.Bag[1].ID != 34058 {
		t.Fatal(c.character.Bag, err)
	}
	if !contains(wire.packets(t), []byte{20, 1, 0, 0, 0, 1, 1, 3, 0, 0, 1, 0, 0, 0, 0, 0xae, 0x75, 0}) {
		t.Fatal("Laura dialogue")
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Map != 10000 || saved[0].Quests[10035].State != game.Completed {
		t.Fatal("not durable", err)
	}
	c.character.Map = 10001
	c.character.Bag[0] = game.Item{}
	c.character.Bag[1] = game.Item{}
	if err = s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err = s.teleport(ctx, c, world.Destination{Map: 10000}, 1); err != nil || !c.character.Bag[0].Empty() {
		t.Fatal("replayed gift", err)
	}
}
