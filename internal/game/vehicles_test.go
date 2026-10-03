package game

import "testing"

func TestVehicleFamilies(t *testing.T) {
	for _, pair := range [][2]uint16{{48019, 48001}, {48028, 48010}, {48032, 48014}, {48033, 48017}, {48034, 48018}, {48036, 48035}, {48038, 48037}, {48040, 48039}, {48042, 48041}, {48016, 48016}} {
		if BaseVehicleID(pair[0]) != pair[1] || !SameVehicle(uint32(pair[0]), pair[1]) || !SameVehicle(uint32(pair[1]), pair[0]) {
			t.Fatal(pair)
		}
	}
	if SameVehicle(0, 0) || SameVehicle(0x1bb90, 48016) || SameVehicle(48016, 48010) {
		t.Fatal("invalid vehicle alias")
	}
	if !Raft(48010) || !Raft(48028) || !Raft(48016) || Raft(48001) {
		t.Fatal("raft family")
	}
}

func TestVehicleOwnershipAndNormalization(t *testing.T) {
	items := map[uint16]ItemDefinition{48016: {ID: 48016, Type: VehicleType}, 48001: {ID: 48001, Type: VehicleType}, 30001: {ID: 30001, Type: 1}}
	c := Character{ActiveVehicle: 48016, VehicleSlot: 2}
	c.Bag[1] = Item{ID: 48016, Count: 1, Damage: 99}
	if _, ok := c.Vehicle(2, 48016, items); !ok || c.NormalizeVehicle(items) {
		t.Fatal("valid vehicle changed")
	}
	for _, slot := range []byte{0, 1, 51, 255} {
		if _, ok := c.Vehicle(slot, 48016, items); ok {
			t.Fatal("invalid slot", slot)
		}
	}
	for _, item := range []Item{{ID: 48016, Count: 0}, {ID: 48016, Count: 1, Damage: 100}, {ID: 48001, Count: 1}, {ID: 30001, Count: 1}} {
		next := c.Clone()
		next.Bag[1] = item
		if !next.NormalizeVehicle(items) || next.ActiveVehicle != 0 || next.VehicleSlot != 0 {
			t.Fatal("stale mount retained", item)
		}
	}
	c.ActiveMount = 14156
	if !c.NormalizeVehicle(items) || c.ActiveMount != 0 || c.ActiveVehicle != 48016 {
		t.Fatal("mutually exclusive mounts")
	}
}
