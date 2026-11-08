package server

import (
	"context"
	"fmt"
	"wonderland-gonline/internal/game"
)

// Called under worldMu after full-round calculation. Each owner's player and
// pet gear are one SQL mutation; no wear receipt is published before commit.
func (s *Server) commitCombatGearWear(run *battleRun) error {
	return s.commitCombatGearWearWithRoll(run, s.rules().Next)
}

func (s *Server) commitCombatGearWearWithRoll(run *battleRun, roll func(int, int) int) error {
	hits := run.b.GearHits
	run.b.GearHits = nil // Consumed once regardless of transport or later settlement.
	if run.b.PvP || len(hits) == 0 {
		return nil
	}
	for _, m := range run.active() {
		c := m.c
		before := c.character.Clone()
		next := before.Clone()
		changed := false
		for _, hit := range hits[m.self] {
			changed = next.Equipment.ApplyCombatWear(s.Assets.Items, game.CombatWearChance(m.self.Level, hit.EnemyLevel), hit.Contacts, roll) || changed
		}
		petIndex := battlePetIndex(next, m)
		if petIndex >= 0 {
			for _, hit := range hits[m.pet] {
				changed = next.Pets[petIndex].Equipment.ApplyCombatWear(s.Assets.Items, game.CombatWearChance(m.pet.Level, hit.EnemyLevel), hit.Contacts, roll) || changed
			}
		}
		if !changed {
			continue
		}
		var saved game.Character
		err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error {
			if stored.Equipment != before.Equipment {
				return fmt.Errorf("combat equipment changed")
			}
			if petIndex >= 0 {
				i, ok := stored.Pet(before.Pets[petIndex].ID)
				if !ok || stored.Pets[i].Equipment != before.Pets[petIndex].Equipment {
					return fmt.Errorf("combat pet equipment changed")
				}
				stored.Pets[i].Equipment = next.Pets[petIndex].Equipment
			}
			stored.Equipment = next.Equipment
			saved = stored.Clone()
			return nil
		})
		if err != nil {
			return err
		}
		s.adoptSavedCharacter(c, saved)
		m.self.Char = c.character
		// These are private roster receipts, not public spectator playback.
		s.sendOrClose(c, c.character.EquipmentPacket())
		if petIndex >= 0 {
			s.sendOrClose(c, s.petListPacket(c.character, c.pets))
		}
	}
	return nil
}
