package server

import (
	"context"
	"fmt"
	"strings"

	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// gmClearSkills follows GmManager.ClearSkills and SkillManager initialization.
// Caller holds worldMu and has checked the actor's GM authorization.
func (s *Server) gmClearSkills(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /clearskills [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before resetting skills.")
	}
	next := target.character.Clone()
	next.Skills = game.StarterSkills(next.Body, next.Head, next.Element)
	next.UnlockQualifiedSkills(false, s.hasSkill)

	// Native AC5:3 overlays indexed skill records without clearing the table
	// (FUN_004381c4 at 0x438526..0x43858a). Include removed skills at grade/EXP
	// zero in the outgoing snapshot only; they must not survive in saved state.
	snapshotCharacter := next.Clone()
	retained := map[uint16]bool{}
	clientID := func(id uint16) uint16 {
		if id == game.StarterStunt(next.Body, next.Head) {
			return game.StarterStuntClientID
		}
		return id
	}
	for _, skill := range next.Skills {
		retained[clientID(skill.ID)] = true
	}
	for _, skill := range target.character.Skills {
		id := clientID(skill.ID)
		if !retained[id] {
			snapshotCharacter.Skills = append(snapshotCharacter.Skills, game.LearnedSkill{ID: skill.ID})
			retained[id] = true
		}
	}
	snapshot, err := snapshotCharacter.BaseStatsPacket(func(id uint16) (uint16, bool) {
		skill, ok := s.Assets.Skills[id]
		return skill.TableOrder, ok
	})
	if err != nil {
		return err
	}
	if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	if err = s.sendAll(target, [][]byte{snapshot, {protocol.CommandCharacterState, protocol.CharacterStateRefresh}}); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Skill reset saved; the target must reconnect to refresh skills.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Reset skills for %s to %d starter/qualified skills at grade 1 with zero proficiency.", next.Name, len(next.Skills)))
}
