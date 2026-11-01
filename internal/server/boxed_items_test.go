package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestBoxedItemsAtomicExchangeAndFailure(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	s.Assets.Items[26001] = game.ItemDefinition{ID: 26001, Type: 17}
	s.Assets.Items[11101] = game.ItemDefinition{ID: 11101, Type: 1}
	s.Assets.BoxedItems = map[uint16]assets.BoxedItem{26001: {ItemID: 11101, Quantity: 1}}
	c.character.Bag[0] = game.Item{ID: 26001, Count: 2}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if handled, err := s.openBoxedItem(context.Background(), c, 1); !handled || err != nil {
		t.Fatal(handled, err)
	}
	if c.character.Bag[0].Count != 1 || c.character.Bag[1].ID != 11101 {
		t.Fatal(c.character.Bag)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Bag != c.character.Bag {
		t.Fatal("exchange not durable", err)
	}
	for i := range c.character.Bag {
		if c.character.Bag[i].Empty() {
			c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
		}
	}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	// A stack of two boxes cannot release its occupied cell.
	c.character.Bag[0].Count = 2
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if _, err := s.openBoxedItem(context.Background(), c, 1); err != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("full bag consumed box", err)
	}
	c.character.Bag[2] = game.Item{}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	before = c.character.Clone()
	wire.Reset()
	s.Store.Close()
	if _, err := s.openBoxedItem(context.Background(), c, 1); err == nil || !reflect.DeepEqual(before, *c.character) || wire.Len() != 0 {
		t.Fatal("failed save published", err)
	}
}
