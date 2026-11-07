package world

import (
	"encoding/binary"
	"time"

	"wonderland-gonline/internal/protocol"
)

const (
	battlePresenceRecordBytes = 8 // kind, player ID, reserved word, status
	battlePresencePlayer      = 2
	battlePresenceIdle        = 0
	battlePresenceSpectator   = 4 // Native spectators retain their ordinary map body.
)

type battlePresence struct {
	status  byte
	started time.Time
}

// ApplyBattlePresence is FUN_0038ce18 (AC11:4's repeated eight-byte records).
// AC11:0 clears map presentation too, but must continue to the combat handler.
func (w *World) ApplyBattlePresence(p []byte, now time.Time) bool {
	if len(p) < 2 || p[0] != protocol.CommandBattleState {
		return false
	}
	if p[1] == protocol.BattleStateWireCode0 && len(p) == 8 && binary.LittleEndian.Uint16(p[6:]) == 0 {
		delete(w.battlePresence, binary.LittleEndian.Uint32(p[2:]))
		return false
	}
	if p[1] != protocol.BattleStateParticipation || len(p) < 2+battlePresenceRecordBytes || (len(p)-2)%battlePresenceRecordBytes != 0 {
		return false
	}
	for p = p[2:]; len(p) > 0; p = p[battlePresenceRecordBytes:] {
		id, status := binary.LittleEndian.Uint32(p[1:]), p[7]
		if p[0] != battlePresencePlayer || id != w.Player.ID && w.Peers[id] == nil {
			continue
		}
		if status == battlePresenceIdle {
			delete(w.battlePresence, id)
			continue
		}
		if w.battlePresence == nil {
			w.battlePresence = map[uint32]battlePresence{}
		}
		previous, exists := w.battlePresence[id]
		if !exists || previous.status != status {
			w.battlePresence[id] = battlePresence{status, now}
			if id == w.Player.ID {
				w.StopWalk()
				w.Marker.Hide()
			} else if peer := w.Peers[id]; peer != nil {
				peer.Walker.Stop(&peer.Player)
			}
		}
	}
	return true
}

func (w *World) fighting(owner uint32) bool {
	p := w.battlePresence[owner]
	return p.status != battlePresenceIdle && p.status != battlePresenceSpectator
}

func (w *World) drawBattleCloud(owner uint32, x, y int, now time.Time) bool {
	if owner == w.Player.ID || !w.fighting(owner) {
		return false
	}
	return w.BattleCloud != nil && w.BattleCloud(w.Env.Screen, x, y, now.Sub(w.battlePresence[owner].started))
}
