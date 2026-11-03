package battle

import "wonderland-gonline/internal/assets"

// PvEBattleManager's mana formula; keep it separate from HP healing.
const (
	manaMinimumRestoration = 25
	manaStatMultiplier     = 1.5
	manaLevelRestoration   = 10
)

// Unstun and general recovery retain Private Server's broader clearing rules.
func cleanses(cure, status byte) bool {
	switch cure {
	case assets.NativeStatusFreeze:
		return status == assets.NativeStatusFreeze || status == assets.NativeStatusFreezeDebuff
	case assets.NativeStatusSleep:
		return status == assets.NativeStatusSleep
	case assets.NativeStatusPoison:
		return status == assets.NativeStatusPoison
	case assets.NativeStatusConfusion:
		return status == assets.NativeStatusConfusion
	case assets.NativeStatusComa:
		return status == assets.NativeStatusComa || status == assets.NativeStatusParalysis
	case assets.NativeStatusSeal:
		return status == assets.NativeStatusTreeBind || status == assets.NativeStatusSeal || status == assets.NativeStatusSealDebuff || status == assets.NativeStatusComa || status == assets.NativeStatusParalysis
	default:
		return true // General recovery clears all statuses in the reference.
	}
}

func (r Rules) needsCleanse(f *Fighter, skill assets.Skill) bool {
	for _, effect := range f.Effects {
		if effect.Turns > 0 && cleanses(skill.StatusCode(), r.Skills[effect.Source].StatusCode()) {
			return true
		}
	}
	return false
}

func (r Rules) cleanse(f *Fighter, skill assets.Skill) {
	out := f.Effects[:0]
	for _, effect := range f.Effects {
		if !cleanses(skill.StatusCode(), r.Skills[effect.Source].StatusCode()) {
			out = append(out, effect)
		}
	}
	f.Effects = out
}
