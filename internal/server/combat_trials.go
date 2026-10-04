package server

import (
	"context"
	"strconv"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

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
	accepted := trial != nil && gmIdle(c) && c.character.Map == trial.Map
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
		return c.send(headBanner("Trial unavailable. Check the stage, location, party availability and inventory space."))
	}
	npc := s.Assets.NPCs[trial.Guardian]
	return s.startBattle(c, &battleRun{trial: trial}, []battle.Enemy{{Template: uint32(npc.ID), Name: npc.Name, Level: int(npc.Level), HP: trial.HP, Attack: trial.Attack, Element: npc.Element}})
}

func (s *Server) trialReward(next *game.Character, trial *assets.CombatTrial) [][]byte {
	limit, _ := s.stackLimit(trial.Reward)
	adds, err := next.Bag.Grant(game.Item{ID: trial.Reward}, int(trial.Count), limit, s.Assets.Items)
	if err != nil {
		return [][]byte{headBanner("Trial reward could not fit in your inventory.")}
	}
	return [][]byte{next.Bag.AdditionPacket(adds), headBanner("Cleared " + trial.Name + ".")}
}
