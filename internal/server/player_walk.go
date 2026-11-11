package server

import (
	"context"
	"math"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
)

const (
	// FUN_004126b8 advances native players 0.16 scene pixels per millisecond.
	playerWalkPixelsPerSecond = 160
	// Count traversed native walk-grid cells, not destination announcements.
	encounterWalkStepPixels = assets.TerrainCellSize
)

// A native AC6:1 announces one straight route leg, not an arrival. World state
// retains the reached position; the destination is only transient animation state.
type playerWalk struct {
	active                               bool
	mapID                                uint16
	fromX, fromY, toX, toY, lastX, lastY uint16
	started                              time.Time
	distance                             float64
	worn                                 bool
	direction                            byte
}

// Caller holds worldMu. Called both by the world ticker and before commands so
// contact/stop packets arriving between ticks see the same reached position.
func (s *Server) advancePlayerWalk(c *Session, now time.Time) error {
	w := &c.walk
	if !w.active {
		return nil
	}
	if c.character == nil || !c.ready || c.character.Map != w.mapID || c.character.X != w.lastX || c.character.Y != w.lastY || c.battle != nil || c.event != nil || c.party != nil && c.party.leader() != c {
		*w = playerWalk{}
		return nil
	}
	length := math.Hypot(float64(int(w.toX)-int(w.fromX)), float64(int(w.toY)-int(w.fromY)))
	fraction := 1.0
	if length > 0 {
		fraction = min(1, max(0, now.Sub(w.started).Seconds()*playerWalkPixelsPerSecond/length))
	}
	x := uint16(math.Round(float64(w.fromX) + float64(int(w.toX)-int(w.fromX))*fraction))
	y := uint16(math.Round(float64(w.fromY) + float64(int(w.toY)-int(w.fromY))*fraction))
	prevX, prevY := c.character.X, c.character.Y
	if x == prevX && y == prevY {
		if fraction == 1 {
			w.active = false
		}
		return nil
	}
	if c.autosaveBaseline == nil {
		baseline := c.character.Clone()
		c.autosaveBaseline = &baseline
	}
	segments := max(1, int(math.Ceil(math.Hypot(float64(int(x)-int(prevX)), float64(int(y)-int(prevY)))/encounterWalkStepPixels)))
	for i := 1; i <= segments; i++ {
		nx := uint16(math.Round(float64(prevX) + float64(int(x)-int(prevX))*float64(i)/float64(segments)))
		ny := uint16(math.Round(float64(prevY) + float64(int(y)-int(prevY))*float64(i)/float64(segments)))
		ox, oy := c.character.X, c.character.Y
		w.distance += math.Hypot(float64(int(nx)-int(ox)), float64(int(ny)-int(oy)))
		steps := int(w.distance / encounterWalkStepPixels)
		w.distance -= float64(steps * encounterWalkStepPixels)
		c.character.X, c.character.Y = nx, ny
		c.arrived = false
		w.lastX, w.lastY = nx, ny
		s.movePartyFollowers(c)
		if !w.worn || s.starterBeachQuestRaft(c) {
			w.worn = true
			if err := s.wearVehicle(context.Background(), c); err != nil {
				return err
			}
		}
		if c.tentOwner == 0 {
			if ran, err := s.tryRegion(context.Background(), c, ox, oy, true); ran || err != nil {
				*w = playerWalk{}
				return err
			}
			if err := s.stepEncounterProgress(c, ox, oy, steps, now); err != nil {
				return err
			}
		}
		if c.battle != nil || c.event != nil {
			*w = playerWalk{}
			return nil
		}
	}
	if fraction == 1 {
		w.active = false
	}
	return nil
}

func playerWalkPacket(c *Session) []byte {
	if !c.walk.active || c.character == nil || c.battle != nil || c.event != nil || c.character.Map != c.walk.mapID || c.character.X != c.walk.lastX || c.character.Y != c.walk.lastY {
		return nil
	}
	return protocol.Builder{protocol.CommandMovement, protocol.MovementMove}.U32(c.character.ID).U8(c.walk.direction).U16(c.walk.toX).U16(c.walk.toY)
}
