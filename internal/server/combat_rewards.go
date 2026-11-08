package server

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
)

// legacyBattleRewards applies the three monster-ID fallbacks only when no native
// EVE callback owns the victory. Completion and recruitment are staged in the
// same character clone as combat rewards and become visible after one save.
func (s *Server) legacyBattleRewards(next *game.Character, run *battleRun, roster *petRoster) [][]byte {
	var packets [][]byte
	for _, definition := range s.Assets.Story.BattleRewards {
		matched := false
		for _, m := range run.b.Defenders {
			if m.Kind != battle.Monster || !m.Dead() || m.Captured {
				continue
			}
			matched = matched || slices.Contains(definition.Monsters, m.Template)
			for _, alias := range definition.Aliases {
				matched = matched || strings.Contains(strings.ToLower(m.Name), alias)
			}
		}
		if !matched {
			continue
		}
		q, exists := next.Quests[definition.Quest]
		if !exists && definition.Start {
			q = game.Quest{ID: definition.Quest, State: game.InProgress, Step: 1, StartedAt: time.Now().UTC()}
		}
		if q.State != game.InProgress {
			continue
		}
		if definition.Companion != 0 {
			if !s.canRecruit(next, definition.Companion) {
				packets = append(packets, serverwire.HeadBanner("Make a companion slot available, then win this quest battle again."))
				continue
			}
			if _, owned := next.Pet(definition.Companion); !owned {
				slot := next.FreePetSlot()
				var pet game.Pet
				if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, definition.Companion) }); i >= 0 {
					pet = next.ReservePets[i]
					next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
					pet.Slot = slot
				} else {
					t := s.petTemplate(definition.Companion)
					pet = game.NewPet(definition.Companion, s.companionName(definition.Companion, ""), slot, t, s.Assets.Items)
					for _, equipment := range definition.Equipment {
						if int(equipment.Slot) < len(pet.Equipment) && pet.Equipment[equipment.Slot].Empty() && s.hasItem(equipment.ItemID) {
							pet.Equipment[equipment.Slot] = game.Item{ID: equipment.ItemID, Count: 1}
						}
					}
					pet.EnsureSkills(t, s.hasSkill)
					pet.Normalize(s.Assets.Items, true, s.petTemplate(pet.ID))
				}
				next.Pets = append(next.Pets, pet)
				if roster.register(pet.ID) {
					packets = append(packets, pet.RecruitPacket(next.ID, s.petTemplate(pet.ID)))
				}
				packets = append(packets, pet.ProgressionPackets(roster.slot(pet.ID), s.Assets.Items, s.petTemplate(pet.ID))...)
				if pet.Equipment != (game.Equipment{}) {
					packets = append(packets, s.petListPacket(next, roster))
				}
			}
		}
		q.ID = definition.Quest
		q.Complete(time.Now())
		if next.Quests == nil {
			next.Quests = map[uint32]game.Quest{}
		}
		next.Quests[q.ID] = q
		next.Gold = uint32(min(uint64(game.MaxGold), uint64(next.Gold)+uint64(definition.Gold)))
		// Settlement prepares native journal updates only after the durable save.
		packets = append(packets, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold), serverwire.HeadBanner("Quest battle completed."))
	}
	return packets
}

func (s *Server) palaceTrialCommand(c *Session, p []byte) error {
	if len(p) != 3 || p[1] != protocol.PalaceTrialChallenge {
		return protocol.ErrMalformed
	}
	return s.challengeTrial(c, p[2], true)
}

func (s *Server) gmPalace(_ context.Context, c *Session, _ string, args []string) error {
	if len(args) != 1 {
		return s.chatFeedback(c, "Usage: /palace <stage>")
	}
	stage, err := strconv.ParseUint(args[0], 10, 8)
	if err != nil {
		return s.chatFeedback(c, "Invalid Palace stage.")
	}
	return s.challengeTrial(c, byte(stage), false)
}

