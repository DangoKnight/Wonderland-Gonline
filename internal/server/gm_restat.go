package server

import (
	"context"
	"fmt"
	"strings"
)

// gmRestat ports GmManager.RestatPlayer, saving before stat/snapshot replies.
// Caller holds worldMu and has checked the actor's GM privileges.
func (s *Server) gmRestat(ctx context.Context, actor *Session, arguments []string) error {
	arguments = strings.Fields(strings.Join(arguments, " "))
	if len(arguments) > 1 {
		return s.chatFeedback(actor, "Usage: /restat [character ID or name]")
	}
	target := actor
	if len(arguments) == 1 {
		target = s.findOnline(arguments[0])
		if target == nil {
			return s.chatFeedback(actor, "That character is offline or unavailable.")
		}
	}
	if !commandTravelAvailable(actor) || actor.trade != nil || !commandTravelAvailable(target) || target.trade != nil {
		return s.chatFeedback(actor, "Finish active interactions before resetting attributes.")
	}
	next := target.character.Clone()
	refund := next.ResetAttributes()
	next.Refill(s.Assets.Items)
	snapshot, err := next.BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if err != nil {
		return err
	}
	if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	packets := append(next.StatPackets(s.Assets.Items), snapshot)
	if err = s.sendAll(target, packets); err != nil {
		if target != actor {
			target.conn.Close()
			return s.chatFeedback(actor, "Attribute reset saved; the target must reconnect to refresh stats.")
		}
		return err
	}
	return s.chatFeedback(actor, fmt.Sprintf("Reset attributes for %s; refunded %d points (%d available).", next.Name, refund, next.StatPoints))
}
