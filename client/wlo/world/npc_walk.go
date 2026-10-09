package world

import (
	"encoding/binary"
	"image"
	"time"
)

const (
	actorWalkBytes    = 7 // AC22:2 payload: click U16, X/Y U16, speed U8.
	actorSpeedDivisor = 8 // FUN_00482980, verified in matching binary instructions.
	// First zero-speed command has no preceding speed to retain. Until the
	// native EVE initial speed is projected, match the server's speed byte 2.
	actorCompatibilityInitialSpeed = 2
)

// ApplyActorWalk follows server destinations, not client-generated wandering.
// FUN_0041869c retains the preceding speed when the speed byte is zero. An
// interrupted leg first advances to its current position, never its old endpoint.
func (w *World) ApplyActorWalk(p []byte, now time.Time) bool {
	if len(p) != actorWalkBytes {
		return false
	}
	n := w.NPCs[binary.LittleEndian.Uint16(p)]
	if n == nil {
		return true // Native dispatcher ignores unknown click IDs.
	}
	n.stepWalk(now)
	if p[6] != 0 {
		n.walker.pixelsPerMillisecond = float64(p[6]) / actorSpeedDivisor * walkSpeed
	} else if n.walker.pixelsPerMillisecond == 0 {
		n.walker.pixelsPerMillisecond = float64(actorCompatibilityInitialSpeed) / actorSpeedDivisor * walkSpeed
	}
	pos := Player{X: n.X, Y: n.Y, Direction: int32(n.Action)}
	goal := image.Pt(int(binary.LittleEndian.Uint16(p[2:])), int(binary.LittleEndian.Uint16(p[4:])))
	if goal == image.Pt(n.X, n.Y) {
		n.stopWalk()
		return true
	}
	n.walker.Start(&pos, []image.Point{goal}, now)
	n.Action = int(pos.Direction)
	return true
}

func (n *NPC) stepWalk(now time.Time) {
	if !n.walker.Walking() {
		return
	}
	pos := Player{X: n.X, Y: n.Y, Direction: int32(n.Action)}
	n.walker.Step(&pos, now)
	n.X, n.Y, n.Action = pos.X, pos.Y, int(pos.Direction)
}

func (n *NPC) stopWalk() {
	pos := Player{X: n.X, Y: n.Y, Direction: int32(n.Action)}
	n.walker.Stop(&pos)
	n.Action = int(pos.Direction)
}
