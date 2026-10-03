package server

import (
	"context"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

// The starter storm and beach rescue. Reference: EveEventInterpreter opcode 8 (0x7B00),
// AC20.Recv6, AC12.Recv1 and AC20.AdvanceBeachCutscene.

// Delays of the beach rescue; variables so tests need not wait.
var (
	beachIntroDelay = 800 * time.Millisecond
	beachTimeout    = 25 * time.Second
)

const (
	beachMap     = 10035
	beachAltMap  = 10039
	beachQuest   = 12040
	stormMovieOp = 0x7b00
	// The deck's chapter/shipwreck warp; door transitions use other authored IDs.
	starterShipwreckTransition = 1
	robinsonClick              = 1
)

// beachRun is one beach rescue; its pointer identifies the run for delayed steps.
type beachRun struct{ step int }

var beachDestination = world.Destination{Map: beachMap, X: 1038, Y: 2235}

// playStorm starts the ship storm movie; the client's next AC20:6 ends it at the beach.
func (s *Server) playStorm(c *Session) error {
	c.storm = true
	return s.sendAll(c, [][]byte{{protocol.CommandMovie, protocol.MovieWireCode12, 1, 0, 0, 0, 0}, {protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, 3, 5, 0, 0, 0, 2, 0x7b, 0, 0, 0, 0, 0, 0}})
}

// endStorm warps to the beach after the storm movie. Caller holds worldMu.
func (s *Server) endStorm(ctx context.Context, c *Session) error {
	c.storm = false
	c.beachPending = true
	if e := c.send([]byte{protocol.CommandEvent, protocol.EventClose}); e != nil {
		return e
	}
	return s.commandTeleport(ctx, c, beachDestination)
}

func onBeach(mapID uint16) bool { return mapID == beachMap || mapID == beachAltMap }

// arrivalScript runs after a map acknowledgment (AC12.Recv1): the beach rescue, or else
// a story arrival or the walk-in regions of a few story maps. Caller holds worldMu.
func (s *Server) arrivalScript(c *Session) error {
	_, rescued := c.character.Quests[beachQuest]
	if c.beach != nil {
		return nil
	}
	if !(c.beachPending || (onBeach(c.character.Map) && !rescued)) {
		// Authored arrivals run first, then the walk-in regions of a few story maps.
		ctx := context.Background()
		if ran, e := s.storyArrival(ctx, c); ran || e != nil {
			return e
		}
		switch c.character.Map {
		case game.MapID12001, game.MapID12050, game.MapID12052, game.MapID12055, game.MapID60001, game.MapID11032, game.MapID60002:
			_, e := s.tryRegion(ctx, c, 0, 0, false)
			return e
		}
		return nil
	}
	c.beachPending = false
	run := &beachRun{step: 1}
	c.beach = run
	s.endEvent(c)
	id := c.character.ID
	// The castaway lies on the sand (emote 9) and cannot move.
	c.emote = 9
	pose := protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(id).U8(9)
	s.broadcastWorld(c, pose)
	rescue := protocol.Builder{protocol.CommandScene, protocol.SceneActorAction, 2, 11, 0, 5}
	s.broadcastWorld(c, rescue)
	if e := s.sendAll(c, [][]byte{pose, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode30, 1}.U32(id).U8(0), {protocol.CommandEvent, protocol.EventResume}, {protocol.CommandScene, protocol.SceneActorMovement, 6, 0, 0xff, 0xff}, {protocol.CommandMovement, protocol.MovementMovementLock, 1}, rescue}); e != nil {
		return e
	}
	time.AfterFunc(beachIntroDelay, func() { s.beachIntro(c, run) })
	return nil
}

func (s *Server) beachIntro(c *Session, run *beachRun) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if c.beach != run || run.step != 1 || !onBeach(c.character.Map) {
		return
	}
	// Set the step before sending, so a fast acknowledgment is not lost.
	run.step = 2
	if c.send(eventFrame(5, 0, 0, 1, 12008, 0, 1, 0)) != nil {
		return
	}
	time.AfterFunc(beachTimeout, func() {
		s.worldMu.Lock()
		defer s.worldMu.Unlock()
		if c.beach == run && onBeach(c.character.Map) {
			s.advanceBeach(context.Background(), c, true)
		}
	})
}

// advanceBeach is AdvanceBeachCutscene; each AC20:6 moves one step. Caller holds worldMu.
func (s *Server) advanceBeach(ctx context.Context, c *Session, force bool) error {
	run := c.beach
	if run == nil {
		return nil
	}
	if force {
		run.step = 6
	}
	switch run.step {
	case 1:
		return nil // The wake-up animation has not started.
	case 2:
		next := c.character.Clone()
		next.Quests[beachQuest] = game.Quest{ID: beachQuest, State: game.InProgress, Step: 1, StartedAt: time.Now().UTC()}
		if e := s.commit(ctx, c, next); e != nil {
			return e
		}
		run.step = 3
		return s.sendAll(c, [][]byte{{protocol.CommandQuest, protocol.QuestMarkState, 8, 47, 1}, {protocol.CommandEvent, protocol.EventStepComplete}})
	case 3:
		run.step = 4
		return c.send([]byte{protocol.CommandEvent, protocol.EventStepComplete})
	case 4:
		run.step = 5
		return s.sendAll(c, [][]byte{{protocol.CommandQuest, protocol.QuestFlag, 97, 0, 1}, {protocol.CommandEvent, protocol.EventStepComplete}})
	case 5:
		run.step = 6
		return s.sendAll(c, [][]byte{{protocol.CommandScene, protocol.SceneActorAction, 1, 1, 0, 6}, {protocol.CommandEvent, protocol.EventStepComplete}})
	}
	// Step 6: release the player, then Robinson speaks.
	c.beach = nil
	c.emote = 0
	if e := s.sendAll(c, [][]byte{{protocol.CommandEvent, protocol.EventResume}, {protocol.CommandMovement, protocol.MovementMovementLock, 0}}); e != nil {
		return e
	}
	_, e := s.runNPCEvent(ctx, c, robinsonClick)
	return e
}
