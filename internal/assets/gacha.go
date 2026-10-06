package assets

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/internal/game"
)

type GachaReward struct {
	ID       uint16 `json:"item_id"`
	Weight   int    `json:"weight"`
	Quantity int    `json:"quantity"`
}
type GachaPack struct {
	ID      uint16        `json:"item_id"`
	Rewards []GachaReward `json:"rewards"`
}

// GachaPackIssue identifies an intact pool unavailable in this item catalog.
type GachaPackIssue struct {
	ID           uint16
	MissingItems []uint16
}

func (i GachaPackIssue) String() string {
	return fmt.Sprintf("gacha pack %d disabled: missing item definitions %v", i.ID, i.MissingItems)
}

// ParseGachaPacks is the strict offline validator: every configured item must
// exist. Pack membership comes from the authored table, not a Go ID allowlist.
func ParseGachaPacks(data []byte, items map[uint16]game.ItemDefinition) (map[uint16]GachaPack, error) {
	entries, err := parseGachaDefinitions(data)
	if err != nil {
		return nil, err
	}
	packs := map[uint16]GachaPack{}
	for _, p := range entries {
		if _, ok := items[p.ID]; !ok {
			return nil, fmt.Errorf("unknown gacha pack %d", p.ID)
		}
		for _, r := range p.Rewards {
			if _, ok := items[r.ID]; !ok {
				return nil, fmt.Errorf("invalid reward for gacha pack %d", p.ID)
			}
		}
		packs[p.ID] = p
	}
	return packs, nil
}

// ParseCompatibleGachaPacks validates the entire authored table before selecting
// complete pools for the current item catalog. Missing definitions disable whole
// pools; rewards are never removed or reweighted. Malformed data enables nothing.
func ParseCompatibleGachaPacks(data []byte, items map[uint16]game.ItemDefinition) (map[uint16]GachaPack, []GachaPackIssue, error) {
	entries, err := parseGachaDefinitions(data)
	if err != nil {
		return nil, nil, err
	}
	packs := map[uint16]GachaPack{}
	var warnings []GachaPackIssue
	for _, p := range entries {
		var missing []uint16
		seen := map[uint16]bool{}
		check := func(id uint16) {
			if _, ok := items[id]; !ok && !seen[id] {
				missing = append(missing, id)
				seen[id] = true
			}
		}
		check(p.ID)
		for _, r := range p.Rewards {
			check(r.ID)
		}
		if len(missing) != 0 {
			warnings = append(warnings, GachaPackIssue{ID: p.ID, MissingItems: missing})
			continue
		}
		packs[p.ID] = p
	}
	return packs, warnings, nil
}

func parseGachaDefinitions(data []byte) ([]GachaPack, error) {
	var entries []GachaPack
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		return nil, fmt.Errorf("null gacha table")
	}
	seen := map[uint16]bool{}
	for _, p := range entries {
		if p.ID == 0 || p.ID == game.UnavailableLuckyPackID || len(p.Rewards) == 0 || len(p.Rewards) > game.GachaMaxRewards {
			return nil, fmt.Errorf("invalid gacha pack %d", p.ID)
		}
		if seen[p.ID] {
			return nil, fmt.Errorf("duplicate gacha pack %d", p.ID)
		}
		seen[p.ID] = true
		weight := 0
		for _, r := range p.Rewards {
			if r.ID == 0 || r.Weight <= 0 || r.Weight > game.GachaWeightTotal || r.Quantity <= 0 || r.Quantity > game.GachaMaxRewardQuantity {
				return nil, fmt.Errorf("invalid reward for gacha pack %d", p.ID)
			}
			weight += r.Weight
		}
		if weight != game.GachaWeightTotal {
			return nil, fmt.Errorf("gacha pack %d weights must sum to %d", p.ID, game.GachaWeightTotal)
		}
	}
	return entries, nil
}

// RewardForRoll preserves the authored interval and preview order.
func (p GachaPack) RewardForRoll(roll int) (GachaReward, bool) {
	if roll < 0 || roll >= game.GachaWeightTotal {
		return GachaReward{}, false
	}
	for _, r := range p.Rewards {
		if roll < r.Weight {
			return r, true
		}
		roll -= r.Weight
	}
	return GachaReward{}, false
}

// IsGachaPack also recognizes unavailable native packs so item use retains them.
func (c *Catalog) IsGachaPack(id uint16) bool {
	_, configured := c.GachaPacks[id]
	_, disabled := c.UnavailableGachaPacks[id]
	return configured || disabled || game.IsGachaPack(id)
}

func (c *Catalog) GachaAvailable(id uint16) bool {
	if !c.IsGachaPack(id) {
		return true
	}
	if id == game.UnavailableLuckyPackID {
		return false
	}
	p, ok := c.GachaPacks[id]
	return ok && len(p.Rewards) > 0
}
