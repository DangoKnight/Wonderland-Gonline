package app

import (
	"image"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
)

// The decoded roster remains authoritative for command validation. Only the
// displayed vitals and visibility wait for animation cues. Absolute stat and
// defeat packets received during an attack reconcile when its track drains.
func (v *battleView) syncShown() {
	if v.shown == nil {
		v.shown = map[clientbattle.Cell]clientbattle.Fighter{}
	}
	pending := len(v.state.Actions) > 0 || len(v.playing) > 0
	for _, f := range v.state.Fighters {
		old, exists := v.shown[f.Cell]
		if exists && old.ID == f.ID && old.Owner == f.Owner && old.Kind == f.Kind && old.Generation == f.Generation {
			if pending {
				f.HP, f.SP, f.Removed = old.HP, old.SP, old.Removed
			}
			if _, leaving := v.departures[f.Cell]; leaving {
				f.Removed = old.Removed
			}
			if _, flying := v.overkills[f.Cell]; flying {
				f.Removed = old.Removed
			}
		}
		v.shown[f.Cell] = f
	}
}

func (v *battleView) displayed(f clientbattle.Fighter) clientbattle.Fighter {
	if shown, ok := v.shown[f.Cell]; ok && shown.ID == f.ID && shown.Owner == f.Owner && shown.Kind == f.Kind && shown.Generation == f.Generation {
		return shown
	}
	return f
}

func (c *Client) battleDepartureMotion() clientbattle.Motion {
	if assets := c.battle.assets; assets != nil {
		if motion, ok := assets.Motions[battleFleeSkill]; ok && len(motion.Frames) > 0 {
			return motion.Departure()
		}
	}
	// Compatibility for missing MBTM data: retain motion 60041's outward path.
	return (clientbattle.Motion{Frames: []clientbattle.MotionFrame{
		{Action: battleDeparturePose, Speed: battleDepartureSpeed, Offset: image.Pt(-ScreenWidth, 0)},
	}}).Departure()
}

const (
	battleDeparturePose  = 2 // Native 60041's authored action; ActionAt mirrors it.
	battleDepartureSpeed = .6
)

// FUN_00392bb4 -> FUN_0037896c selects motion 60041; FUN_00385e58
// completes it through FUN_0039b81c before the map fade. AC11:1's coordinate
// departure records use that presentation for living units.
func (c *Client) battleDepartures(now time.Time) {
	v := &c.battle
	c.departingPetsTick(now)
	if len(v.playing) > 0 || len(v.state.Actions) > 0 {
		return
	}
	motion := c.battleDepartureMotion()
	for cell, at := range v.departures {
		f := v.shown[cell]
		if f.HP == 0 || f.Removed || !at.IsZero() && now.Sub(at) >= motion.Duration() {
			f.Removed = true
			v.shown[cell] = f
			delete(v.departures, cell)
		} else if at.IsZero() {
			v.departures[cell] = now
		}
	}
}

func (c *Client) finishBattlePresentation() {
	if c.battleResults.active {
		c.battleResults.closed = true
	}
	v := &c.battle
	v.state = clientbattle.State{Self: v.state.Self, Turn: v.state.Turn}
	c.closeBattleView()
	c.startSceneFade(sceneFadeLeaveBattle)
}
