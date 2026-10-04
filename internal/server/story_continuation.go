package server

import (
	"context"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

const (
	robinsonRecruitmentEvent     = 19
	robinsonRaftBranch           = 1
	robinsonDialogueBranch       = 3
	robinsonRecruitmentMark      = 12047
	robinsonRecruitmentCompanion = 12178
)

const (
	cliveOfferMap       = 11040
	cliveOfferActor     = 4
	cliveOfferEvent     = 2
	cliveOfferMark      = 13102
	cliveJoinedMark     = 13103
	cliveOfferStep      = 4
	springReturnMap     = 60002
	springRocaActor     = 43
	springXaolanActor   = 44
	springProgressMark  = 50042
	springMovieMark     = 50047
	springReturnEvent   = 51
	dinnerMap           = 12000
	dinnerTemplate      = 14140
	dinnerProgressMark  = 12018
	dinnerAgeQuizMark   = 12043
	dinnerCompletedMark = 12019
)

func storyMark(c *game.Character, id uint32) int {
	q, ok := c.Quests[id]
	if ok && q.State == game.InProgress {
		return q.Step
	}
	return 0
}

// storyClick resumes reference checkpoints through their authored event. The
// normal visibility/range checks still occur before this controller is reached.
func (s *Server) storyClick(ctx context.Context, c *Session, click uint16) (bool, error) {
	mapID := c.character.Map
	event := uint16(0)
	if mapID == cliveOfferMap && click == cliveOfferActor && storyMark(c.character, cliveOfferMark) == cliveOfferStep && storyMark(c.character, cliveJoinedMark) == 0 {
		event = cliveOfferEvent
	}
	spring := mapID == springReturnMap && (click == springRocaActor || click == springXaolanActor) && storyMark(c.character, springProgressMark) > 1 && storyMark(c.character, springMovieMark) > 0
	if spring {
		event = springReturnEvent
	}
	if event != 0 {
		if ev, ok := s.World.Event(mapID, event); ok {
			if branch := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); branch >= 0 {
				return true, s.startEvent(ctx, c, click, ev, branch, true)
			}
		}
		if spring {
			return true, s.sendAll(c, [][]byte{headBanner("Please leave two free companion slots for Roca and Xaolan."), {protocol.CommandEvent, protocol.EventResume}})
		}
	}
	if mapID == dinnerMap && storyMark(c.character, dinnerProgressMark) == 1 && storyMark(c.character, dinnerAgeQuizMark) == 1 && storyMark(c.character, dinnerCompletedMark) == 0 {
		if npc, ok := s.World.NPC(mapID, click); ok && npc.Template == dinnerTemplate {
			next := c.character.Clone()
			q := next.Quests[dinnerProgressMark]
			q.Step = 2
			next.Quests[dinnerProgressMark] = q
			if err := s.commit(ctx, c, next); err != nil {
				return true, err
			}
			if err := s.sendAll(c, s.World.QuestUpdate(c.view, dinnerProgressMark, q)); err != nil {
				return true, err
			}
		}
	}
	return false, nil
}

// storyContinuation handles only the reference's explicit sequence transitions.
// Forward-only branch selection prevents replaying rewards and earlier choices.
func (s *Server) storyContinuation(ctx context.Context, c *Session, es *eventSession) (bool, error) {
	mapID, event, branch := es.mapID, es.ev.ClickID, es.ev.Branches[es.branch].Index
	nextEvent, click := event, es.click
	follow := false
	switch {
	case mapID == game.MapID11077 && event == 7 && branch == 5:
		nextEvent, click, follow = 12, 13, true
	case mapID == game.MapID12052 && event == 6 && (branch == 1 || branch == 6 || branch == 14):
		follow = true
		if branch == 14 {
			nextEvent = 9
		}
	case mapID == game.MapID11013 && event == 11, mapID == game.MapID12211 && event == 1,
		mapID == game.MapID11149 && event == 2 && branch == 10,
		mapID == game.MapID11159 && event == 1 && branch == 1,
		mapID == game.MapID11157 && (event == 1 || event == 2 || event == 5 || event == 6),
		(mapID == beachMap || mapID == beachAltMap) && event == robinsonRecruitmentEvent && (branch == robinsonRaftBranch || branch == robinsonDialogueBranch):
		follow = true
	}
	if !follow {
		return false, nil
	}
	ev := es.ev
	exclude := es.branch
	if nextEvent != event {
		var ok bool
		ev, ok = s.World.Event(mapID, nextEvent)
		if !ok {
			return false, nil
		}
		exclude = -1
	}
	next := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, exclude)
	if next < 0 {
		return false, nil
	}
	return true, s.startEvent(ctx, c, click, ev, next, false)
}

// robinsonRecoveryEvents ports e63e9fe's candidate rescue. Keep existing linked
// ordering when Event 19 is already present; otherwise try it ahead of greetings.
// The ordinary branch, disabled-event, reward and capacity checks still apply.
func (s *Server) robinsonRecoveryEvents(c *game.Character, click uint16, events []*assets.Event) []*assets.Event {
	if (c.Map != beachMap && c.Map != beachAltMap) || click != robinsonClick || c.Quests[robinsonRecruitmentMark].State != game.InProgress {
		return events
	}
	if _, recruited := c.Pet(robinsonRecruitmentCompanion); recruited {
		return events
	}
	ev, ok := s.World.Event(c.Map, robinsonRecruitmentEvent)
	if !ok {
		return events
	}
	for _, candidate := range events {
		if candidate == ev {
			return events
		}
	}
	return append([]*assets.Event{ev}, events...)
}
