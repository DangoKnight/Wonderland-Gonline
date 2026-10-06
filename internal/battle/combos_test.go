package battle

import (
	"bytes"
	"math"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func comboFixture(speeds ...int) (*Battle, Rules, []Action) {
	b, _ := setup(10000)
	b.Attackers = nil
	var actions []Action
	for i, speed := range speeds {
		f := PlayerFighter(hero(), nil, i%4)
		f.ID, f.Spd = uint32(i+1), speed+1
		f.Level = 30
		if i >= 4 {
			f.Kind = Pet
			f.Char = nil
			f.Pet = &game.Pet{Skills: []game.PetSkill{{ID: 11001, Grade: 2}}}
			f.X = 3
		}
		b.Attackers = append(b.Attackers, f)
		actions = append(actions, Action{Actor: f, Kind: "attack", Skill: 10001, TX: 2, TY: 2})
	}
	// Keep the enemy below all attackers, including the zero-gap fixture.
	b.Defenders[0].Spd = 0
	return b, lowest, actions
}

func TestComboSpeedChains(t *testing.T) {
	for _, tc := range []struct {
		name   string
		speeds []int
		sizes  []int
	}{
		{"bridge", []int{30, 60, 150}, []int{3, 1}},
		{"without bridge", []int{30, 150}, []int{1, 1, 1}},
		{"reference example", []int{100, 199, 294}, []int{3, 1}},
		{"99 inclusive", []int{0, 99}, []int{2, 1}},
		{"100 excluded", []int{0, 100}, []int{1, 1, 1}},
		{"full party with pets", []int{0, 99, 198, 297, 396, 495, 594, 693}, []int{8, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, r, actions := comboFixture(tc.speeds...)
			steps := b.order(r, actions)
			if len(steps) != len(tc.sizes) {
				t.Fatalf("groups: %v", steps)
			}
			for i, size := range tc.sizes {
				if len(steps[i]) != size {
					t.Fatalf("group %d: %d, want %d", i, len(steps[i]), size)
				}
			}
		})
	}
}

func TestComboBarriers(t *testing.T) {
	for _, name := range []string{"enemy", "support", "other target", "other side", "area", "dead", "blocked", "absent", "SP", "unlearned"} {
		t.Run(name, func(t *testing.T) {
			b, r, actions := comboFixture(150, 60, 30)
			// Copy the map rather than mutating the shared deterministic rules fixture.
			r.Skills = map[uint16]assets.Skill{11001: lowest.Skills[11001]}
			bridge := actions[1].Actor
			switch name {
			case "enemy":
				b.Defenders[0].Spd = 100
			case "support":
				actions[1].Kind = "effect"
			case "other target":
				actions[1].TY = 3
			case "other side":
				bridge.Side = Defender
			case "area":
				area := true
				r.Skills[11001] = assets.Skill{ID: 11001, SP: 10, AreaAttack: &area}
				actions[1].Skill = 11001
			case "dead":
				bridge.HP = 0
			case "blocked":
				bridge.ApplyEffect(50000, 0, blocking(2, false))
			case "absent":
				bridge.Absent = true
			case "SP":
				actions[1].Skill = 11001
				bridge.SP = 0
			case "unlearned":
				actions[1].Skill = 11001
				bridge.Char.Skills = nil
			}
			for _, group := range b.order(r, actions) {
				if len(group) > 1 && !(name == "enemy" && len(group) == 2 && group[0].Actor != actions[0].Actor) {
					t.Fatalf("%s allowed combo: %v", name, group)
				}
			}
		})
	}
}

func TestComboExecutionRechecksBridgeAndRetainsSpeedSnapshot(t *testing.T) {
	for _, name := range []string{"dead", "blocked", "SP", "redirected", "speed changed"} {
		t.Run(name, func(t *testing.T) {
			b, r, actions := comboFixture(150, 60, 30)
			actions[1].Skill = 11001
			group := b.order(r, actions)[0]
			bridge := actions[1].Actor
			switch name {
			case "dead":
				bridge.HP = 0
			case "blocked":
				bridge.ApplyEffect(50000, 0, blocking(2, false))
			case "SP":
				bridge.SP = 0
			case "redirected":
				bridge.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, AllyAttackChancePercent: 100})
			case "speed changed":
				bridge.Spd = 500
			}
			steps, _ := r.attack(b, group, map[int]bool{})
			if name == "speed changed" {
				if len(steps) != 1 || len(steps[0].SkillUses) != 3 {
					t.Fatal("round speed changed mid-round", steps)
				}
			} else {
				for _, step := range steps {
					if len(step.SkillUses) > 1 {
						t.Fatal("missing bridge retained combo", step)
					}
				}
			}
		})
	}
}

