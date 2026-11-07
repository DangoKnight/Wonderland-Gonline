package server

import (
	"context"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

type questConversationBranch struct {
	event  uint16
	branch byte
}

// Capture only dialogue branches that can unlock a same-NPC reward. This is
// transient interaction state, never an extra gameplay checkpoint or SQL write.
func questConversationSnapshot(c *game.Character, es *eventSession) *game.Character {
	speech, _, ok := world.QuestConversation(es.ev.Branches[es.branch], true)
	if !ok || !speech {
		return nil
	}
	hasMark := false
	for _, raw := range es.ev.Branches[es.branch].Operations {
		if world.DecodeOp(raw).Code == world.ActionQuestMark {
			hasMark = true
			break
		}
	}
	if !hasMark {
		return nil
	}
	before := c.Clone()
	return &before
}

// A choice callback is still part of the conversation. Retain its checkpoint
// provenance through conversation callbacks, stopping at intervening gameplay.
func prepareQuestConversation(c *game.Character, previous, next *eventSession) {
	key := questConversationBranch{next.ev.ClickID, next.ev.Branches[next.branch].Index}
	if previous != nil && previous.conversationBefore != nil && previous.mapID == next.mapID && previous.click == next.click {
		if _, _, pure := world.QuestConversation(next.ev.Branches[next.branch], true); pure {
			next.conversationBefore, next.conversationSource, next.conversationBranch = previous.conversationBefore, previous.conversationSource, previous.conversationBranch
			next.conversationVisited = previous.conversationVisited
			if next.conversationVisited == nil {
				next.conversationVisited = map[questConversationBranch]bool{}
			}
			next.conversationVisited[key] = true
			return
		}
	}
	next.conversationBefore = questConversationSnapshot(c, next)
	if next.conversationBefore != nil {
		next.conversationSource, next.conversationBranch = next.ev, next.branch
		next.conversationVisited = map[questConversationBranch]bool{key: true}
	}
}
func (s *Server) continueQuestConversation(ctx context.Context, c *Session, es *eventSession) (bool, error) {
	if es.conversationBefore == nil || c.event != es || c.character == nil {
		return false, nil
	}
	ev, next := s.World.ImmediateQuestReward(es.conversationBefore, c.character, c.view, es.mapID, es.click, es.conversationSource, es.conversationBranch)
	if ev == nil {
		return false, nil
	}
	// Never re-enter a delivered reward or another already visited branch in
	// this conversation, even if a malformed native receipt leaves it eligible.
	if es.conversationVisited[questConversationBranch{ev.ClickID, ev.Branches[next].Index}] {
		return false, nil
	}
	s.Log.Debug("immediate quest reward conversation", "map", es.mapID, "actor", es.click, "event", ev.ClickID, "branch", ev.Branches[next].Index)
	return true, s.startEvent(ctx, c, es.click, ev, next, false)
}
