package game

import "testing"

func TestPointForgeEligibilityAndMetadata(t *testing.T) {
	for _, test := range []struct {
		slot     uint16
		status   uint16
		value    int32
		eligible bool
	}{{1, 210, 100, true}, {5, 210, 110, true}, {6, 210, 110, false}, {0, 210, 110, false}, {3, 0, 110, false}, {3, 250, 110, false}, {3, 210, 99, false}} {
		d := ItemDefinition{EquipSlot: test.slot, Status: [2]uint16{0, test.status}, Values: [2]int32{0, test.value}}
		if d.CanPointForge() != test.eligible {
			t.Fatal(test)
		}
	}
	item := Item{ID: 1, Count: 1, Damage: 7, Metadata: [26]byte{9}}
	item.SetForge(3)
	if item.Forge() != 3 || item.Metadata[18] != 3 || item.Metadata[0] != 9 || item.Damage != 7 || item.ID != 1 {
		t.Fatal("forge metadata corrupted", item)
	}
}
