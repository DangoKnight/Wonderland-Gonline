package server

import (
	"context"
	"wonderland-go/internal/protocol"
)

// movementCommand runs with worldMu held by worldCommand.
func (s *Server) movementCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	stopped := p[1] == protocol.MovementStop
	// Native stop reports have the same coordinate prefix as AC6:1, followed
	// by eight movement-check bytes. Outbound AC6:2 is a different lock packet.
	if stopped {
		if len(p) != protocol.MovementNativeRequestBytes {
			return protocol.ErrMalformed
		}
	} else if len(p) < protocol.MovementRequestMinBytes || p[1] != protocol.MovementMove {
		return protocol.ErrMalformed
	}

	r := protocol.NewReader(p[2:])
	direction, x, y := r.U8(), r.U16(), r.U16()
	validDirection := direction <= protocol.MovementDirectionMax
	if stopped {
		// Stops copy current pose/facing, including values beyond walking's 0..7.
		validDirection = direction <= protocol.MovementStopPoseMax || direction == protocol.MovementStopWirePose99
	}
	if !validDirection {
		return protocol.ErrMalformed
	}
	// AC06 ignores movement while an event holds the player.
	if c.event != nil {
		return nil
	}
	prevX, prevY := c.character.X, c.character.Y
	valid := s.World.CanMove(c.character.Map, prevX, prevY, x, y)
	nativeDestination := len(p) == protocol.MovementNativeRequestBytes
	if nativeDestination {
		// Native AC6 announces a destination before walking and resolves its
		// route locally. The previously announced destination is not the current
		// rendered position, so it cannot define a straight collision leg.
		valid = s.World.CanMove(c.character.Map, x, y, x, y)
	}
	// Land collision bits cannot reject legitimate water/air travel. Preserve
	// native vehicle movement within scene bounds for a validated active item;
	// unowned or stale riding flags never grant the exception.
	if _, mounted := c.character.Vehicle(c.character.VehicleSlot, c.character.ActiveVehicle, s.Assets.Items); mounted {
		valid = s.World.Contains(c.character.Map, x, y)
	}
	if !valid {
		s.Log.Debug("movement destination rejected", "session", c.info.ID, "map", c.character.Map, "x", x, "y", y, "previous_x", prevX, "previous_y", prevY, "native_destination", nativeDestination, "stopped", stopped)
		if nativeDestination {
			// AC7 resets the local avatar immediately. Sending the old destination
			// here teleports a still-walking client; leave invalid clicks to its UI.
			return nil
		}
		return c.send(c.character.PositionPacket())
	}
	// Keep recoverable walking in memory until a guarded checkpoint.
	if c.autosaveBaseline == nil {
		baseline := c.character.Clone()
		c.autosaveBaseline = &baseline
	}
	if x != prevX || y != prevY {
		s.stopFishing(c)
		s.cancelTrade(c)
		c.arrived = false
	}
	s.Log.Debug("movement destination accepted", "session", c.info.ID, "map", c.character.Map, "x", x, "y", y, "previous_x", prevX, "previous_y", prevY, "native_destination", nativeDestination, "stopped", stopped)
	c.character.X, c.character.Y = x, y
	packet := protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(c.character.ID).U8(direction).U16(x).U16(y)
	s.broadcastWorld(c, packet)
	if stopped {
		// Peers understand AC6:1 with the stopped endpoint. The sender has
		// already stopped locally: echoing AC6:1 could restart its movement.
		// A stop correction is not another walking step; do not charge vehicle
		// wear, roll encounters, or activate destination regions a second time.
		return nil
	}
	if e := c.send(packet); e != nil {
		return e
	}
	if c.tentOwner != 0 {
		return nil
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
