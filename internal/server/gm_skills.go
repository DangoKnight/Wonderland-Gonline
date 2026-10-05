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

	snapshotCharacter := skillSnapshotWithRemovals(*target.character, next)
	snapshot, err := s.nativeStatsSnapshot(snapshotCharacter).BaseStatsPacket(func(id uint16) (uint16, bool) {
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
