package assets

import "fmt"

const (
	WorldPKChestWood      = "wood"
	WorldPKChestSilver    = "silver"
	WorldPKChestGold      = "gold"
	WorldPKRewardItem     = "item"
	WorldPKRewardCurrency = "currency"
)

// WorldPKRewardDefinitions records authored possibilities, including unresolved
// mobile references. A zero reward weight is unknown, not an enabled drop.
// No runtime grant policy may infer missing weights, quantities or item IDs.
type WorldPKRewardDefinitions struct {
	Model  string                `json:"model"`
	Source string                `json:"source"`
	Chests []WorldPKChestRewards `json:"chests"`
}

type WorldPKChestRewards struct {
	Tier    string          `json:"tier"`
	Weight  int             `json:"weight"`
	Rewards []WorldPKReward `json:"rewards"`
}

type WorldPKReward struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	ItemID   uint16 `json:"item_id"`
	Currency string `json:"currency"`
	Minimum  int    `json:"minimum"`
	Maximum  int    `json:"maximum"`
	Weight   int    `json:"weight"`
	// Value ranges describe capsule denominations, never inventory quantities.
	ValueMinimum int    `json:"value_minimum"`
	ValueMaximum int    `json:"value_maximum"`
	Unresolved   string `json:"unresolved"`
}

func ValidateWorldPKRewards(v WorldPKRewardDefinitions) error {
	// An intentionally empty definition disables all pools and is preserved.
	if len(v.Chests) == 0 {
		return nil
	}
	if v.Model == "" || v.Source == "" {
		return fmt.Errorf("World PK rewards require model and source")
	}
	seen := map[string]bool{}
	for _, chest := range v.Chests {
		if (chest.Tier != WorldPKChestWood && chest.Tier != WorldPKChestSilver && chest.Tier != WorldPKChestGold) || seen[chest.Tier] || chest.Weight <= 0 {
			return fmt.Errorf("invalid or duplicate World PK chest %q", chest.Tier)
		}
		seen[chest.Tier] = true
		keys := map[string]bool{}
		for _, reward := range chest.Rewards {
			if reward.Key == "" || keys[reward.Key] || reward.Weight < 0 || reward.Minimum < 0 || reward.Maximum < reward.Minimum || reward.ValueMinimum < 0 || reward.ValueMaximum < reward.ValueMinimum {
				return fmt.Errorf("invalid World PK reward %q in %s", reward.Key, chest.Tier)
			}
			keys[reward.Key] = true
			if reward.Kind != WorldPKRewardItem && reward.Kind != WorldPKRewardCurrency {
				return fmt.Errorf("invalid World PK reward kind %q", reward.Kind)
			}
			if (reward.Kind == WorldPKRewardItem && reward.Currency != "") || (reward.Kind == WorldPKRewardCurrency && (reward.Currency == "" || reward.ItemID != 0)) {
				return fmt.Errorf("invalid World PK reward delivery %q", reward.Key)
			}
			if (reward.Weight == 0 || reward.Minimum == 0 || (reward.Kind == WorldPKRewardItem && reward.ItemID == 0)) && reward.Unresolved == "" {
				return fmt.Errorf("World PK reward %q needs unresolved evidence", reward.Key)
			}
		}
	}
	return nil
}
