package game

import (
	"errors"
	"testing"
)

func TestExchangePreservesItemsAndGold(t *testing.T) {
	a, b := Character{Gold: 100}, Character{Gold: 200}
	a.Bag[0] = Item{ID: 32176, Count: 10, Damage: 2}
	a.Bag[0].Metadata[6] = 7
	b.Bag[0] = Item{ID: 32176, Count: 20, Damage: 3}
	b.Bag[0].Metadata[6] = 9
	offers := [2]TradeOffer{{Gold: 20, Items: []TradeItem{{Slot: 1, Count: 3, Item: a.Bag[0]}}}, {Gold: 50, Items: []TradeItem{{Slot: 1, Count: 4, Item: b.Bag[0]}}}}
	result, err := Exchange(a, b, offers, map[uint16]ItemDefinition{32176: {ID: 32176, Type: 23}})
	if err != nil {
		t.Fatal(err)
	}
	left, right := result.Characters[0], result.Characters[1]
	if left.Gold != 130 || right.Gold != 170 || left.Bag[0].Count != 7 || right.Bag[0].Count != 16 || left.Bag[1].Count != 4 || right.Bag[1].Count != 3 {
		t.Fatal("exchange quantities", left.Gold, right.Gold, left.Bag[:2], right.Bag[:2])
	}
	if left.Bag[1].Damage != 3 || left.Bag[1].Metadata[6] != 9 || right.Bag[1].Damage != 2 || right.Bag[1].Metadata[6] != 7 {
		t.Fatal("metadata lost")
	}
	if a.Bag[0].Count != 10 || b.Bag[0].Count != 20 || a.Gold != 100 || b.Gold != 200 {
		t.Fatal("planning changed original state")
	}
}

func TestExchangeFullBagsCanSwap(t *testing.T) {
	a, b := Character{}, Character{}
	for i := range a.Bag {
		a.Bag[i] = Item{ID: 100, Count: 1}
		b.Bag[i] = Item{ID: 200, Count: 1}
	}
	offers := [2]TradeOffer{{Items: []TradeItem{{Slot: 50, Count: 1, Item: a.Bag[49]}}}, {Items: []TradeItem{{Slot: 50, Count: 1, Item: b.Bag[49]}}}}
	items := map[uint16]ItemDefinition{100: {ID: 100}, 200: {ID: 200}}
	r, err := Exchange(a, b, offers, items)
	if err != nil || r.Characters[0].Bag[49].ID != 200 || r.Characters[1].Bag[49].ID != 100 {
		t.Fatal("full bag swap", err)
	}
	offers[1] = TradeOffer{}
	if _, err := Exchange(a, b, offers, items); !errors.Is(err, ErrInventoryFull) {
		t.Fatal("full recipient received partial transfer", err)
	}
}

func TestExchangeRejectsStaleDuplicateAndOverflowOffers(t *testing.T) {
	a, b := Character{Gold: 100}, Character{Gold: 999999}
	a.Bag[0] = Item{ID: 32176, Count: 10}
	item := TradeItem{Slot: 1, Count: 3, Item: a.Bag[0]}
	catalog := map[uint16]ItemDefinition{32176: {ID: 32176, Type: 23}}
	for _, offer := range []TradeOffer{{Gold: 101}, {Gold: 1}, {Items: []TradeItem{item, item}}, {Items: []TradeItem{{Slot: 51, Count: 1}}}, {Items: []TradeItem{{Slot: 1, Count: 0, Item: a.Bag[0]}}}} {
		if _, err := Exchange(a, b, [2]TradeOffer{offer, {}}, catalog); err == nil {
			t.Fatal("invalid offer accepted", offer)
		}
	}
	changed := a.Clone()
	changed.Bag[0].Metadata[10] = 1
	if _, err := Exchange(changed, b, [2]TradeOffer{{Items: []TradeItem{item}}, {}}, catalog); !errors.Is(err, ErrTradeChanged) {
		t.Fatal("same ID replaced after offer", err)
	}
	if _, err := Exchange(a, b, [2]TradeOffer{{Items: []TradeItem{item}}, {}}, nil); err == nil {
		t.Fatal("unknown item transferred")
	}
}
