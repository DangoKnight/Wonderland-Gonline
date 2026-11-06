package game

import (
	"bytes"
	"testing"
)

func TestVehicleProgressNativeMetadataOffsets(t *testing.T) {
	i := Item{ID: 48010, Count: 1}
	for n := range i.Metadata {
		i.Metadata[n] = 99
	}
	i.SetVehicleProgress(0x04030201, 0x0605)
	if !bytes.Equal(i.Metadata[12:18], []byte{1, 2, 3, 4, 5, 6}) || i.Metadata[11] != 99 || i.Metadata[18] != 99 || i.VehicleWear() != 0x04030201 || i.VehicleFuel() != 0x0605 {
		t.Fatal("vehicle metadata mapping", i.Metadata)
	}
}

func TestVehicleNormalizationPreservesBattlePet(t *testing.T) {
	c := Character{ActiveVehicle: 48016, VehicleSlot: 1, ActiveMount: 12032, ActivePet: 12032, Pets: []Pet{{ID: 12032, Battle: true, HP: 43, SP: 7}}}
	c.Bag[0] = Item{ID: 48016, Count: 1}
	if !c.NormalizeVehicle(map[uint16]ItemDefinition{48016: {Type: VehicleType}}) || c.ActiveVehicle != 48016 || c.ActiveMount != 0 || c.ActivePet != 12032 || c.BattlePet() == nil || c.Pets[0].HP != 43 || c.Pets[0].SP != 7 {
		t.Fatal("old vehicle/pet state not normalized safely")
	}
}
