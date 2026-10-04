package assets

import (
	"testing"
	"wonderland-go/internal/game"
)

func TestAdminChestPoolValidation(t *testing.T) {
	catalog := &Catalog{Maps: map[uint16]Map{10017: {ID: 10017}}, Items: map[uint16]game.ItemDefinition{42: {ID: 42}}}
	valid := `[{"map_id":10017,"respawn_seconds":60,"rewards":[{"item_id":42,"count":1,"weight":1}]}]`
	pools, err := ParseChestPools([]byte(valid), catalog)
	if err != nil || len(pools) != 1 {
		t.Fatal(pools, err)
	}
	for _, raw := range []string{
		`[{"map_id":10017,"respawn_seconds":60,"rewards":[{"item_id":42,"count":1,"weight":0}]}]`,
		`[{"map_id":10017,"respawn_seconds":0,"rewards":[{"item_id":42,"count":1,"weight":1}]}]`,
		`[{"map_id":10018,"respawn_seconds":60,"rewards":[{"item_id":42,"count":1,"weight":1}]}]`,
		`[{"map_id":10017,"category":"one","respawn_seconds":60,"rewards":[{"item_id":42,"count":1,"weight":1}]},{"map_id":10017,"category":"two","respawn_seconds":60,"rewards":[{"item_id":42,"count":1,"weight":1}]}]`,
	} {
		if _, err := ParseChestPools([]byte(raw), catalog); err == nil {
			t.Fatal("invalid pool accepted", raw)
		}
	}
}
