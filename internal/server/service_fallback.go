package server

import "wonderland-go/internal/protocol"

import (
	"context"
	"strings"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

// serviceKind follows QuestNpc.Interact's priority and name/template heuristics.
func serviceKind(n world.NPC) byte {
	name := strings.ToLower(n.Name)
	contains := func(parts ...string) bool {
		for _, part := range parts {
			if strings.Contains(name, part) {
				return true
			}
		}
		return false
	}
	switch {
	case contains("props keep", "storage") || n.Template == 14134:
		return 4
	case contains("stock keep", "bank", "vault", "exchanger") || n.Template == 14181 || n.Template == 14157:
		return 9
	case contains("doctor", "witch", "clinic") || n.Template == 14151:
		return 6
	case contains("pet hotel", "pet keep", "hotel") || n.Template == 14182 || n.Template == 14152:
		return 5
	case contains("props shop", "weapon shop", "armor shop") || n.Template == 13007 || n.Template == 13006 || n.Template == 13005:
		if n.Template == 13006 || contains("weapon") {
			return 1
		}
		return 2 // Legacy armor fallback uses the props-sale catalog too.
	}
	return 0
}

func serviceQuestion(click uint16, question, branch byte) []byte {
	return []byte{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, 1, 6, 3, byte(click), 0, 0, 0, 0, 0, 0, question, 0, branch}
}

// fallbackService only runs when EVE has no event for this actor. Its callbacks
// use the same session token and cancellation path as native dialogue.
func (s *Server) fallbackService(ctx context.Context, c *Session, n world.NPC) (bool, error) {
	kind := serviceKind(n)
	if kind == 0 {
		return false, nil
	}
	if kind == 4 || kind == 9 || kind == 5 {
		return true, s.openService(c, uint16(kind))
	}
	es := &eventSession{mapID: c.character.Map, click: n.ClickID}
	c.event = es
	if err := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); err != nil {
		return true, err
	}
	if kind == 6 {
		es.onChoice = func(choice byte) error {
			if c.event != es || c.character.Map != es.mapID {
				return nil
			}
			switch choice {
			case 30, 1:
				if err := s.openService(c, 6); err != nil {
					return err
				}
			case 31, 2:
				next := c.character.Clone()
				next.RecordPoint = &game.Location{Map: next.Map, X: next.X, Y: next.Y}
				if err := s.commit(ctx, c, next); err != nil {
					return err
				}
				if err := s.sendAll(c, [][]byte{dialogueFrame(1, 3, byte(n.ClickID), 0x0379b6), recordPointStatus(next), {protocol.CommandEvent, protocol.EventStepComplete}, {protocol.CommandEvent, protocol.EventResume}}); err != nil {
					return err
				}
			case 32:
				if err := s.openService(c, 5); err != nil {
					return err
				}
			}
			return s.cancelInteraction(c)
		}
		return true, c.send(serviceQuestion(n.ClickID, 3, 1))
	}
	initial, second := byte(5), byte(6)
	if kind == 1 {
		initial, second = 9, 8
	}
	es.onChoice = func(choice byte) error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		if choice != 30 && choice != 1 {
			if err := s.sendAll(c, [][]byte{{protocol.CommandShop, protocol.ShopPropsWindow}, {protocol.CommandEvent, protocol.EventChoice}, {protocol.CommandEvent, protocol.EventResume}}); err != nil {
				return err
			}
			return s.cancelInteraction(c)
		}
		es.onChoice = func(choice byte) error {
			if c.event != es || c.character.Map != es.mapID {
				return nil
			}
			if err := c.send([]byte{protocol.CommandEvent, protocol.EventResume}); err != nil {
				return err
			}
			if choice == 30 || choice == 31 {
				if err := s.openService(c, uint16(kind)); err != nil {
					return err
				}
			}
			return s.cancelInteraction(c)
		}
		return c.send(serviceQuestion(n.ClickID, second, 2))
	}
	return true, c.send(serviceQuestion(n.ClickID, initial, 1))
}
