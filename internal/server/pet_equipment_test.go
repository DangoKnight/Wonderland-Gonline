package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestPetSpecialEquipmentRecruitVoucherReserveAndRollback(t *testing.T) {
	for _, voucher := range []bool{false, true} {
		t.Run(map[bool]string{false: "recruit", true: "voucher"}[voucher], func(t *testing.T) {
			s, players, wires := petFixture(t)
			c := players[0]
			s.Assets.NPCs[14081] = assets.NPC{ID: 14081, Type: 4, Name: "Niss", Stats: [5]uint16{4, 4, 8, 8, 4}}
			s.Assets.Items[14073] = game.ItemDefinition{ID: 14073, EquipSlot: 3}
			s.Assets.Story.CompanionEquipment = []assets.StoryCompanionEquipment{{Template: 14081, Equipment: []assets.StoryEquipment{{Slot: 2, ItemID: 14073}}}}
			s.Assets.PetVouchers = map[uint16]uint16{30068: 14081}
			s.Assets.Items[30068] = game.ItemDefinition{ID: 30068}
			c.character.Bag[8] = game.Item{ID: 30068, Count: 1}
			s.World = world.New(s.Assets)
			if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
				t.Fatal(err)
			}
			acquire := func(ctx context.Context) error {
				if voucher {
					return s.dispatch(ctx, c, []byte{23, 96, 9})
				}
				_, err := s.recruit(ctx, c, 14081)
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := acquire(ctx); err == nil || len(c.character.Pets) != 0 || wires[0].Len() != 0 {
				t.Fatal("failed grant leaked pet or packets", err)
			}
			if err := acquire(context.Background()); err != nil {
				t.Fatal(err)
			}
			pet := c.character.Pets[0]
			if pet.Equipment != (game.Equipment{2: {ID: 14073, Count: 1}}) {
				t.Fatal("starter not equipped", pet)
			}
			foundRoster := false
			for _, p := range wires[0].packets(t) {
				if len(p) >= 2 && p[0] == 15 && p[1] == 8 {
					foundRoster = true
				}
			}
			if !foundRoster {
				t.Fatal("equipped roster not published")
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Pets[0].Equipment != pet.Equipment {
				t.Fatal("gear not durable", err)
			}
			// A returning pet whose original weapon was removed must not get another.
			pet.Equipment = game.Equipment{}
			next := c.character.Clone()
			next.Pets = nil
			next.ReservePets = []game.Pet{pet}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			if ok, err := s.recruit(context.Background(), c, 14081); err != nil || !ok || c.character.Pets[0].Equipment != (game.Equipment{}) {
				t.Fatal("returning pet gear regenerated", ok, err)
			}
		})
	}
}
