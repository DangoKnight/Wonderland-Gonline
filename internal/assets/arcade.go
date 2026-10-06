package assets

import (
	"fmt"
	"math"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	arcadeMinimumCooldownMilliseconds = 1000
	arcadeMaximumCooldownMilliseconds = 60000
)

// ArcadeGame is a server-owned paid sport definition. Index is the native
// executable's prize-table index; it is unrelated to selection probability.
type ArcadeGame struct {
	Kind                 byte           `json:"kind"`
	Enabled              bool           `json:"enabled"`
	PointCost            int64          `json:"point_cost"`
	TokenCount           byte           `json:"token_count"`
	CooldownMilliseconds uint32         `json:"cooldown_milliseconds"`
	Rewards              []ArcadeReward `json:"rewards"`
}
type ArcadeReward struct {
	Index    byte    `json:"index"`
	ItemID   uint16  `json:"item_id"`
	Quantity byte    `json:"quantity"`
	Weight   int64   `json:"weight"`
	Reels    [3]byte `json:"reels"`
}

func (g ArcadeGame) TotalWeight() int64 {
	var total int64
	for _, r := range g.Rewards {
		total += r.Weight
	}
	return total
}
func (g ArcadeGame) RewardForRoll(roll int64) (ArcadeReward, bool) {
	if roll < 0 || roll >= g.TotalWeight() {
		return ArcadeReward{}, false
	}
	for _, r := range g.Rewards {
		if roll < r.Weight {
			return r, true
		}
		roll -= r.Weight
	}
	return ArcadeReward{}, false
}
func ArcadeKindSupported(kind byte) bool {
	switch kind {
	case protocol.ArcadeEgg, protocol.ArcadeSlots, protocol.ArcadeSlots2, protocol.ArcadeSlots3, protocol.ArcadeEgg2:
		return true
	}
	return false
}

// ValidateArcades also constrains the native presentation domain. Missing item
// definitions are allowed only for disabled machines in minimal asset imports.
func ValidateArcades(games []ArcadeGame, items map[uint16]game.ItemDefinition) error {
	kinds := map[byte]bool{}
	for _, g := range games {
		if !ArcadeKindSupported(g.Kind) || kinds[g.Kind] || g.PointCost < 1 || g.PointCost > game.MaxMallPoints || g.CooldownMilliseconds < arcadeMinimumCooldownMilliseconds || g.CooldownMilliseconds > arcadeMaximumCooldownMilliseconds {
			return fmt.Errorf("invalid arcade game %d", g.Kind)
		}
		kinds[g.Kind] = true
		indexes := map[byte]bool{}
		var total int64
		for _, r := range g.Rewards {
			if r.ItemID != ArcadePrizeItem(g.Kind, r.Index) || r.Index >= protocol.ArcadePrizeCount || ((g.Kind == protocol.ArcadeEgg || g.Kind == protocol.ArcadeEgg2) && r.Index == 0) || indexes[r.Index] || r.ItemID == 0 || r.Quantity == 0 || r.Weight < 0 || r.Weight > math.MaxInt64-total {
				return fmt.Errorf("invalid arcade reward %d:%d", g.Kind, r.Index)
			}
			if _, ok := items[r.ItemID]; g.Enabled && !ok {
				return fmt.Errorf("unknown arcade reward item %d", r.ItemID)
			}
			if g.Kind != protocol.ArcadeEgg && g.Kind != protocol.ArcadeEgg2 {
				for _, symbol := range r.Reels {
					if symbol < 1 || symbol > protocol.ArcadeReelSymbols {
						return fmt.Errorf("invalid arcade reel %d", symbol)
					}
				}
			}
			indexes[r.Index] = true
			total += r.Weight
		}
		if g.Enabled && total == 0 {
			return fmt.Errorf("empty arcade game %d", g.Kind)
		}
	}
	return nil
}

// The native executable displays a fixed item for each index. SQL weights and
// quantities are editable, but changing these identities would mislead aLogin.
// WLRI ca19ee087b60: PTR_DAT_004c9944/004ca8e4/004c9888/004ca80c/004ca194.
func ArcadePrizeItem(kind, index byte) uint16 {
	tables := map[byte][protocol.ArcadePrizeCount]uint16{
		protocol.ArcadeEgg:    {0, 32162, 46004, 33044, 34089, 30553, 22156, 30065, 30066, 30556, 22166},
		protocol.ArcadeEgg2:   {0, 32162, 51142, 33044, 34122, 33053, 22894, 21601, 21707, 34155, 22115},
		protocol.ArcadeSlots:  {30063, 61044, 61039, 61037, 61035, 61033, 61032, 61031, 61030, 61028, 61026},
		protocol.ArcadeSlots2: {32095, 34123, 34116, 34115, 34114, 34112, 34111, 34110, 34109, 34108, 34107},
		protocol.ArcadeSlots3: {30063, 62011, 62010, 62007, 62006, 62005, 62004, 62003, 62002, 62001, 62000},
	}
	if index >= protocol.ArcadePrizeCount {
		return 0
	}
	return tables[kind][index]
}
