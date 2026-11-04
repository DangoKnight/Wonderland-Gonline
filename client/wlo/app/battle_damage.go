package app

import (
	"fmt"
	"image"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/minigame"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

const (
	battleDamageInk              = 0xf800
	battleCriticalInk            = 0xffe0
	battleSPInk                  = 0x001f
	battleDamagePopupGap         = 24 // Fallback for text-rendered numbers.
	battleDamagePopupPadding     = 4  // Pixels between native bitmap digit boxes.
	battleFloatRiseTick          = 30 * time.Millisecond
	battleFloatOffsetPixels      = 60
	battleMaxDigitAmount         = uint64(^uint(0) >> 1) // Bitmap number renderer accepts int.
	battleFloatFlightStepPixels  = 3                     // FUN_003a2e08 advances native horizontal travel by three.
	battleFloatFlightRangePixels = 100
	battleFloatFlightArcPixels   = 60
)

type battleDamageGroup struct {
	target     clientbattle.Cell
	generation uint64
	stat       byte
	critical   bool
	parts      []uint64
}

type battleDamageResultKey struct {
	target     clientbattle.Cell
	generation uint64
	stat       byte
}

type battleDamageKey struct {
	batch      uint64
	unbatched  int
	target     clientbattle.Cell
	generation uint64
	stat       byte
}

type battleDamageContribution struct {
	play     int
	amount   uint32
	critical bool
}

func battleDamageStat(result clientbattle.StatResult) bool {
	return (result.Stat == game.StatCurrentHP || result.Stat == game.StatCurrentSP) && (result.Mode == protocol.BattleStatDamage || result.Mode == protocol.BattleStatCriticalDamage)
}

// Only records from the same native animation packet may form a damage group.
// One multi-hit skill pools the singles too; multiple different multi-hit skills
// each keep their own group, with a separate combined group for the singles.
func planBattleDamage(plays []battlePlay, assets *clientbattle.Assets) {
	byTarget := map[battleDamageKey][]battleDamageContribution{}
	mixed := map[battleDamageKey]bool{}
	var keys []battleDamageKey
	motion := func(index int) clientbattle.Motion {
		if assets != nil {
			return assets.Motions[plays[index].action.Skill]
		}
		return clientbattle.Motion{}
	}
	for i := range plays {
		for _, target := range plays[i].action.Targets {
			if target.Result != protocol.BattleHitLanded {
				continue
			}
			for _, result := range target.Stats {
				if result.Stat != game.StatCurrentHP && result.Stat != game.StatCurrentSP {
					continue
				}
				key := battleDamageKey{batch: plays[i].action.Batch, target: target.Cell, generation: target.Generation, stat: result.Stat}
				if key.batch == 0 {
					key.unbatched = i + 1
				}
				if !battleDamageStat(result) {
					if result.Amount > 0 {
						mixed[key] = true
					}
					continue
				}
				if _, ok := byTarget[key]; !ok {
					keys = append(keys, key)
				}
				byTarget[key] = append(byTarget[key], battleDamageContribution{play: i, amount: result.Amount, critical: result.Mode == protocol.BattleStatCriticalDamage})
			}
		}
	}
	for _, key := range keys {
		// Combining damage around an interleaved recovery would change HP/SP
		// clamping. Preserve the original stat order for that resource instead.
		if mixed[key] {
			continue
		}
		contributions := byTarget[key]
		for _, contribution := range contributions {
			play := &plays[contribution.play]
			if play.damagePlanned == nil {
				play.damagePlanned = map[battleDamageResultKey]bool{}
			}
			play.damagePlanned[battleDamageResultKey{key.target, key.generation, key.stat}] = true
		}
		multi := map[uint16]bool{}
		multiLeader := -1
		for _, contribution := range contributions {
			if motion(contribution.play).HitCount() > 1 {
				multi[plays[contribution.play].action.Skill] = true
				if multiLeader < 0 {
					multiLeader = contribution.play
				}
			}
		}
		type family struct {
			multi bool
			skill uint16
		}
		families := map[family][]battleDamageContribution{}
		var order []family
		for _, contribution := range contributions {
			var kind family
			if len(multi) > 1 && motion(contribution.play).HitCount() > 1 {
				kind = family{multi: true, skill: plays[contribution.play].action.Skill}
			}
			if _, ok := families[kind]; !ok {
				order = append(order, kind)
			}
			families[kind] = append(families[kind], contribution)
		}
		for _, kind := range order {
			members := families[kind]
			leader := members[0].play
			if len(multi) == 1 {
				leader = multiLeader
			}
			group := &battleDamageGroup{target: key.target, generation: key.generation, stat: key.stat}
			var total uint64
			for _, member := range members {
				total += uint64(member.amount)
				group.critical = group.critical || member.critical
			}
			group.parts = motion(leader).DamageParts64(total)
			plays[leader].damage = append(plays[leader].damage, group)
		}
	}
}

func (c *Client) battleDamageHit(group *battleDamageGroup, cue int, now, at time.Time) {
	if cue >= len(group.parts) || group.parts[cue] == 0 {
		return
	}
	f, exists := c.battle.shown[group.target]
	if !exists || group.generation != 0 && f.Generation != group.generation {
		return
	}
	amount := group.parts[cue]
	if group.stat == game.StatCurrentHP {
		f.HP -= uint32(min(uint64(f.HP), amount))
	} else {
		f.SP -= uint16(min(uint64(f.SP), amount))
	}
	c.battle.shown[group.target] = f
	c.battleDamagePopup(group.target, group.generation, group.stat, group.critical, amount, now, at, cue, len(group.parts))
}

// displacement keeps animation tied to the contact timestamp, including late
// frames. Multi-hit lanes alternate and widen as in FUN_0039e884; the arch is a
// time-based approximation of the native FUN_003a2ff4 curve.
func (f battleFloat) displacement(now time.Time) image.Point {
	ticks := int(max(time.Duration(0), now.Sub(f.at)) / battleFloatRiseTick)
	if f.flight == 0 {
		return image.Pt(0, -ticks-f.rise)
	}
	travel := min(battleFloatFlightRangePixels, ticks*battleFloatFlightStepPixels)
	rise := 4 * battleFloatFlightArcPixels * travel * (battleFloatFlightRangePixels - travel) / (battleFloatFlightRangePixels * battleFloatFlightRangePixels)
	return image.Pt(f.flight*travel/2, -rise)
}

func (c *Client) battleDamagePopup(target clientbattle.Cell, generation uint64, stat byte, critical bool, amount uint64, now, at time.Time, hit, hits int) {
	ink, font := uint16(battleDamageInk), battleDamageDigits
	if critical {
		ink = battleCriticalInk
	}
	if stat == game.StatCurrentSP {
		ink, font = battleSPInk, battleSPDigits
	}
	flight := 0
	if hits > 1 {
		// Native hit indices start at one: odd/even hits take opposite sides,
		// with spread (1 + nativeHit/2). Store half-units to retain the
		// native half-step rather than truncating it before multiplication.
		flight = hit + 3
		if hit%2 == 0 {
			flight = -flight
		}
		if target.X < 3 {
			flight = -flight
		}
	}
	// Stack older popups above this one, including simultaneous contacts and
	// cues caught up in one late frame. Cue timestamps retain their true age.
	floor := int(now.Sub(at) / battleFloatRiseTick)
	for i := len(c.battle.floats) - 1; flight == 0 && i >= 0; i-- {
		old := &c.battle.floats[i]
		if !old.damage || old.flight != 0 || old.cell != target || old.generation != generation || now.Sub(old.at) >= battleFloatDuration {
			continue
		}
		gap := battleDamagePopupGap
		if c.Pics != nil {
			gap = max(gap, minigame.NumberHeight(c.Pics, old.font)+battleDamagePopupPadding)
		}
		floor += gap
		rise := int(now.Sub(old.at)/battleFloatRiseTick) + old.rise
		if rise < floor {
			old.rise += floor - rise
			rise = floor
		}
		floor = rise
	}
	c.battle.floats = append(c.battle.floats, battleFloat{cell: target, generation: generation, text: []byte(fmt.Sprint(amount)), ink: ink, at: at, font: font, amount: amount, damage: true, flight: flight})
}
