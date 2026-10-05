package world

import (
	"encoding/binary"
	"image"
	"strconv"
	"time"

	"wonderland-go/internal/protocol"
)

const (
	playerRecordBytes    = 5 // character ID and one presentation byte
	expressionFirst      = 1
	expressionLimit      = 125 // exclusive, FUN_00432478
	expressionFrameSize  = 32
	expressionFrameEvery = 200 * time.Millisecond
	expressionRepeats    = 4
	expressionHeadLeft   = 12
	expressionHeadLift   = 20
)

type Expression struct {
	Code    byte
	Started time.Time
}

// ApplyPose handles the repeated ID/action records of FUN_00438684.
// Validate the entire packet before changing any player's state.
func (w *World) ApplyPose(p []byte) bool {
	if len(p) < 1+playerRecordBytes || p[0] != protocol.PoseBroadcast || (len(p)-1)%playerRecordBytes != 0 {
		return false
	}
	for p = p[1:]; len(p) > 0; p = p[playerRecordBytes:] {
		id, action := binary.LittleEndian.Uint32(p), int32(p[4])
		if id == w.Player.ID {
			w.StopWalk()
			w.Player.Direction = action
		} else if peer := w.Peers[id]; peer != nil {
			peer.Walker.Stop(&peer.Player)
			peer.Direction = action
		}
	}
	return true
}

// StartExpression is FUN_00432478 -> FUN_00430700. Expressions have their
// own clock and never replace a held pose; repeating one restarts its strip.
func (w *World) StartExpression(id uint32, code byte, now time.Time) bool {
	if code < expressionFirst || code >= expressionLimit {
		return false
	}
	if id != w.Player.ID && w.Peers[id] == nil {
		return false
	}
	if w.Expressions == nil {
		w.Expressions = map[uint32]Expression{}
	}
	w.Expressions[id] = Expression{Code: code, Started: now}
	return true
}

// drawExpressions is FUN_004307a8: vertical E<code> strips, one 32x32
// frame every 200 ms, four passes, with the native expression offsets.
func (w *World) drawExpressions(cx, cy int, now time.Time) {
	for id, expression := range w.Expressions {
		p := &w.Player
		if id != p.ID {
			peer := w.Peers[id]
			if peer == nil {
				delete(w.Expressions, id)
				continue
			}
			p = &peer.Player
		} else if w.HidePlayer {
			continue
		}
		pic := w.Env.Pics.Find("E" + strconv.Itoa(int(expression.Code)))
		width, height := w.Env.Pics.Size(pic)
		frames := height / expressionFrameSize
		if width < expressionFrameSize || frames == 0 {
			delete(w.Expressions, id)
			continue
		}
		elapsed := max(now.Sub(expression.Started), 0)
		frame := int(elapsed / expressionFrameEvery)
		if frame >= frames*expressionRepeats {
			delete(w.Expressions, id)
			continue
		}
		dx, dy := 0, 0
		switch code := expression.Code; {
		case code >= 101 && code <= 111:
			dx, dy = 30, -5
		case code == 112 || code == 113 || code == 116:
			dx, dy = 7, 22
		case code == 114 || code == 115:
			dx = 20
		case code >= 117 && code <= 119:
			dx, dy = -2, -25
		}
		row := frame % frames
		w.Env.Pics.DrawRect(w.Env.Screen, pic, p.X-cx-expressionHeadLeft+dx, p.Y-cy-peerNameLift-expressionHeadLift+dy,
			image.Rect(0, row*expressionFrameSize, expressionFrameSize, (row+1)*expressionFrameSize), true)
	}
}

// ApplyPeerMovement preserves native stop actions (8..73 and 99), whose
// payload is an authoritative point rather than a new walking path.
func (w *World) ApplyPeerMovement(p []byte, now time.Time) bool {
	id, x, y, ok := ParseMove(p)
	if !ok {
		return false
	}
	if peer := w.Peers[id]; peer != nil {
		if p[5] >= standingAction {
			w.PlacePeer(id, x, y)
			peer.Direction = int32(p[5])
		} else {
			w.MovePeer(id, x, y, now)
		}
	}
	return true
}

// ApplyPeerEquipment is FUN_0043b2a8: ID followed by worn item words.
// The snapshot replaces all worn items, including an empty equipment set.
func (w *World) ApplyPeerEquipment(p []byte) bool {
	const equipmentIDBytes = 4
	if len(p) < equipmentIDBytes || (len(p)-equipmentIDBytes)%2 != 0 {
		return false
	}
	peer := w.Peers[binary.LittleEndian.Uint32(p)]
	if peer == nil {
		return true
	}
	var items []uint16
	for p = p[equipmentIDBytes:]; len(p) > 0; p = p[2:] {
		items = append(items, binary.LittleEndian.Uint16(p))
	}
	peer.Items = items
	Dress(peer.Role, peer.Player)
	return true
}
