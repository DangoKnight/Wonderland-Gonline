package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-go/internal/game"
)

func TestNPCItemAcquisitionCapacityPreservesRewardsAndAllowsRetry(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	for i := range next.Bag {
		next.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	next.Bag[0] = game.Item{ID: 32176, Count: game.MaxItemStack - 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	wire.Reset()
	start := func() error {
		ev := s.Assets.Maps[10017].Events[0]
		es := &eventSession{mapID: 10017, click: 5, ev: &ev, branch: 1}
		c.event = es
		return s.advance(ctx, c, es)
	}
	if err := start(); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag != before.Bag || c.character.Gold != before.Gold || !reflect.DeepEqual(c.character.Quests, before.Quests) {
		t.Fatal("failed acquisition changed inventory, gold or quest state")
	}
	if !contains(wire.packets(t), headBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")) {
		t.Fatal("failed acquisition did not report insufficient capacity")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before.Bag || chars[0].Gold != before.Gold || !reflect.DeepEqual(chars[0].Quests, before.Quests) {
		t.Fatal("failed acquisition persisted partial rewards", err)
	}
	next = c.character.Clone()
	next.Bag[game.BagSize-1] = game.Item{}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := start(); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[0].Count != game.MaxItemStack || c.character.Bag[game.BagSize-1] != (game.Item{ID: 32176, Count: 1}) || c.character.Gold != before.Gold+100 || c.character.Quests[900].Step != 1 {
		t.Fatal("reward was not available after freeing sufficient capacity")
	}
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Gold != c.character.Gold || chars[0].Quests[900].Step != 1 {
		t.Fatal("successful retry was not persisted", err)
	}
}
