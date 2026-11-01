package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestTeleportScrollRecordAndAtomicTravel(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.Travel.Scrolls = map[uint16]byte{34163: 1}
	s.Assets.Items[34163] = game.ItemDefinition{ID: 34163, Type: 10}
	c.character.Bag[1] = game.Item{ID: 34163, Count: 2}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{74, 1, 2, 1})
	points, err := s.Store.TeleportPoints(context.Background(), store.CharacterRef{Account: c.account.ID, ID: c.character.ID})
	if err != nil || points[1].Map != c.character.Map || points[1].X != c.character.X {
		t.Fatal("record", points, err)
	}
	oldX := c.character.X
	c.character.X += 20 // Already validated pending walking, still on the same map.
	wires[0].Reset()
	tradeDo(t, s, c, []byte{74, 2, 2, 1})
	if c.character.X != oldX || c.character.Bag[1].Count != 1 || c.ready {
		t.Fatal("scroll travel")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != oldX || saved[0].Bag[1].Count != 1 {
		t.Fatal("debit and location not durable", err)
	}
	tradeDo(t, s, c, []byte{12, 1})
	s.Store.Close()
	wires[0].Reset()
	if err := s.dispatch(context.Background(), c, []byte{74, 2, 2, 1}); err == nil || c.character.Bag[1].Count != 1 || wires[0].Len() != 0 {
		t.Fatal("failed SQL published travel", err)
	}
}

func TestBambooDragonflyRejectsUnavailableDestination(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.Travel.BambooItems = []uint16{34169}
	s.Assets.Travel.Destinations = map[byte]assets.TravelDestination{1: {Map: 60000, X: 100, Y: 100}}
	s.Assets.Items[34169] = game.ItemDefinition{ID: 34169, Type: 10}
	c.character.Bag[1] = game.Item{ID: 34169, Count: 1}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{23, 89, 1, 2, 0})
	if c.character.Bag[1].Count != 1 || c.character.Map != 10017 {
		t.Fatal("unavailable destination consumed item")
	}
	if len(wires[0].packets(t)) != 1 {
		t.Fatal("missing rejection")
	}
}

func TestKelanReturnScrollAtomicDebit(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Travel.ReturnScrolls = map[uint16]assets.TravelDestination{30001: {Map: 12000, X: 892, Y: 734}}
	s.Assets.Maps[12000] = assets.Map{ID: 12000}
	s.Assets.Items[30001] = game.ItemDefinition{ID: 30001, Type: 21}
	s.World = world.New(s.Assets)
	c.character.Bag[1] = game.Item{ID: 30001, Count: 2}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.useItemOn(ctx, c, 2, 1, 0); err != nil || c.character.Map != 12000 || c.character.Bag[1].Count != 1 {
		t.Fatal("return", err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Map != 12000 || saved[0].Bag[1].Count != 1 {
		t.Fatal("not durable", err)
	}
	if err := s.dispatch(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Store.Close()
	wires[0].Reset()
	if err := s.useItem(ctx, c, 2); err == nil || c.character.Bag[1].Count != 1 || wires[0].Len() != 0 {
		t.Fatal("failed debit published", err)
	}
}
