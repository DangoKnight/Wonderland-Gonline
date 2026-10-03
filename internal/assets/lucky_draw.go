package assets

import (
	"encoding/json"
	"fmt"
	"math"
	"wonderland-go/internal/game"
)

// Slot is the one-based native presentation position in the ordered catalog.
// Keep presentation order separate from item IDs and probability weights.
type LuckyDrawReward struct {
	ID       uint16 `json:"item_id"`
	Quantity int    `json:"quantity"`
	Weight   int64  `json:"weight"`
	Slot     byte   `json:"slot"`
}
type LuckyDrawPool struct {
	Rewards     []LuckyDrawReward
	TotalWeight int64
}

func ParseLuckyDrawPool(data []byte, items map[uint16]game.ItemDefinition) (LuckyDrawPool, error) {
	var rows []LuckyDrawReward
	if err := json.Unmarshal(data, &rows); err != nil {
		return LuckyDrawPool{}, err
	}
	if rows == nil {
		return LuckyDrawPool{}, fmt.Errorf("null Lucky Draw rewards")
	}
	if len(rows) > game.LuckyDrawMaxRewards {
		return LuckyDrawPool{}, fmt.Errorf("Lucky Draw supports at most %d rewards", game.LuckyDrawMaxRewards)
	}
	var pool LuckyDrawPool
	for i, r := range rows {
		if _, ok := items[r.ID]; !ok || r.ID == 0 || r.Quantity < 1 || r.Quantity > game.GachaMaxRewardQuantity || r.Weight < 1 || int(r.Slot) != i+1 || r.Weight > math.MaxInt64-pool.TotalWeight {
			return LuckyDrawPool{}, fmt.Errorf("invalid Lucky Draw reward item %d slot %d", r.ID, r.Slot)
		}
		pool.TotalWeight += r.Weight
	}
	pool.Rewards = rows
	return pool, nil
}

func (p LuckyDrawPool) RewardForRoll(roll int64) (LuckyDrawReward, bool) {
	if roll < 0 || roll >= p.TotalWeight {
		return LuckyDrawReward{}, false
	}
	for _, reward := range p.Rewards {
		if roll < reward.Weight {
			return reward, true
		}
		roll -= reward.Weight
	}
	return LuckyDrawReward{}, false
}
