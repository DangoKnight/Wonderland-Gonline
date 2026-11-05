package world

import (
	"encoding/binary"
	"image"
	"time"
)

const (
	actorWalkBytes    = 7 // AC22:2 payload: click U16, X/Y U16, speed U8.
	actorSpeedDivisor = 8 // FUN_00482980, verified in matching binary instructions.
	// A record speed of 0 leaves a zero-speed command nothing to retain;
	// the original would not move it. Walk at the server's speed byte 2.
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
	if n.Conversation {
		return true
	}
	n.stepWalk(now)
	if p[6] != 0 {
		n.walker.pixelsPerMillisecond = actorSpeed(p[6])
	} else if n.walker.pixelsPerMillisecond == 0 {
		n.walker.pixelsPerMillisecond = actorSpeed(actorCompatibilityInitialSpeed)
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

func (n *NPC) BeginConversation(now time.Time) {
	n.stepWalk(now)
	n.stopWalk()
	n.Conversation = true
}
