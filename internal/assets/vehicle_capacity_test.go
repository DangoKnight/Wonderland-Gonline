package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestNativeVehicleCapacity(t *testing.T) {
	i := NativeItem{Definition: game.ItemDefinition{Type: game.VehicleType}}
	for _, tt := range []struct {
		rank byte
		want uint32
	}{{0, 0}, {1, 1800}, {5, 14400}, {10, 36000}} {
		i.Record[45] = tt.rank
		if got := i.VehicleCapacity(); got != tt.want {
			t.Fatal(tt.rank, got, tt.want)
		}
	}
}

func TestVehicleConditionSharedByInventoryAndHUD(t *testing.T) {
	i := NativeItem{Definition: game.ItemDefinition{Type: game.VehicleType}}
	i.Record[45] = 5
	item := game.Item{ID: 48010, Count: 1, Damage: 25}
	item.SetVehicleProgress(7200, 3600)
	if got := i.VehicleCondition(item); got != (VehicleCondition{Health: 7200, Maximum: 14400, Fuel: 3600}) {
		t.Fatal(got)
	}
	item.SetVehicleProgress(0, 0)
	if got := i.VehicleCondition(item); got.Health != 10800 {
		t.Fatal("damage percentage ignored", got)
	}
	item.ID = 48016
	if got := i.VehicleCondition(item); got.Health != got.Maximum {
		t.Fatal("Robinson raft lost health", got)
	}
}
