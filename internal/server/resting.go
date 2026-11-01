package server

import (
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// Resting recovery is a server rule of this port: neither aLogin nor the
// reference server defines one. A character holding a sitting or lying
// pose outside battles and events recovers restRecoveryPercent of their
// maximum HP and SP (at least one point) every restRecoveryInterval.
const (
	restRecoveryInterval = 3 * time.Second
	restRecoveryPercent  = 2
)

func restingPose(pose byte) bool {
	return (pose >= protocol.PoseLieFirst && pose <= protocol.PoseLieLast) ||
		(pose >= protocol.PoseSitFirst && pose <= protocol.PoseSitLast)
}

// restCharacters runs on the one-second world ticker. Recovery is a
// buffered session field like walking: it updates memory, reports AC8
// current HP/SP, and leaves persistence to the guarded checkpoint.
func (s *Server) restCharacters(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	for _, c := range s.world {
		s.rest(c, now)
	}
}

// rest recovers one session. Caller holds worldMu.
func (s *Server) rest(c *Session, now time.Time) {
	if !c.ready || c.character == nil || c.battle != nil || c.event != nil || !restingPose(c.emote) {
		c.restedAt = time.Time{}
		return
	}
	if c.restedAt.IsZero() {
		c.restedAt = now
		return
	}
	if now.Sub(c.restedAt) < restRecoveryInterval {
		return
	}
	c.restedAt = now
	full := c.character.Combat(s.Assets.Items)
	maxHP, maxSP := uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	gain := func(current, maximum uint32) uint32 {
		if current >= maximum {
			return current
		}
		return min(maximum, current+max(1, maximum*restRecoveryPercent/100))
	}
	hp, sp := gain(c.character.HP, maxHP), gain(c.character.SP, maxSP)
	if hp == c.character.HP && sp == c.character.SP {
		return
	}
	if c.autosaveBaseline == nil {
		baseline := c.character.Clone()
		c.autosaveBaseline = &baseline
	}
	var packets [][]byte
	if hp != c.character.HP {
		c.character.HP = hp
		packets = append(packets, vitalPacket(game.StatCurrentHP, hp))
	}
	if sp != c.character.SP {
		c.character.SP = sp
		packets = append(packets, vitalPacket(game.StatCurrentSP, sp))
	}
	if err := s.sendAll(c, packets); err != nil {
		s.Log.Debug("resting recovery report failed", "session", c.info.ID, "error", err)
	}
}

// vitalPacket is the AC8 absolute stat update StatPackets sends for one vital.
func vitalPacket(stat byte, value uint32) []byte {
	return protocol.Builder{protocol.CommandStats, protocol.StatsStatUpdate, stat, protocol.StatsValueAbsolute}.U32(value).U32(0)
}
