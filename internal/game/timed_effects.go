package game

import "errors"

type TimedItemKind byte

const (
	TimedItemNone TimedItemKind = iota
	TimedItemStatue
	TimedItemPotion
	TimedItemTraining
	TimedItemStopwatch
)
const (
	EXPMultiplierBase          = 10
	TimedMinuteSeconds         = 60
	MaxTimedSeconds            = uint32(1<<32 - 1)
	ItemEXPStatus              = 250
	ItemStopwatch       uint16 = 34180
	ItemTrainingVoucher uint16 = 34181
	ItemTrainingTicket  uint16 = 34253
	ItemTrainingFare    uint16 = 34258
)

var (
	ErrTimedIncompatible  = errors.New("Growth Statues and EXP potions cannot be used together")
	ErrTimedAlreadyActive = errors.New("This EXP potion is already active")
	ErrTimedUnavailable   = errors.New("No EXP boost is active")
)

// Remaining online seconds are durable; the session's last tick is transient.
type TimedEffects struct {
	EXPKind         TimedItemKind `json:"exp_kind,omitempty"`
	EXPTenths       uint16        `json:"exp_tenths,omitempty"`
	EXPSeconds      uint32        `json:"exp_seconds,omitempty"`
	EXPPaused       bool          `json:"exp_paused,omitempty"`
	TrainingSeconds uint32        `json:"training_seconds,omitempty"`
}

func (e TimedEffects) Multiplier() float64 {
	if e.EXPSeconds == 0 || e.EXPPaused {
		return 1
	}
	return max(1, float64(e.EXPTenths)/EXPMultiplierBase)
}
func (e *TimedEffects) Advance(seconds uint32, training bool) {
	if !e.EXPPaused {
		e.EXPSeconds -= min(e.EXPSeconds, seconds)
		if e.EXPSeconds == 0 {
			e.EXPKind = TimedItemNone
			e.EXPTenths = 0
			e.EXPPaused = false
		}
	}
	if training {
		e.TrainingSeconds -= min(e.TrainingSeconds, seconds)
	}
}
func (e *TimedEffects) Activate(d ItemDefinition) error {
	if d.TimedKind != TimedItemStopwatch && (d.TimedMinutes == 0 || ((d.TimedKind == TimedItemStatue || d.TimedKind == TimedItemPotion) && (d.EXPTenths < EXPMultiplierBase || d.EXPTenths > 255))) {
		return ErrInvalidItem
	}
	seconds := uint64(d.TimedMinutes) * TimedMinuteSeconds
	switch d.TimedKind {
	case TimedItemStopwatch:
		if e.EXPSeconds == 0 {
			return ErrTimedUnavailable
		}
		e.EXPPaused = !e.EXPPaused
	case TimedItemTraining:
		e.TrainingSeconds = uint32(min(uint64(MaxTimedSeconds), uint64(e.TrainingSeconds)+seconds))
	case TimedItemStatue, TimedItemPotion:
		if e.EXPSeconds > 0 && e.EXPKind != d.TimedKind {
			return ErrTimedIncompatible
		}
		if e.EXPSeconds > 0 && d.TimedKind == TimedItemPotion {
			return ErrTimedAlreadyActive
		}
		e.EXPSeconds = uint32(min(uint64(MaxTimedSeconds), uint64(e.EXPSeconds)+seconds))
		e.EXPKind = d.TimedKind
		e.EXPTenths = d.EXPTenths
	default:
		return ErrTimedUnavailable
	}
	return nil
}
