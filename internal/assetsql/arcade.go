package assetsql

import (
	_ "embed"
	"encoding/json"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

//go:embed arcade_defaults.json
var arcadeDefaults []byte

// The seed is used only during explicit creation/migration. Existing SQL edits
// are never replaced on restart or when migration is repeated.
func defaultArcades(items map[uint16]game.ItemDefinition) ([]assets.ArcadeGame, error) {
	var result []assets.ArcadeGame
	if err := json.Unmarshal(arcadeDefaults, &result); err != nil {
		return nil, err
	}
	for i, g := range result {
		for _, r := range g.Rewards {
			if _, ok := items[r.ItemID]; !ok {
				result[i].Enabled = false
			}
		}
	}
	return result, assets.ValidateArcades(result, items)
}
