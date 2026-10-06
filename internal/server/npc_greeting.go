package server

import (
	"fmt"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// QuestNpc.ResolveTalkIdForNpc's default native Kelan welcome line.
const fallbackNPCGreetingTalk = 0x21284c
const fallbackDialogueStep = 1
const fallbackNPCPortrait = 3

func (s *Server) npcGreeting(c *Session, n world.NPC) error {
	// Authored events, visibility, services and resource nodes have already run.
	es := &eventSession{mapID: c.character.Map, click: n.ClickID}
	c.event = es
	es.onComplete = func() error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		return s.finishEvent(c, es)
	}
	text := s.Assets.Talks.ByID[fallbackNPCGreetingTalk]
	if text == "" {
		text = "Hello!"
	}
	return s.sendAll(c, [][]byte{
		{protocol.CommandMovement, protocol.MovementMovementLock, 1},
		dialogueFrame(fallbackDialogueStep, fallbackNPCPortrait, byte(n.ClickID), fallbackNPCGreetingTalk),
		systemLine(fmt.Sprintf(" %s: %s", n.Name, text)),
	})
}
