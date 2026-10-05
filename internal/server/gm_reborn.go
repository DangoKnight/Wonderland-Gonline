package server

import (
	"context"
	"fmt"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

const rebornAuraEffect = 60050

func (s *Server) gmReborn(ctx context.Context, c *Session, command string, args []string) error {
	if len(args) != 1 {
		return s.chatFeedback(c, "Usage: /reborn <Killer|Warrior|Knight|Wit|Priest|Seer>")
	}
	var job byte
	var cape uint16
	var name string
	for _, class := range s.Assets.RebornClasses {
		if class.Enabled && strings.EqualFold(class.Name, args[0]) {
			job, cape, name = class.Job, class.CapeID, class.Name
			break
		}
	}
	if job == game.JobNone {
		return s.chatFeedback(c, "That reborn class/cape is unavailable.")
	}
	definition, known := s.Assets.Items[cape]
	if !known {
		return s.chatFeedback(c, "The reborn cape definition is unavailable.")
	}
	var adds []game.Addition
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if next.Reborn || next.Job != game.JobNone || next.Level < game.RebornMinimumLevel {
			return fmt.Errorf("rebirth requires level %d and a character that has not reborn", game.RebornMinimumLevel)
		}
		var err error
		adds, err = next.Bag.Grant(game.Item{ID: cape}, 1, definition.StackLimit(), s.Assets.Items)
		if err != nil {
			return err
		}
		next.Job = job
		next.Reborn = true
		next.EXP = 0
		next.Level = 1
		next.Refill(s.Assets.Items)
		return nil
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Rebirth failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	packets := append([][]byte{next.Bag.AdditionPacket(adds), next.ExpPacket()}, next.StatPackets(s.Assets.Items)...)
	base, err := s.nativeStatsSnapshot(next).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if err != nil {
		return err
	}
	packets = append(packets, base)
	appearance, err := next.AppearancePacket(true)
	if err != nil {
		return err
	}
	s.broadcastWorld(c, appearance)
	self, err := next.AppearancePacket(false)
	if err != nil {
		return err
	}
	packets = append(packets, self)
	effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(next.ID).U16(rebornAuraEffect)
	s.broadcastWorld(c, effect)
	packets = append(packets, effect)
	if err = s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, "Rebirth completed: "+name+", level 1.")
}
