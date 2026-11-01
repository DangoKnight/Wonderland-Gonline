package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func (s *Server) gmReborn(ctx context.Context, c *Session, command string, args []string) error {
	if len(args) < 1 || len(args) > 2 || len(args) == 2 && !strings.EqualFold(args[1], "force") {
		return s.chatFeedback(c, "Usage: /reborn <Killer|Warrior|Knight|Wit|Priest|Seer|1-6> [force]")
	}
	query := args[0]
	if strings.EqualFold(query, "sage") || strings.EqualFold(query, "mage") {
		query = "Wit"
	}
	var job byte
	for _, class := range s.Assets.RebornClasses {
		if class.Enabled && (strings.EqualFold(class.Name, query) || strconv.Itoa(int(class.Job)) == query) {
			job = class.Job
			break
		}
	}
	if job == game.JobNone {
		return s.chatFeedback(c, "That reborn class/cape is unavailable.")
	}
	return s.performReborn(ctx, c, job, len(args) == 2)
}

func (s *Server) performReborn(ctx context.Context, c *Session, job byte, force bool) error {
	var skillPackets [][]byte
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(next *game.Character) error {
		if !next.RebornAs(job, force) {
			return fmt.Errorf("rebirth requires level %d and a character that has not reborn", game.RebornMinimumLevel)
		}
		skillPackets = s.unlockQualifiedSkills(next, true)
		next.Refill(s.Assets.Items)
		return nil
	}, *c.character)
	if err != nil {
		return s.chatFeedback(c, "Rebirth failed: "+err.Error())
	}
	s.adoptSavedCharacter(c, next)
	c.view.RebornFormOpen = false
	packets := append([][]byte{next.ExpPacket()}, next.StatPackets(s.Assets.Items)...)
	packets = append(packets, skillPackets...)
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
	effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateShapeshift}.U32(next.ID).U16(0)
	s.broadcastWorld(c, effect)
	packets = append(packets, effect)
	if err = s.sendAll(c, packets); err != nil {
		return err
	}
	return s.chatFeedback(c, fmt.Sprintf("Rebirth completed: level %d.", next.Level))
}
