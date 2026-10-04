package server

import (
	"context"
	"errors"
	"sort"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

type AdminOpcode struct {
	Operation world.Op `json:"operation"`
	Meaning   string   `json:"meaning"`
	Talk      string   `json:"talk,omitempty"`
}
type AdminEventBranch struct {
	Index      byte          `json:"index"`
	Condition  world.Cond    `json:"condition"`
	Operations []AdminOpcode `json:"operations"`
}
type AdminActorEvent struct {
	ID       uint16             `json:"id"`
	Name     string             `json:"name"`
	Branches []AdminEventBranch `json:"branches"`
}
type AdminActorReport struct {
	MapID          uint16            `json:"map_id"`
	Actor          assets.MapNPC     `json:"actor"`
	LinkedQuestIDs []uint32          `json:"linked_quest_ids"`
	Events         []AdminActorEvent `json:"events"`
}

func actorReport(catalog *assets.Catalog, mapID, click uint16) (AdminActorReport, error) {
	m, ok := catalog.Maps[mapID]
	if !ok {
		return AdminActorReport{}, errors.New("unknown map")
	}
	var actor assets.MapNPC
	found := false
	for _, n := range m.NPCs {
		if n.ClickID == click {
			actor = n
			found = true
			break
		}
	}
	if !found {
		return AdminActorReport{}, errors.New("unknown actor")
	}
	report := AdminActorReport{MapID: mapID, Actor: actor, LinkedQuestIDs: []uint32{}, Events: []AdminActorEvent{}}
	ids := map[uint16]bool{}
	for _, id := range actor.Events {
		ids[uint16(id)] = true
	}
	if len(ids) == 0 {
		ids[click] = true
	}
	linked := map[uint32]bool{}
	for _, ev := range m.Events {
		if !ids[ev.ClickID] {
			continue
		}
		entry := AdminActorEvent{ID: ev.ClickID, Name: ev.Name, Branches: []AdminEventBranch{}}
		for _, branch := range ev.Branches {
			cond := world.DecodeCond(branch.Condition)
			row := AdminEventBranch{Index: branch.Index, Condition: cond, Operations: []AdminOpcode{}}
			for _, raw := range branch.Operations {
				op := world.DecodeOp(raw)
				meaning := "unresolved"
				talk := ""
				switch op.Code {
				case world.ActionActor:
					meaning = "actor action"
					if op.D2 == world.ActorActionPropState {
						meaning = "prop frame / break animation"
						if cond.Kind == world.ConditionQuest && cond.W1 != 0 {
							linked[uint32(cond.W1)] = true
						}
					}
				case world.ActionPlayer:
					meaning = "player action"
				case world.ActionQuestMark:
					meaning = "quest mark action"
				case world.ActionCompanion:
					meaning = "companion action"
				case world.ActionBattle, world.ActionBattleAlternate:
					meaning = "battle"
				case world.ActionServiceOrTeleport:
					meaning = "NPC service or map transition"
				case world.ActionMovie:
					meaning = "movie"
				case world.ActionMinigame:
					meaning = "minigame"
				case world.ActionLearnSkill:
					meaning = "learn skill"
				case world.ActionTransform:
					meaning = "character transformation"
				case world.ActionMinimapMarker:
					meaning = "minimap marker"
				case world.ActionGather:
					meaning = "gathering"
				case world.ActionEffect:
					meaning = "effect"
				}
				if op.Code == world.ActionActor && op.D2 != world.ActorActionPropState && op.D2 != world.ActorActionAnimation && op.D2 != world.ActorActionAnimationAlternate && op.D2 != world.ActorActionPath && op.D2 != world.ActorActionPose {
					var talkID uint32
					if op.D3 >= world.MinDialogueID && op.D3 <= world.MaxDialogueWordID {
						talkID = uint32(op.D3) | uint32(op.D2)<<16
					} else if op.D2 >= world.MinDialogueID && op.D2 <= world.MaxDialogueWordID {
						talkID = uint32(op.D2)
					}
					if talkID >= world.MinDialogueID {
						meaning = "dialogue"
						talk, _ = catalog.Talks.Resolve(talkID)
					}
				}
				row.Operations = append(row.Operations, AdminOpcode{Operation: op, Meaning: meaning, Talk: talk})
			}
			entry.Branches = append(entry.Branches, row)
		}
		report.Events = append(report.Events, entry)
	}
	for id := range linked {
		report.LinkedQuestIDs = append(report.LinkedQuestIDs, id)
	}
	sort.Slice(report.LinkedQuestIDs, func(i, j int) bool { return report.LinkedQuestIDs[i] < report.LinkedQuestIDs[j] })
	return report, nil
}
func (s *Server) AdminNPCReport(template uint32) []AdminActorReport {
	catalog := s.AssetSnapshot()
	out := []AdminActorReport{}
	maps := make([]int, 0, len(catalog.Maps))
	for id := range catalog.Maps {
		maps = append(maps, int(id))
	}
	sort.Ints(maps)
	for _, id := range maps {
		for _, n := range catalog.Maps[uint16(id)].NPCs {
			if n.Template == template {
				report, err := actorReport(catalog, uint16(id), n.ClickID)
				if err == nil {
					out = append(out, report)
				}
			}
		}
	}
	return out
}
func (s *Server) AdminMapActors(mapID uint16) ([]AdminActorReport, error) {
	catalog := s.AssetSnapshot()
	m, ok := catalog.Maps[mapID]
	if !ok {
		return nil, errors.New("unknown map")
	}
	out := []AdminActorReport{}
	for _, n := range m.NPCs {
		report, err := actorReport(catalog, mapID, n.ClickID)
		if err != nil {
			return nil, err
		}
		out = append(out, report)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Actor.ClickID < out[j].Actor.ClickID })
	return out, nil
}

