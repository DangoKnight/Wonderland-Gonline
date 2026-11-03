package assets

import "strings"

// Native Skill.dat layers and status codes, also used by Private Server's
// support classifiers. They describe existing SQL catalog columns, not new data.
const (
	SkillLayerCleanse        = 5
	SkillLayerMana           = 6
	SkillLayerHealing        = 7
	SkillLayerRevival        = 8
	NativeStatusFreeze       = 1
	NativeStatusTreeBind     = 3
	NativeStatusSeal         = 4
	NativeStatusSleep        = 6
	NativeStatusComa         = 7
	NativeStatusParalysis    = 8
	NativeStatusCleanseAll   = 80
	NativeStatusPoison       = 171
	NativeStatusConfusion    = 173
	NativeStatusFreezeDebuff = 177
	NativeStatusSealDebuff   = 178
)

func (s Skill) RestoresSP() bool {
	return s.EffectLayer == SkillLayerMana || strings.Contains(s.Name, "Magic Recovery") || strings.Contains(s.Name, "Water Mana")
}

func (s Skill) CleansesStatus() bool {
	if s.EffectLayer == SkillLayerCleanse {
		return true
	}
	for _, name := range []string{"Wake", "Ice-out", "Detox", "Free From", "Unstun", "Antidote", "Ammonia", "Recover Normal"} {
		if strings.Contains(s.Name, name) {
			return true
		}
	}
	return false
}

// StatusCode prefers native metadata, allowing translated skill names. Names
// retain compatibility with older authored catalogs without those fields.
func (s Skill) StatusCode() byte {
	if s.NativeEffectCode52 != 0 {
		return s.NativeEffectCode52
	}
	switch {
	case strings.Contains(s.Name, "Ice-out"), strings.Contains(s.Name, "Freeze"):
		return NativeStatusFreeze
	case strings.Contains(s.Name, "Wake"), strings.Contains(s.Name, "Ammonia"), strings.Contains(s.Name, "Sleep"):
		return NativeStatusSleep
	case strings.Contains(s.Name, "Detox"), strings.Contains(s.Name, "Poison"):
		return NativeStatusPoison
	case strings.Contains(s.Name, "Coma"), strings.Contains(s.Name, "Paraly"):
		return NativeStatusComa
	case strings.Contains(s.Name, "Unstun"), strings.Contains(s.Name, "Seal"), strings.Contains(s.Name, "Tree Bind"):
		return NativeStatusSeal
	case strings.Contains(s.Name, "Free From mess"), strings.Contains(s.Name, "Confus"), strings.Contains(s.Name, "Mess"):
		return NativeStatusConfusion
	}
	return NativeStatusCleanseAll
}
