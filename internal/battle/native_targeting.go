package battle

import (
	"sort"
	"wonderland-gonline/internal/assets"
)

const (
	nativePatternColumn = 2
	nativePatternRow    = 3
	nativePatternCross  = 4
	nativePatternFive   = 5
	nativePatternSix    = 6
	nativePatternTeam   = 7
	nativePatternAll    = 8

	nativeColumnTargetLimit = 2
	nativeRowTargetLimit    = 3
	nativeCrossTargetLimit  = 4
	nativeFiveTargetLimit   = 5
	nativeSixTargetLimit    = 6
)

// skillTargets ports AoeTargetResolver.ResolveTargets at Private Server
// 0cb6915, including sparse-row expansion and stable distance fallbacks.
// Explicit SQL grade shapes and an explicit area_attack=false take precedence.
func skillTargets(side []*Fighter, selected *Fighter, skill assets.Skill, grade int, includeDead bool) []*Fighter {
	if shape, ok := skill.TargetingAt(grade); ok {
		return formationTargets(side, selected, shape, includeDead)
	}
	eligible := formationTargets(side, selected, assets.SkillTargeting{All: true}, includeDead)
	if selected == nil || len(eligible) == 0 {
		return nil
	}
	var out []*Fighter
	add := func(f *Fighter, limit int) {
		if f == nil || len(out) >= limit {
			return
		}
		for _, old := range out {
			if old == f {
				return
			}
		}
		for _, valid := range eligible {
			if f == valid {
				out = append(out, f)
				return
			}
		}
	}
	add(selected, assets.MaxBattleSideTargets)
	pattern := skill.NativePatternAt(grade)
	if skill.AreaAttack != nil && !*skill.AreaAttack || pattern < nativePatternColumn || pattern > nativePatternAll {
		return out
	}
	px, py := int(selected.X), int(selected.Y)
	opp := px - 1
	if px == 1 || px == 3 {
		opp = px + 1
	}
	cell := func(x, y, limit int) {
		for _, f := range eligible {
			if int(f.X) == x && int(f.Y) == y {
				add(f, limit)
				return
			}
		}
	}
	fill := func(limit int) {
		remaining := append([]*Fighter(nil), eligible...)
		distance := func(f *Fighter) int {
			x, y := int(f.X)-px, int(f.Y)-py
			if x < 0 {
				x = -x
			}
			if y < 0 {
				y = -y
			}
			return x + y
		}
		sort.SliceStable(remaining, func(i, j int) bool { return distance(remaining[i]) < distance(remaining[j]) })
		for _, f := range remaining {
			add(f, limit)
		}
	}
	switch pattern {
	case nativePatternColumn:
		cell(opp, py, nativeColumnTargetLimit)
	case nativePatternRow:
		for _, dy := range []int{-1, 1, -2, 2} {
			cell(px, py+dy, nativeRowTargetLimit)
		}
	case nativePatternCross, nativePatternFive:
		limit := nativeCrossTargetLimit
		if pattern == nativePatternFive {
			limit = nativeFiveTargetLimit
		}
		cell(opp, py, limit)
		cell(px, py-1, limit)
		cell(px, py+1, limit)
		cell(opp, py-1, limit)
		cell(opp, py+1, limit)
		if pattern == nativePatternFive {
			fill(limit)
		}
	case nativePatternSix:
		for _, dy := range []int{0, -1, 1, -2, 2} {
			cell(px, py+dy, nativeSixTargetLimit)
			cell(opp, py+dy, nativeSixTargetLimit)
		}
		fill(nativeSixTargetLimit)
	case nativePatternTeam, nativePatternAll:
		for _, f := range eligible {
			add(f, assets.MaxBattleSideTargets)
		}
	}
	return out
}
