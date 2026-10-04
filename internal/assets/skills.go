package assets

import (
	"fmt"
	"math"
	"strings"
)

type Skill struct {
	Targeting          []SkillTargeting `json:"targeting,omitempty"`
	EffectRefs         []string         `json:"effect_refs,omitempty"`
	NativePattern109   byte             `json:"unknown_u8_offset_109"`
	NativePattern110   byte             `json:"unknown_u8_offset_110"`
	NativePattern111   byte             `json:"unknown_u8_offset_111"`
	NativePattern112   byte             `json:"unknown_u8_offset_112"`
	AreaAttack         *bool            `json:"area_attack,omitempty"`
	NativeRounds51     byte             `json:"unknown_u8_offset_51"`
	NativeEffectCode52 byte             `json:"unknown_u8_offset_52"`
	Effects            []SkillEffect    `json:"effects,omitempty"`
	ID                 uint16           `json:"id"`
	Name               string           `json:"name"`
	Type               byte             `json:"type"`
	SP                 uint16           `json:"sp"`
	Element            byte             `json:"element"`
	Attack             uint16           `json:"attack_category"`
	EffectLayer        byte             `json:"effect_layer"`
	PowerPerLevel      float64          `json:"power_per_level"`
	StatMultiplier     float64          `json:"stat_multiplier"`
	AdditionalDamage   uint16           `json:"additional_damage"`
	TableOrder         uint16           `json:"table_order"`
}

// ParseSkills uses SkillManager.LoadSkillDatabase's native 148-byte layout.
func ParseSkills(data []byte) (map[uint16]Skill, error) {
	if len(data)%148 != 0 {
		return nil, fmt.Errorf("Skill.dat: invalid record length")
	}
	out := map[uint16]Skill{}
	for off := 0; off < len(data); off += 148 {
		b := data[off : off+148]
		name := make([]byte, 20)
		for i := range name {
			name[i] = b[20-i]
		}
		str := strings.Trim(string(name), "\x00 ")
		word := func(i int) uint16 { return (le.Uint16(b[i:]) ^ 0x6ea0) - 4 }
		id := word(22)
		if id == 0 || str == "" {
			continue
		}
		v := Skill{ID: id, Name: str, Type: (b[21] ^ 0xfd) - 4, SP: word(24), Element: (b[26] ^ 0xfd) - 4, Attack: word(27), EffectLayer: (b[29] ^ 0xfd) - 4, PowerPerLevel: math.Float64frombits(le.Uint64(b[32:])), StatMultiplier: math.Float64frombits(le.Uint64(b[40:])), AdditionalDamage: word(49), TableOrder: word(97)}
		if math.IsNaN(v.PowerPerLevel) || math.IsInf(v.PowerPerLevel, 0) || math.IsNaN(v.StatMultiplier) || math.IsInf(v.StatMultiplier, 0) {
			return nil, fmt.Errorf("Skill.dat: nonfinite coefficients for %d", id)
		}
		v.NativeRounds51 = (b[51] ^ 0xfd) - 4
		v.NativeEffectCode52 = (b[52] ^ 0xfd) - 4
		v.NativePattern109 = (b[nativeSkillPatternOffset109] ^ 0xfd) - 4
		v.NativePattern110 = (b[nativeSkillPatternOffset110] ^ 0xfd) - 4
		v.NativePattern111 = (b[nativeSkillPatternOffset111] ^ 0xfd) - 4
		v.NativePattern112 = (b[nativeSkillPatternOffset112] ^ 0xfd) - 4
		if err := PopulateSkillEffects(&v); err != nil {
			return nil, fmt.Errorf("Skill.dat %d: %w", id, err)
		}
		out[id] = v
	}
	return out, nil
}

const (
	SkillLayerPhysical            = 1
	SkillLayerMagical             = 2
	nativeSkillPatternOffset109   = 109
	nativeSkillPatternOffset110   = 110
	nativeSkillPatternOffset111   = 111
	nativeSkillPatternOffset112   = 112
	nativeSingleTargetPattern     = 1
	nativeLastAreaPattern         = 8
	compatibilityPatternGradeBand = 3
)

// IsAreaAttack reads native targeting patterns, not the attack_category (range).
// The four patterns use compatibility grade bands 1–3, 4–6, 7–9 and 10.
// Exact native band thresholds and shapes remain to be verified. SQL authors
// may override classification explicitly without changing native provenance.
func (s Skill) IsAreaAttack(grade int) bool {
	if shape, ok := s.TargetingAt(grade); ok {
		return shape.All || len(shape.Offsets) > 1
	}
	if s.AreaAttack != nil {
		return *s.AreaAttack
	}
	patterns := [4]byte{s.NativePattern109, s.NativePattern110, s.NativePattern111, s.NativePattern112}
	index := min((max(1, grade)-1)/compatibilityPatternGradeBand, len(patterns)-1)
	pattern := patterns[index]
	return pattern > nativeSingleTargetPattern && pattern <= nativeLastAreaPattern
}
