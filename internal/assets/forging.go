package assets

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/internal/game"
)

const (
	minForgingFamilyItems = 2
	maxForgingFamilyItems = 11
)

type ForgeUpgrade struct {
	Next    uint16
	Scrolls byte
}
type Forging struct {
	Upgrades   map[uint16]ForgeUpgrade
	PointItems map[uint16]bool
}

// ParseForging preserves family order: level zero costs one scroll; the terminal
// family item has no successor. Unknown item IDs are checked at use, as in C#.
func ParseForging(raw []byte, items map[uint16]game.ItemDefinition) (Forging, error) {
	var config struct {
		Families   [][]uint16 `json:"families"`
		PointItems []uint16   `json:"point_items"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return Forging{}, err
	}
	if len(config.Families) == 0 || config.PointItems == nil {
		return Forging{}, fmt.Errorf("missing forging configuration")
	}
	if _, ok := items[game.StrongScrollItemID]; !ok {
		return Forging{}, fmt.Errorf("missing Strong Scroll item")
	}
	result := Forging{Upgrades: map[uint16]ForgeUpgrade{}, PointItems: map[uint16]bool{}}
	for _, family := range config.Families {
		if len(family) < minForgingFamilyItems || len(family) > maxForgingFamilyItems {
			return Forging{}, fmt.Errorf("invalid forging family length")
		}
		for index, id := range family {
			if _, exists := result.Upgrades[id]; id == 0 || exists {
				return Forging{}, fmt.Errorf("duplicate or empty forging item")
			}
			upgrade := ForgeUpgrade{Scrolls: byte(index + 1)}
			if index+1 < len(family) {
				upgrade.Next = family[index+1]
			}
			result.Upgrades[id] = upgrade
		}
	}
	for _, id := range config.PointItems {
		if id == 0 || result.PointItems[id] {
			return Forging{}, fmt.Errorf("duplicate or empty point-forging item")
		}
		result.PointItems[id] = true
	}
	return result, nil
}
