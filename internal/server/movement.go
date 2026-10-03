package server

import (
	"context"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// movementCommand runs with worldMu held by worldCommand.
func (s *Server) movementCommand(ctx context.Context, c *Session, p []byte) error {
	// AC06.Recv1 consumes only the direction and coordinates; the native
	// client sends an additional eight bytes after this minimum layout.
	if len(p) < protocol.MovementRequestMinBytes || p[1] != protocol.MovementMove {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	direction, x, y := r.U8(), r.U16(), r.U16()
	if direction > protocol.MovementDirectionMax {
		return protocol.ErrMalformed
	}
	// AC06 ignores movement while an event holds the player.
	if c.event != nil {
		return nil
	}
	prevX, prevY := c.character.X, c.character.Y
	if e := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(char *game.Character) error { char.X = x; char.Y = y; return nil }); e != nil {
		return e
	}
	if x != prevX || y != prevY {
		s.cancelTrade(c)
		c.arrived = false
	}
	c.character.X, c.character.Y = x, y
	packet := protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(c.character.ID).U8(direction).U16(x).U16(y)
	s.broadcastWorld(c, packet)
	if e := c.send(packet); e != nil {
		return e
	}
	if ran, e := s.tryRegion(ctx, c, prevX, prevY, true); ran || e != nil {
		return e
	}
	if x != prevX || y != prevY {
		if err := s.wearVehicle(ctx, c); err != nil {
			return err
		}
	}
	return s.stepEncounter(c, prevX, prevY)
}