type AdminActorEdit struct {
	Version       string `json:"version"`
	ClickID       uint16 `json:"click_id"`
	Action        string `json:"action"`
	Scope         string `json:"scope"`
	LinkedQuestID uint32 `json:"linked_quest_id"`
}

// EditAdminActor ties durable linked marks to an optimistic owned-character edit.
// Visibility/frame publication is visit-local; it never edits other players' quests.
func (s *Server) EditAdminActor(ctx context.Context, id uint32, edit AdminActorEdit) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	if s.adminCharacterLoading(id) {
		return ErrAdminPlayerUnavailable
	}
	row, err := s.Store.AdminCharacter(ctx, id)
	if err != nil {
		return err
	}
	if edit.Version == "" || edit.Version != row.Version {
		return store.ErrAdminConflict
	}
	if edit.Scope != "session" && edit.Scope != "map" {
		return errors.New("actor scope must be session or map")
	}
	if edit.Action != "show" && edit.Action != "hide" && edit.Action != "open" && edit.Action != "close" {
		return errors.New("invalid actor action")
	}
	report, err := actorReport(s.Assets, row.State.Map, edit.ClickID)
	if err != nil {
		return err
	}
	c := s.friendSessions[id]
	if c != nil && (!gmIdle(c) || c.stall != nil || c.openTent != nil || c.tentOwner != 0) {
		return ErrAdminPlayerUnavailable
	}
	quest := edit.LinkedQuestID
	if quest != 0 {
		found := false
		for _, qid := range report.LinkedQuestIDs {
			if qid == quest {
				found = true
			}
		}
		if !found {
			return errors.New("quest is not linked to this prop")
		}
	}
	if quest == 0 && (edit.Action == "open" || edit.Action == "close") {
		if len(report.LinkedQuestIDs) > 1 {
			return errors.New("select one of the linked quest IDs")
		}
		if len(report.LinkedQuestIDs) == 1 {
			quest = report.LinkedQuestIDs[0]
		}
	}
	if quest != 0 {
		if edit.Action != "open" && edit.Action != "close" {
			return errors.New("linked quest edits require open or close")
		}
		next := row.State.Clone()
		if edit.Action == "open" {
			if next.Quests == nil {
				next.Quests = map[uint32]game.Quest{}
			}
			q := next.Quests[quest]
			q.ID = quest
			q.State = game.Completed
			q.Step = max(1, q.Step)
			if q.StartedAt.IsZero() {
				q.StartedAt = time.Now().UTC()
			}
			now := time.Now().UTC()
			q.CompletedAt = &now
			next.Quests[quest] = q
		} else {
			delete(next.Quests, quest)
		}
		if err = s.Store.ReplaceAdminCharacter(ctx, id, edit.Version, next); err != nil {
			return err
		}
		if c != nil {
			s.adoptSavedCharacter(c, next)
			if err := s.sendAll(c, s.World.Journal(c.character, c.view)); err != nil {
				c.conn.Close()
			}
		}
	} else if c == nil {
		return ErrAdminPlayerUnavailable
	}
	if c == nil {
		return nil
	}
	targets := []*Session{c}
	if edit.Scope == "map" {
		targets = append(targets, s.peers(c)...)
	}
	for _, target := range targets {
		if target.view == nil {
			continue
		}
		var packet []byte
		switch edit.Action {
		case "hide", "show":
			if target.view.AdminActors == nil {
				target.view.AdminActors = map[uint16]bool{}
			}
			shown := edit.Action == "show"
			target.view.AdminActors[edit.ClickID] = shown
			if shown {
				packet = s.World.ShowActor(target.view, target.character.Map, edit.ClickID)
			} else {
				packet = s.World.HideActor(target.view, target.character.Map, edit.ClickID)
			}
		case "open", "close":
			frame := byte(propIntactFrame)
			if edit.Action == "open" {
				frame = propBrokenFrame
			}
			target.view.Props[edit.ClickID] = int32(frame)
			packet = propFrame(edit.ClickID, frame)
		}
		s.sendOrClose(target, packet)
	}
	return nil
}
