package assets

import (
	"encoding/json"
	"fmt"
	"math"
	"wonderland-go/internal/game"
)

const CombatTrialsAsset = "combat_trials.json"
const MaxPalaceStages = 12

// CombatTrial is an optional SQL-authored stage. The reference's guardian IDs
// 1001..1012 do not exist in WLRI and its chest IDs are vehicle capsules, so
// these identities must be supplied and validated against the active catalog.
type CombatTrial struct {
	Stage    byte   `json:"stage"`
	Name     string `json:"name"`
	Map      uint16 `json:"map_id"`
	Guardian uint16 `json:"guardian_id"`
	HP       int    `json:"hp"`
	Attack   int    `json:"attack"`
	Reward   uint16 `json:"reward_item_id"`
	Count    byte   `json:"reward_count"`
}

func ParseCombatTrials(raw []byte, c *Catalog) ([]CombatTrial, error) {
	var trials []CombatTrial
	if err := json.Unmarshal(raw, &trials); err != nil {
		return nil, err
	}
	seen := map[byte]bool{}
	for _, t := range trials {
		npc, known := c.NPCs[t.Guardian]
		_, item := c.Items[t.Reward]
		_, location := c.Maps[t.Map]
		if t.Stage == 0 || t.Stage > MaxPalaceStages || seen[t.Stage] || t.Name == "" || !known || npc.HP == 0 || npc.Level == 0 || !item || !location || t.HP < 1 || t.HP > math.MaxInt32 || t.Attack < 1 || t.Attack > math.MaxInt32 || t.Count < 1 || t.Count > game.MaxItemStack {
			return nil, fmt.Errorf("invalid combat trial stage %d", t.Stage)
		}
		seen[t.Stage] = true
	}
	return trials, nil
}
