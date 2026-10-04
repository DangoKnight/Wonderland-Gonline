package server

import (
	"slices"
	"strings"
	"time"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// Compatibility content references from CheckQuestBattleCompletion. Niss's
// companion ID is corrected against WLRI Npc.dat: 11066 is not her template.
const (
	legacySaveNissQuest      uint32 = 1005
	legacyRescueXaolanQuest  uint32 = 1010
	legacyRedRidingHoodQuest uint32 = 1012
	legacyWolfGuard          uint32 = 11066
	legacyPirateLea          uint32 = 14155
	legacyHijackerFirst      uint32 = 12049
	legacyHijackerSecond     uint32 = 12050
	legacyWildWolf           uint32 = 17437
	legacyNissCompanion      uint32 = 14081
	legacyXaolanCompanion    uint32 = 14156
	legacyWolfQuestGold      uint32 = 400
	legacyXaolanStarterJade  uint16 = 25028
)

// legacyBattleRewards applies the three monster-ID fallbacks only when no native
// EVE callback owns the victory. Completion and recruitment are staged in the
// same character clone as combat rewards and become visible after one save.
func (s *Server) legacyBattleRewards(next *game.Character, run *battleRun, roster *petRoster) [][]byte {
	var packets [][]byte
	for _, definition := range []struct {
		quest    uint32
		monsters []uint32
		pet      uint32
		gold     uint32
		start    bool
		aliases  []string
	}{
		{legacySaveNissQuest, []uint32{legacyWolfGuard}, legacyNissCompanion, 0, false, []string{"wolf guard"}},
		{legacyRescueXaolanQuest, []uint32{legacyPirateLea, legacyHijackerFirst, legacyHijackerSecond}, legacyXaolanCompanion, 0, true, []string{"pirate lea", "hijacker"}},
		{legacyRedRidingHoodQuest, []uint32{legacyWildWolf}, 0, legacyWolfQuestGold, false, []string{"wild wolf"}},
	} {
		matched := false
		for _, m := range run.b.Defenders {
			if m.Kind != battle.Monster || !m.Dead() || m.Captured {
				continue
			}
			matched = matched || slices.Contains(definition.monsters, m.Template)
			for _, alias := range definition.aliases {
				matched = matched || strings.Contains(strings.ToLower(m.Name), alias)
			}
		}
		if !matched {
			continue
		}
		q, exists := next.Quests[definition.quest]
		if !exists && definition.start {
			q = game.Quest{ID: definition.quest, State: game.InProgress, Step: 1, StartedAt: time.Now().UTC()}
		}
		if q.State != game.InProgress {
			continue
		}
		if definition.pet != 0 {
			if !s.canRecruit(next, definition.pet) {
				packets = append(packets, headBanner("Make a companion slot available, then win this quest battle again."))
				continue
			}
			if _, owned := next.Pet(definition.pet); !owned {
				slot := next.FreePetSlot()
				var pet game.Pet
				if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, definition.pet) }); i >= 0 {
					pet = next.ReservePets[i]
					next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
					pet.Slot = slot
				} else {
					t := s.petTemplate(definition.pet)
					pet = game.NewPet(definition.pet, s.companionName(definition.pet, ""), slot, t, s.Assets.Items)
					if definition.pet == legacyXaolanCompanion && s.hasItem(legacyXaolanStarterJade) {
						pet.Equipment[5] = game.Item{ID: legacyXaolanStarterJade, Count: 1}
					}
					pet.EnsureSkills(t, s.hasSkill)
					pet.Normalize(s.Assets.Items, true)
				}
				next.Pets = append(next.Pets, pet)
				if roster.register(pet.ID) {
					packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
				}
				packets = append(packets, pet.ProgressionPackets(roster.slot(pet.ID), s.Assets.Items)...)
			}
		}
		q.ID = definition.quest
		q.Complete(time.Now())
		if next.Quests == nil {
			next.Quests = map[uint32]game.Quest{}
		}
		next.Quests[q.ID] = q
		next.Gold = uint32(min(uint64(game.MaxGold), uint64(next.Gold)+uint64(definition.gold)))
		// Settlement prepares native journal updates only after the durable save.
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold), headBanner("Quest battle completed."))
	}
	return packets
}
