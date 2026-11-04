package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/protocol"
)

func TestVehicleBoardingPreservesAndAllowsBattlePets(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	if err := s.petCommand(context.Background(), c, []byte{19, 1, 1}); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	vehicleDo(t, s, c, 7, 2, 48016)
	if c.character.ActivePet == 0 || c.character.BattlePet() == nil {
		t.Fatal("vehicle rested active pet")
	}
	owner, peer := wires[0].packets(t), wires[1].packets(t)
	if len(owner) != 1 || owner[0][1] != 10 {
		t.Fatal("boarding must not send a pet-rest reply", owner)
	}
	if len(peer) != 1 || peer[0][1] != 10 {
		t.Fatal("boarding must not send a peer pet-rest reply", peer)
	}
	for _, p := range [][]byte{{19, 1, 2}, {19, 4, 2}} {
		if err := s.worldCommand(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
	}
	if c.character.ActiveVehicle != 48016 || c.character.BattlePet() == nil || c.character.BattlePet().ID != 12032 || c.character.ActiveMount != 0 {
		t.Fatal("battle pet switch while riding failed")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].BattlePet() == nil || saved[0].ActivePet == 0 || saved[0].ActiveVehicle != 48016 {
		t.Fatal("battle selection and vehicle not saved together", err)
	}
	if err := s.petCommand(context.Background(), c, []byte{19, 2}); err != nil {
		t.Fatal(err)
	}
	if c.character.BattlePet() != nil || c.character.ActiveVehicle != 48016 {
		t.Fatal("rest while riding failed")
	}
	if err := s.petCommand(context.Background(), c, []byte{19, 1, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	wires[1].Reset()
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 1}.U32(14156)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 0 || c.character.ActiveVehicle != 48016 || c.character.BattlePet() == nil || wires[0].Len() != 0 {
		t.Fatal("pet riding must remain blocked")
	}
	vehicleDo(t, s, c, 10, 2, 48016)
	if c.character.BattlePet() == nil || c.character.ActivePet == 0 {
		t.Fatal("landing changed battle selection")
	}
}

func TestVehicleBoardingFailedSaveKeepsPetOut(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	if err := s.petCommand(context.Background(), c, []byte{19, 1, 1}); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	s.Store.Close()
	if err := s.vehicleCommand(context.Background(), c, []byte{15, 7, 2, 144, 187}); err == nil {
		t.Fatal("failed save accepted")
	}
	if c.character.BattlePet() == nil || c.character.ActiveVehicle != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed boarding rested pet or published success")
	}
}
