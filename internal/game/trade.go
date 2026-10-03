package game

import "errors"

var ErrTradeChanged = errors.New("offered items or gold have changed")
var ErrTradeGoldLimit = errors.New("trade would exceed the gold limit")

// TradeItem captures an exact bag record when the offer is displayed. Metadata
// and damage are part of its identity; replacing it with the same ID is not enough.
type TradeItem struct {
	Slot  byte
	Count byte
	Item  Item
}

type TradeOffer struct {
	Gold  uint32
	Items []TradeItem
}

type TradeResult struct {
	Characters [2]Character
	Additions  [2][]Addition
}

// Exchange plans both sides without changing either input. Outgoing items are
// removed first, allowing a swap between full bags. No partial grant is accepted.
func Exchange(a, b Character, offers [2]TradeOffer, items map[uint16]ItemDefinition) (TradeResult, error) {
	result := TradeResult{Characters: [2]Character{a.Clone(), b.Clone()}}
	for i := range offers {
		c := &result.Characters[i]
		offer := offers[i]
		if offer.Gold > c.Gold {
			return TradeResult{}, ErrTradeChanged
		}
		total := uint64(c.Gold-offer.Gold) + uint64(offers[1-i].Gold)
		if total > MaxGold {
			return TradeResult{}, ErrTradeGoldLimit
		}
		c.Gold = uint32(total)
		seen := map[byte]bool{}
		for _, it := range offer.Items {
			if it.Slot < 1 || it.Slot > BagSize || it.Count == 0 || seen[it.Slot] {
				return TradeResult{}, ErrTradeChanged
			}
			seen[it.Slot] = true
			current := c.Bag[it.Slot-1]
			if current.Empty() || current != it.Item || it.Count > current.Count {
				return TradeResult{}, ErrTradeChanged
			}
			if _, known := items[current.ID]; !known {
				return TradeResult{}, ErrTradeChanged
			}
			if err := c.Bag.Remove(it.Slot, it.Count); err != nil {
				return TradeResult{}, err
			}
		}
	}
	for i := range offers {
		recipient := &result.Characters[1-i]
		for _, it := range offers[i].Items {
			adds, err := recipient.Bag.Grant(it.Item, int(it.Count), items[it.Item.ID].StackLimit())
			if err != nil {
				return TradeResult{}, err
			}
			result.Additions[1-i] = append(result.Additions[1-i], adds...)
		}
	}
	for i := range result.Characters {
		result.Characters[i].NormalizeVehicle(items)
	}
	return result, nil
}
