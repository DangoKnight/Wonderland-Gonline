package app

import (
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
)

// FUN_00393c58 -> FUN_00392484 state 7 -> FUN_0039b58c: rise one pixel
// per millisecond and remove after 300 pixels. This never runs for HP zero alone.
const (
	battleOverkillRisePixels = 300
	battleOverkillDuration   = 300 * time.Millisecond
)

func (c *Client) battleOverkills(now time.Time) {
	v := &c.battle
	for cell, at := range v.overkills {
		if at.IsZero() {
			pending := false
			for _, a := range v.state.Actions {
				for _, target := range a.Targets {
					pending = pending || target.Cell == cell && target.Generation == v.shown[cell].Generation
				}
			}
			for _, p := range v.playing {
				m := clientbattle.Motion{}
				if v.assets != nil {
					m = v.assets.Motions[p.action.Skill]
				}
				cues := len(m.Events)
				if m.DamageMode == 0 || cues == 0 {
					cues = 1
				}
				allHits := uint64(1)<<cues - 1
				if p.hits&allHits == allHits {
					continue
				}
				for _, target := range p.action.Targets {
					pending = pending || target.Cell == cell && target.Generation == v.shown[cell].Generation
				}
			}
			if pending {
				continue
			}
			f := v.shown[cell]
			f.HP, f.Removed = 0, false
			v.shown[cell] = f
			v.overkills[cell] = now
			continue
		}
		if now.Sub(at) >= battleOverkillDuration {
			f := v.shown[cell]
			f.Removed = true
			v.shown[cell] = f
			delete(v.overkills, cell)
		}
	}
}
