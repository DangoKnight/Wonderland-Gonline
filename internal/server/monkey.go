package server

import (
	"context"
	"slices"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

const (
	monkeyTemplate           = 17162
	monkeyMap                = 11016
	monkeyActor              = 1
	monkeyRescueMark         = 12002
	monkeyFollowMark         = 12003
	monkeyFirstTalk          = 20038
	monkeySqueakTalk         = 20042
	monkeyFullPartyTalk      = 31146
	monkeyNPCPortrait        = 3
	monkeyPlayerPortrait     = 7
	monkeyMovieFrameKind     = 5
	monkeyMovieHoldMode      = 2
	nativeMoviePlay          = 1
	nativeMoviePlayAlternate = 2
)

type monkeyLine struct {
	talk     uint32
	portrait byte
}

type monkeyMovieStep struct {
	op     world.Op
	branch byte
}

// monkeyMovies preserves the movie operations from the authored rescue branch.
// Recruitment is still committed atomically by finishMonkey, rather than by the
// authored branch's separate companion and quest-mark writes.
func (s *Server) monkeyMovies(mapID, click uint16) []monkeyMovieStep {
	if mapID != monkeyMap {
		return nil
	}
	for _, ev := range s.World.NPCEvents(mapID, click) {
		for _, br := range ev.Branches {
			var movies []monkeyMovieStep
			for _, raw := range br.Operations {
				op := world.DecodeOp(raw)
				if op.Code == world.ActionMovie && (op.D1 == nativeMoviePlay || op.D1 == nativeMoviePlayAlternate) {
					movies = append(movies, monkeyMovieStep{op: op, branch: br.Index})
				}
			}
			if len(movies) != 0 {
				return movies
			}
		}
	}
	return nil
}

// startMonkey plays the authored rescue movies when available, retaining the
// sixteen-step dialogue for legacy definitions without movie operations. Only a linked,
// enabled native event may invoke it; range/visibility checks still precede it.
func (s *Server) startMonkey(ctx context.Context, c *Session, click uint16) error {
	_, owned := c.character.Pet(monkeyTemplate)
	done := c.character.Quests[monkeyRescueMark].State == game.Completed
	lines := []monkeyLine{{monkeySqueakTalk, monkeyNPCPortrait}}
	recruit := !owned && !done
	var movies []monkeyMovieStep
	if recruit {
		if _, known := s.World.Template(monkeyTemplate); !known {
			return s.sendAll(c, [][]byte{headBanner("The companion definition is unavailable."), {protocol.CommandEvent, protocol.EventResume}})
		}
		portraits := []byte{7, 3, 7, 7, 3, 7, 7, 3, 7, 7, 3, 7, 3, 7, 7, 3}
		count := len(portraits)
		if len(c.character.Pets) >= game.MaxPets {
			count = 11
		}
		lines = make([]monkeyLine, 0, count+2)
		for i := 0; i < count; i++ {
			lines = append(lines, monkeyLine{uint32(monkeyFirstTalk + i), portraits[i]})
		}
		movies = s.monkeyMovies(c.character.Map, click)
		if len(movies) != 0 {
			// The cinematic already contains the rescue dialogue.
			lines = nil
		}
		if count < len(portraits) {
			lines = append(lines, monkeyLine{monkeyFullPartyTalk, monkeyPlayerPortrait}, monkeyLine{20048, monkeyNPCPortrait})
			recruit = false
		}
	}
	s.endEvent(c)
	ev := assets.Event{Branches: []assets.Branch{{Index: 1}}}
	es := &eventSession{mapID: c.character.Map, click: click, ev: &ev}
	c.event = es
	if err := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); err != nil {
		return err
	}
	index, movieIndex := 0, 0
	var advance func() error
	advance = func() error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		if movieIndex < len(movies) {
			movie := movies[movieIndex]
			op := movie.op
			movieIndex++
			es.onComplete = advance
			return c.send(eventFrame(monkeyMovieFrameKind, 0, 0, monkeyMovieHoldMode, op.Value(), 0, op.Index, movie.branch))
		}
		if index == len(lines) {
			if recruit {
				return s.finishMonkey(ctx, c, es)
			}
			return s.finishEvent(c, es)
		}
		line := lines[index]
		index++
		speaker := byte(click)
		if line.portrait == monkeyPlayerPortrait {
			speaker = 0
		}
		es.onComplete = advance
		return c.send(dialogueFrame(byte(index), line.portrait, speaker, line.talk))
	}
	return advance()
}
func (s *Server) finishMonkey(ctx context.Context, c *Session, es *eventSession) error {
	next := c.character.Clone()
	if _, owned := next.Pet(monkeyTemplate); owned || next.Quests[monkeyRescueMark].State == game.Completed {
		return s.finishEvent(c, es)
	}
	if next.FreePetSlot() == 0 || slices.ContainsFunc(next.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, monkeyTemplate) }) {
		if err := c.send(headBanner("Make room in your party and retrieve S.Monkey from Pet Hotel before continuing.")); err != nil {
			return err
		}
		return s.finishEvent(c, es)
	}
	template := s.petTemplate(monkeyTemplate)
	pet := game.NewPet(monkeyTemplate, "S.Monkey", next.FreePetSlot(), template, s.Assets.Items)
	if i := slices.IndexFunc(next.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, monkeyTemplate) }); i >= 0 {
		pet = next.ReservePets[i]
		pet.Slot = next.FreePetSlot()
		next.ReservePets = slices.Delete(next.ReservePets, i, i+1)
	}
	pet.EnsureSkills(template, s.hasSkill)
	pet.Normalize(s.Assets.Items, true)
	next.Pets = append(next.Pets, pet)
	now := time.Now().UTC()
	next.Quests[monkeyRescueMark] = game.Quest{ID: monkeyRescueMark, State: game.Completed, Step: 1, StartedAt: now, CompletedAt: &now}
	if q := next.Quests[monkeyFollowMark]; q.State != game.Completed && (q.State != game.InProgress || q.Step < 1) {
		next.Quests[monkeyFollowMark] = game.Quest{ID: monkeyFollowMark, State: game.InProgress, Step: 1, StartedAt: now}
	}
	// The companion and both marks commit together before any success or hide.
	if err := s.commit(ctx, c, next); err != nil {
		return err
	}
	c.view.Hidden[es.click] = true
	packets := [][]byte{protocol.Builder{protocol.CommandScene, protocol.SceneActorHide}.U16(es.click).U16(0xffff)}
	if c.pets.register(pet.ID) {
		packets = append(packets, pet.RecruitPacket(next.ID, template))
	}
	slot := c.pets.slot(pet.ID)
	packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items)...)
	packets = append(packets, petNamePacket(next.ID, slot, s.companionName(pet.ID, pet.Name)))
	if list := s.petListPacket(c.character, c.pets); list != nil {
		packets = append(packets, list)
	}
	for _, mark := range []uint32{monkeyRescueMark, monkeyFollowMark} {
		packets = append(packets, s.World.QuestUpdate(c.view, mark, next.Quests[mark])...)
	}
	packets = append(packets, systemLine("S.Monkey has joined your party!"), []byte{protocol.CommandEvent, protocol.EventStepComplete})
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.finishEvent(c, es)
}