func TestFullPartyComboPacketDamageAndSkillUses(t *testing.T) {
	b, r, actions := comboFixture(0, 99, 198, 297, 396, 495, 594, 693)
	for i := range actions {
		actions[i].Skill = 11001
	}
	group := b.order(r, actions)[0]
	steps, _ := r.attack(b, group, map[int]bool{})
	if len(steps) != 1 || len(steps[0].SkillUses) != 8 {
		t.Fatal("party split into pairs", steps)
	}
	var animation []byte
	for _, p := range steps[0].Packets {
		if bytes.HasPrefix(p, []byte{50, 1}) {
			animation = p
		}
	}
	if len(animation) != 154 {
		t.Fatalf("eight native hit records: %d bytes", len(animation))
	}
	total := 0
	for i, a := range group {
		// Native record offsets and the 30 percent bonus are authored
		// independently of the production encoder/grouping constants.
		off := 2 + i*19
		if animation[off] != 17 || animation[off+2] != a.Actor.X || animation[off+3] != a.Actor.Y || animation[off+4] != 249 || animation[off+5] != 42 {
			t.Fatalf("record %d: %v", i, animation[off:off+19])
		}
		damage := int(animation[off+14]) | int(animation[off+15])<<8
		if damage != int(float64(r.baseDamage(a.Actor, b.Defenders[0], 11001, 1))*1.3) {
			t.Fatalf("actor %d damage %d", i, damage)
		}
		total += damage
		if a.Actor.SP != 40 || steps[0].SkillUses[i].Actor != a.Actor {
			t.Fatal("participant cost/proficiency", i)
		}
	}
	if b.Defenders[0].HP != 10000-total {
		t.Fatal("aggregated damage", b.Defenders[0].HP, total)
	}
}

func TestComboUsesPlayerAndPetLearnedAreaGrade(t *testing.T) {
	for _, pet := range []bool{false, true} {
		b, r, actions := comboFixture(150, 60, 30)
		r.Skills = map[uint16]assets.Skill{11001: {ID: 11001, SP: 10, NativePattern109: 1, NativePattern111: 2}}
		bridge := actions[1].Actor
		actions[1].Skill = 11001
		if pet {
			bridge.Char = nil
			bridge.Pet = &game.Pet{Skills: []game.PetSkill{{ID: 11001, Grade: 1}}}
		}
		if len(b.order(r, actions)[0]) != 3 {
			t.Fatal("single-target grade rejected")
		}
		if pet {
			bridge.Pet.Skills[0].Grade = 7
		} else {
			bridge.Char.Skills[0].Grade = 7
		}
		for _, group := range b.order(r, actions) {
			if len(group) > 1 {
				t.Fatal("area grade bridged combo")
			}
		}
	}
}

func TestComboKnockoutDoesNotRetargetRemainingParticipants(t *testing.T) {
	b, r, actions := comboFixture(150, 60, 30)
	b.Defenders[0].HP = 1
	b.Defenders = append(b.Defenders, &Fighter{Side: Defender, Kind: Monster, X: 2, Y: 3, HP: 100, MaxHP: 100})
	steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
	if len(steps) != 2 || len(steps[0].SkillUses) != 3 || !bytes.Equal(steps[1].Packets[0], []byte{53, 3, 2, 2}) {
		t.Fatal("combo knockout packets", steps)
	}
	if b.Defenders[0].HP != 0 || b.Defenders[0].Deaths != 1 || b.Defenders[1].HP != 100 {
		t.Fatal("combo split across enemies")
	}
}

func TestComboAggregateDamageClampsWithoutOverflow(t *testing.T) {
	b, r, actions := comboFixture(150, 60, 30)
	for _, a := range actions {
		a.Actor.ApplyEffect(50000, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectDamageDealt, Flat: math.MaxInt32}}})
	}
	b.Defenders[0].HP = math.MaxInt32
	steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
	if b.Defenders[0].HP != 0 || b.Defenders[0].Deaths != 1 || len(steps[0].SkillUses) != 3 {
		t.Fatal("large combo overflowed", b.Defenders[0].HP, steps)
	}
}

func TestComboAggregatesEffectiveSpeedEffects(t *testing.T) {
	b, r, actions := comboFixture(150, 0, 30)
	bridge := actions[1].Actor
	bridge.ApplyEffect(50000, 0, effect(assets.EffectSPD, 20, 0, 2, ""))
	bridge.ApplyEffect(50001, 0, effect(assets.EffectSPD, 40, 0, 2, ""))
	if bridge.Spd != 1 || bridge.speed() != 61 || len(b.order(r, actions)[0]) != 3 {
		t.Fatal("aggregate speed buffs did not bridge combo")
	}
	bridge.RemoveEffects(50001)
	for _, group := range b.order(r, actions) {
		for _, a := range group {
			if a.Actor == actions[0].Actor && len(group) > 1 {
				t.Fatal("expired bridge kept fastest attacker in combo")
			}
		}
	}
}
