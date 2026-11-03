package battle

import (
	"time"
	"wonderland-gonline/internal/protocol"
)

// FUN_0039b58c raises the defeated sprite 300 pixels at one pixel/ms, then
// completes removal. AC53:3 is specifically this flight, not a generic death.
const OverkillFlightDuration = 300 * time.Millisecond

func (b *Battle) appendOverkill(steps []Step, f *Fighter) []Step {
	return b.appendOverkills(steps, f)
}

func (b *Battle) appendOverkills(steps []Step, fighters ...*Fighter) []Step {
	step := Step{Delay: OverkillFlightDuration}
	for _, f := range fighters {
		if !f.Overkilled || f.overkillAnnounced {
			continue
		}
		f.overkillAnnounced = true
		if f.Kind == Player {
			for _, owned := range b.all() {
				if owned.Kind == Pet && owned.Owner == f.ID {
					owned.Withdrawn = true
				}
			}
		}
		step.Packets = append(step.Packets, []byte{protocol.CommandBattleEffect, protocol.BattleEffectOverkill, f.X, f.Y})
		step.Overkills = append(step.Overkills, f)
	}
	if len(step.Overkills) == 0 {
		return steps
	}
	return append(steps, step)
}
