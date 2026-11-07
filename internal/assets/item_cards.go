package assets

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/internal/game"
)

const MaximumCardRewards = 255 // Native Gec.Dat reward count is one byte.

type CardReward struct {
	ItemID   uint16 `json:"item_id"`
	Quantity byte   `json:"quantity"`
}
type ItemCard struct {
	MaleItem   uint16       `json:"male_item,omitempty"`
	FemaleItem uint16       `json:"female_item,omitempty"`
	Rewards    []CardReward `json:"rewards"`
}

func ParseItemCards(raw []byte, items map[uint16]game.ItemDefinition) (map[uint16]ItemCard, error) {
	var rows []struct {
		ID uint16 `json:"card_id"`
		ItemCard
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	result := map[uint16]ItemCard{}
	for _, row := range rows {
		if _, exists := result[row.ID]; exists || row.ID == 0 || len(row.Rewards) == 0 || len(row.Rewards) > MaximumCardRewards {
			return nil, fmt.Errorf("invalid item card %d", row.ID)
		}
		if _, ok := items[row.ID]; !ok {
			return nil, fmt.Errorf("missing card %d", row.ID)
		}
		seen := map[uint16]bool{}
		for _, r := range row.Rewards {
			if _, ok := items[r.ItemID]; !ok || r.ItemID == row.ID || r.Quantity == 0 || seen[r.ItemID] {
				return nil, fmt.Errorf("invalid card reward %d:%d", row.ID, r.ItemID)
			}
			seen[r.ItemID] = true
		}
		if (row.MaleItem != 0 || row.FemaleItem != 0) && (!seen[row.MaleItem] || !seen[row.FemaleItem]) {
			return nil, fmt.Errorf("incomplete gender card %d", row.ID)
		}
		result[row.ID] = row.ItemCard
	}
	return result, nil
}
