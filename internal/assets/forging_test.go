package assets

import (
	"testing"
	"wonderland-go/internal/game"
)

func TestForgingFamiliesAndValidation(t *testing.T) {
	items := map[uint16]game.ItemDefinition{30101: {ID: 30101}}
	config, err := ParseForging([]byte(`{"families":[[100,101,102]],"point_items":[200]}`), items)
	if err != nil || config.Upgrades[100] != (ForgeUpgrade{Next: 101, Scrolls: 1}) || config.Upgrades[101] != (ForgeUpgrade{Next: 102, Scrolls: 2}) || config.Upgrades[102].Next != 0 || !config.PointItems[200] {
		t.Fatal(config, err)
	}
	for _, raw := range []string{`invalid`, `{}`, `{"families":[[100]],"point_items":[]}`, `{"families":[[100,101],[101,102]],"point_items":[]}`, `{"families":[[0,100]],"point_items":[]}`, `{"families":[[100,101]],"point_items":[200,200]}`, `{"families":[[100,101]],"point_items":[0]}`, `{"families":[[100,101]],"point_items":null}`, `{"families":[[1,2,3,4,5,6,7,8,9,10,11,12]],"point_items":[]}`} {
		if parsed, err := ParseForging([]byte(raw), items); err == nil || parsed.Upgrades != nil || parsed.PointItems != nil {
			t.Fatal("invalid configuration accepted", raw, err)
		}
	}
	if _, err := ParseForging([]byte(`{"families":[[100,101]],"point_items":[]}`), nil); err == nil {
		t.Fatal("missing scroll data accepted")
	}
}
