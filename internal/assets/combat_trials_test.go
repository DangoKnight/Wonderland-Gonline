package assets

import (
	"encoding/json"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestCombatTrialsValidation(t *testing.T) {
	c := &Catalog{Items: map[uint16]game.ItemDefinition{10002: {}}, NPCs: map[uint16]NPC{42: {ID: 42, HP: 100, Level: 5}}, Maps: map[uint16]Map{10017: {ID: 10017}}}
	base := CombatTrial{Stage: 1, Name: "Aries", Map: 10017, Guardian: 42, HP: 15000, Attack: 450, Reward: 10002, Count: 1}
	for _, change := range []func(*CombatTrial){nil, func(v *CombatTrial) { v.Stage = 0 }, func(v *CombatTrial) { v.Stage = 13 }, func(v *CombatTrial) { v.Guardian = 1001 }, func(v *CombatTrial) { v.Reward = 0 }, func(v *CombatTrial) { v.Map = 0 }, func(v *CombatTrial) { v.HP = 0 }, func(v *CombatTrial) { v.Attack = -1 }, func(v *CombatTrial) { v.Count = 0 }, func(v *CombatTrial) { v.Name = "" }} {
		trial := base
		if change != nil {
			change(&trial)
		}
		raw, _ := json.Marshal([]CombatTrial{trial})
		_, err := ParseCombatTrials(raw, c)
		if (err == nil) != (change == nil) {
			t.Fatal("trial validation", trial, err)
		}
	}
	raw, _ := json.Marshal([]CombatTrial{base, base})
	if _, err := ParseCombatTrials(raw, c); err == nil {
		t.Fatal("duplicate stage accepted")
	}
}
