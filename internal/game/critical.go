package game

import (
	"encoding/json"
	"fmt"
	"math"
)

type CriticalHits struct {
	Multiplier float64
	Chances    map[uint16]int
}

func ParseCriticalHits(b []byte) (CriticalHits, error) {
	var config struct {
		Multiplier float64 `json:"damage_multiplier"`
		Items      []struct {
			ID     uint16 `json:"item_id"`
			Chance int    `json:"chance_percent"`
		} `json:"items"`
	}
	if e := json.Unmarshal(b, &config); e != nil {
		return CriticalHits{}, e
	}
	if len(config.Items) == 0 || config.Multiplier < 1 || config.Multiplier > 10 || math.IsNaN(config.Multiplier) || math.IsInf(config.Multiplier, 0) {
		return CriticalHits{}, fmt.Errorf("invalid critical hit configuration")
	}
	c := CriticalHits{Multiplier: config.Multiplier, Chances: map[uint16]int{}}
	for _, item := range config.Items {
		if item.ID == 0 || item.Chance < 1 || item.Chance > 520 || c.Chances[item.ID] != 0 {
			return CriticalHits{}, fmt.Errorf("invalid critical hit item")
		}
		c.Chances[item.ID] = min(100, item.Chance)
	}
	return c, nil
}
func (c CriticalHits) Chance(equipment [6]uint16) int {
	chance := 0
	for _, id := range equipment {
		chance += c.Chances[id]
	}
	return min(100, chance)
}
func (c CriticalHits) Damage(damage int32, equipment [6]uint16, action string, roll int) (int32, bool) {
	critical := damage > 0 && action == "attack" && roll >= 0 && roll < 100 && roll < c.Chance(equipment)
	if !critical {
		return damage, false
	}
	return int32(math.Min(math.MaxInt32, math.Floor(float64(damage)*c.Multiplier))), true
}