func (s *Server) challengeTrial(c *Session, stage byte, native bool) error {
	var trial *assets.CombatTrial
	for _, definition := range s.Assets.CombatTrials {
		if definition.Stage == stage {
			copy := definition
			trial = &copy
			break
		}
	}
	accepted := trial != nil && battleInteractionAvailable(c, nil) && gmIdle(c) && c.character.Map == trial.Map
	if accepted {
		// Reserve enough space before admission for every participant. Reward is
		// still checked again at settlement before the durable character save.
		for _, m := range s.battleTeam(c) {
			if !gmIdle(m) {
				accepted = false
				break
			}
			copy := m.character.Clone()
			limit, _ := s.stackLimit(trial.Reward)
			if _, err := copy.Bag.Grant(game.Item{ID: trial.Reward}, int(trial.Count), limit, s.Assets.Items); err != nil {
				accepted = false
				break
			}
		}
	}
	if native {
		status := protocol.PalaceTrialRejected
		if accepted {
			status = protocol.PalaceTrialAccepted
		}
		if err := c.send([]byte{protocol.CommandPalaceTrial, protocol.PalaceTrialChallenge, stage, status}); err != nil {
			return err
		}
	}
	if !accepted {
		return c.send(serverwire.HeadBanner("Trial unavailable. Check the stage, location, party availability and inventory space."))
	}
	npc := s.Assets.NPCs[trial.Guardian]
	return s.startBattle(c, &battleRun{trial: trial}, []battle.Enemy{{Template: uint32(npc.ID), Name: npc.Name, Level: int(npc.Level), HP: trial.HP, Attack: trial.Attack, Element: npc.Element}})
}

func (s *Server) trialReward(next *game.Character, trial *assets.CombatTrial) [][]byte {
	limit, _ := s.stackLimit(trial.Reward)
	adds, err := next.Bag.Grant(game.Item{ID: trial.Reward}, int(trial.Count), limit, s.Assets.Items)
	if err != nil {
		return [][]byte{serverwire.HeadBanner("Trial reward could not fit in your inventory.")}
	}
	return [][]byte{next.Bag.AdditionPacket(adds), serverwire.HeadBanner("Cleared " + trial.Name + ".")}
}

// customQuestKills runs on the settlement clone. Persistence and all receipts
// share the normal victory transaction; captures and PvP never count.
func (s *Server) customQuestKills(next *game.Character, run *battleRun) [][]byte {
	if run.b.PvP {
		return nil
	}
	ids := make([]uint32, 0, len(next.Quests))
	for id := range next.Quests {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var packets [][]byte
	for _, monster := range run.b.Defenders {
		if monster.Kind != battle.Monster || monster.Captured || monster.HP > 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(monster.Name))
		for _, id := range ids {
			q := next.Quests[id]
			d, ok := s.Assets.QuestDefinitions[id]
			if !ok || q.State != game.InProgress {
				continue
			}
			kind, tid, pattern, required := d.Type, d.BattleMonsterID, d.BattleMonsterName, d.RequiredKillCount
			if q.Step > 0 && q.Step <= len(d.Steps) {
				step := d.Steps[q.Step-1]
				kind, tid, pattern, required = step.Type, step.BattleMonsterID, step.BattleMonsterName, step.RequiredKillCount
			}
			if kind != assets.QuestMonsterBattle {
				continue
			}
			pattern = strings.ToLower(strings.TrimSpace(pattern))
			matched := tid > 0 && tid == monster.Template || (name != "" && pattern != "" && (strings.Contains(name, pattern) || strings.Contains(pattern, name))) || (tid == 0 && pattern == "")
			if !matched || q.Kills >= math.MaxInt32 {
				continue
			}
			q.Kills++
			next.Quests[id] = q
			required = max(1, required)
			text := fmt.Sprintf("[Quest] %s: %d/%d defeated.", d.Title, q.Kills, required)
			packets = append(packets, serverwire.QuestProgressMessage(text))
			if q.Kills >= required {
				packets = append(packets, serverwire.QuestProgressMessage("[Quest Objective Completed] "+d.Title))
			}
		}
	}
	return packets
}
