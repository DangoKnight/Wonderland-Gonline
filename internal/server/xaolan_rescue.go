package server

import (
	"context"
	"fmt"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// Compatibility content from IsXaolanRobberyReward in EveEventRuntime.
const (
	xaolanRobberyMap       = 12002
	xaolanRobberySource    = 1
	xaolanRobberyShale     = 43002
	xaolanRobberyStar      = 30025
	xaolanRobberyCompleted = 13032
	xaolanRobberyFollow    = 13033
	xaolanRobberyStarCount = 1
	questMarkAdvance       = 1
	questMarkComplete      = 2
	maxNativeQuestStep     = 255
	xaolanRobberyTailMarks = 2
)

func xaolanRobberyReward(mapID uint16, branch assets.Branch, op world.Op) bool {
	rule := world.DecodeCond(branch.Condition)
	if mapID != xaolanRobberyMap || rule.Kind != world.ConditionBattleResult || rule.W1 != xaolanRobberySource || op.Code != world.ActionPlayer || op.D1 != world.PlayerActionReward || op.D3 != xaolanRobberyShale || int32(op.Value()) <= 0 {
		return false
	}
	for _, raw := range branch.Operations {
		mark := world.DecodeOp(raw)
		if mark.Code == world.ActionQuestMark && mark.D1 == xaolanRobberyCompleted && mark.D2 == questMarkComplete {
			return true
		}
	}
	return false
}

// xaolanRobberyTail verifies the exact two authored marks after the reward.
// Both native rescue branches have this tail, with no intervening client step.
func xaolanRobberyTail(es *eventSession) bool {
	ops := es.ev.Branches[es.branch].Operations
	if es.index+xaolanRobberyTailMarks != len(ops) {
		return false
	}
	complete, follow := world.DecodeOp(ops[es.index]), world.DecodeOp(ops[es.index+1])
	return complete.Code == world.ActionQuestMark && complete.D1 == xaolanRobberyCompleted && complete.D2 == questMarkComplete &&
		follow.Code == world.ActionQuestMark && follow.D1 == xaolanRobberyFollow && follow.D2 == questMarkAdvance && follow.Value() == 1
}

// claimXaolanRobbery commits both items and the immediately following marks as
// one recovery checkpoint. A failed save or full bag cannot leave half a reward.
func (s *Server) claimXaolanRobbery(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	if !xaolanRobberyTail(es) {
		return false, c.send(headBanner("This rescue reward has an unsupported quest tail."))
	}
	if c.character.Quests[xaolanRobberyCompleted].State == game.Completed {
		es.index += xaolanRobberyTailMarks
		return true, nil
	}
	next := c.character.Clone()
	result, err := next.Bag.ApplyQuestItems([]game.ItemChange{{ID: op.D3, Count: int(int32(op.Value()))}, {ID: xaolanRobberyStar, Count: xaolanRobberyStarCount}}, s.stackLimit, s.Assets.Items)
	if err != nil {
		return false, c.send(headBanner("Cannot receive reward. Make room for Shale and Star."))
	}
	now := time.Now().UTC()
	complete := next.Quests[xaolanRobberyCompleted]
	if complete.State != game.InProgress {
		complete.Step = 0
	}
	complete.ID, complete.State, complete.Kills, complete.CompletedAt = xaolanRobberyCompleted, game.Completed, 0, &now
	if complete.StartedAt.IsZero() {
		complete.StartedAt = now
	}
	follow := next.Quests[xaolanRobberyFollow]
	if follow.State != game.InProgress {
		follow.Step = 0
	}
	if follow.Step >= maxNativeQuestStep {
		return false, c.send(headBanner("The rescue quest checkpoint cannot advance."))
	}
	follow.ID, follow.State, follow.Kills, follow.CompletedAt = xaolanRobberyFollow, game.InProgress, 0, nil
	follow.Step++
	if follow.StartedAt.IsZero() {
		follow.StartedAt = now
	}
	next.Quests[xaolanRobberyCompleted], next.Quests[xaolanRobberyFollow] = complete, follow
	if err := s.commit(ctx, c, next); err != nil {
		return false, err
	}
	es.index += xaolanRobberyTailMarks
	packets := [][]byte{next.Bag.AdditionPacket(result.Added), headBanner(fmt.Sprintf("Received Shale x%d and Star x1", op.Value())), {protocol.CommandEvent, protocol.EventStepComplete}}
	packets = append(packets, s.World.QuestUpdate(c.view, xaolanRobberyCompleted, complete)...)
	packets = append(packets, s.World.QuestUpdate(c.view, xaolanRobberyFollow, follow)...)
	return true, s.sendAll(c, packets)
}
