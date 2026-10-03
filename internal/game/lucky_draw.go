package game

import "time"

const (
	LuckyDrawsPerDay    = 3
	LuckyDrawMaxRewards = 20 // Native Lottery form has twenty presentation slots.
)

// LuckyDrawState uses UTC calendar dates. An older clock cannot replenish draws.
type LuckyDrawState struct {
	Day  string `json:"day,omitempty"`
	Used byte   `json:"used,omitempty"`
}

func (s LuckyDrawState) Remaining(now time.Time) byte {
	today := now.UTC().Format(time.DateOnly)
	if s.Day == "" || s.Day < today {
		return LuckyDrawsPerDay
	}
	if s.Day > today || s.Used >= LuckyDrawsPerDay {
		return 0
	}
	return LuckyDrawsPerDay - s.Used
}

func (s *LuckyDrawState) Consume(now time.Time) bool {
	if s.Remaining(now) == 0 {
		return false
	}
	today := now.UTC().Format(time.DateOnly)
	if s.Day != today {
		s.Day = today
		s.Used = 0
	}
	s.Used++
	return true
}

func (s LuckyDrawState) Valid() bool {
	if s.Day == "" {
		return s.Used == 0
	}
	_, err := time.Parse(time.DateOnly, s.Day)
	return err == nil && s.Used <= LuckyDrawsPerDay
}
