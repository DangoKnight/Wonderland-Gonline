// Package lottery implements the native daily Lucky Draw (AC104:1).
package lottery

import (
	"encoding/binary"
	"time"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	catalogHeaderBytes = 5
	rewardRecordBytes  = 3
	resultPacketBytes  = 5
	// FUN_0015af1c cycles quickly for 1600 ms, slows, then sends at 2800 ms.
	spinFastDuration = 1600 * time.Millisecond
	spinInterval     = 80 * time.Millisecond
	SpinDuration     = 2800 * time.Millisecond
	ReplyTimeout     = 15 * time.Second
)

type Reward struct {
	ID       uint16
	Quantity byte
}

// State contains presentation only. Usage and the winning slot come from the
// server; preview animation never grants inventory or consumes an allowance.
type State struct {
	Rewards []Reward
	Used    byte
	Ready   bool
	Slot    int // One-based server result; zero means no confirmed reward.
	Rolling bool
	Pending bool
	display int
	started time.Time
	sent    time.Time
}

func (s *State) Remaining() int { return max(0, game.LuckyDrawsPerDay-int(s.Used)) }
func (s *State) Busy() bool     { return s.Rolling || s.Pending }

// Apply validates the whole native packet before changing state.
func (s *State) Apply(p []byte) bool {
	if len(p) < catalogHeaderBytes || p[0] != protocol.CommandLuckyDraw || p[1] != protocol.LuckyDrawMode {
		return false
	}
	switch p[2] {
	case protocol.LuckyDrawCatalog:
		used, count := p[3], int(p[4])
		if used > game.LuckyDrawsPerDay || count > game.LuckyDrawMaxRewards || len(p) != catalogHeaderBytes+count*rewardRecordBytes {
			return false
		}
		rewards := make([]Reward, count)
		for i := range rewards {
			row := p[catalogHeaderBytes+i*rewardRecordBytes:]
			rewards[i] = Reward{binary.LittleEndian.Uint16(row), row[2]}
			if rewards[i].ID == 0 || rewards[i].Quantity == 0 {
				return false
			}
		}
		if used > s.Used || used == game.LuckyDrawsPerDay || count == 0 {
			s.Pending = false
		}
		s.Rewards, s.Used, s.Ready = rewards, used, true
		s.Rolling, s.Slot, s.display = false, 0, 0
		return true
	case protocol.LuckyDrawResult:
		slot, used := int(p[3]), p[4]
		if len(p) != resultPacketBytes || !s.Ready || slot < 1 || slot > len(s.Rewards) || used < 1 || used > game.LuckyDrawsPerDay {
			return false
		}
		s.Used, s.Slot, s.display = used, slot, slot
		s.Rolling, s.Pending = false, false
		return true
	}
	return false
}

func (s *State) Start(now time.Time) bool {
	if !s.Ready || len(s.Rewards) == 0 || s.Remaining() == 0 || s.Busy() {
		return false
	}
	s.Rolling, s.started, s.Slot, s.display = true, now, 0, 1
	return true
}

// Advance runs without drawing, including in background sessions. The caller
// sends exactly once, after the presentation spin, and never retries a draw.
func (s *State) Advance(now time.Time) (send, timedOut bool) {
	if s.Rolling {
		elapsed := max(now.Sub(s.started), 0)
		if elapsed >= SpinDuration {
			s.Rolling, s.Pending, s.sent = false, true, now
			return true, false
		}
		step := int(min(elapsed, spinFastDuration) / spinInterval)
		for _, at := range []time.Duration{1800, 2000, 2300} {
			if elapsed >= at*time.Millisecond {
				step++
			}
		}
		s.display = step%len(s.Rewards) + 1
	}
	if s.Pending && now.Sub(s.sent) >= ReplyTimeout {
		s.Fail()
		return false, true
	}
	return false, false
}

// CancelAnimation can cancel an unsent spin. A sent request remains pending
// across window closure so reopening cannot submit a second draw.
func (s *State) CancelAnimation() { s.Rolling = false }
func (s *State) Fail() {
	s.Rolling, s.Pending, s.Slot, s.display = false, false, 0, 0
}
func (s *State) Reset() { *s = State{} }

func (s *State) DisplayReward() (Reward, bool) {
	if s.display < 1 || s.display > len(s.Rewards) {
		return Reward{}, false
	}
	return s.Rewards[s.display-1], true
}
