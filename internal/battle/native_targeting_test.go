package battle

import (
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
)

func TestAutomaticNativeTargetPatterns(t *testing.T) {
	want := map[byte][][2]byte{
		1: {{2, 2}},
		2: {{2, 2}, {1, 2}},
		3: {{2, 2}, {2, 1}, {2, 3}},
		4: {{2, 2}, {1, 2}, {2, 1}, {2, 3}},
		5: {{2, 2}, {1, 2}, {2, 1}, {2, 3}, {1, 1}},
		6: {{2, 2}, {1, 2}, {2, 1}, {1, 1}, {2, 3}, {1, 3}},
		7: {{2, 2}, {1, 1}, {1, 2}, {1, 3}, {1, 4}, {2, 1}, {2, 3}, {2, 4}},
		8: {{2, 2}, {1, 1}, {1, 2}, {1, 3}, {1, 4}, {2, 1}, {2, 3}, {2, 4}},
	}
	for _, mirrored := range []bool{false, true} {
		var side []*Fighter
		for x := byte(1); x <= 2; x++ {
			for y := byte(1); y <= 4; y++ {
				cellX := x
				if mirrored {
					cellX = 5 - x
				}
				side = append(side, &Fighter{X: cellX, Y: y, HP: 100, MaxHP: 100})
			}
		}
		selectedX := byte(2)
		if mirrored {
			selectedX = 3
		}
		selected := at(side, selectedX, 2, true)
		for pattern, cells := range want {
			got := skillTargets(side, selected, assets.Skill{NativePattern109: pattern}, 1, false)
			var actual, expected [][2]byte
			for _, f := range got {
				actual = append(actual, [2]byte{f.X, f.Y})
			}
			for _, c := range cells {
				if mirrored {
					c[0] = 5 - c[0]
				}
				expected = append(expected, c)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatal("native target order", pattern, mirrored, actual, expected)
			}
		}
	}
}

func TestAutomaticTargetsSparseSlotsAndOverrides(t *testing.T) {
	primary := &Fighter{X: 2, Y: 1, HP: 100}
	far := &Fighter{X: 2, Y: 3, HP: 100}
	other := &Fighter{X: 1, Y: 2, HP: 100}
	dead := &Fighter{X: 1, Y: 1, HP: 0}
	joining := &Fighter{X: 2, Y: 2, HP: 100, Joining: true}
	withdrawn := &Fighter{X: 1, Y: 3, HP: 100, Withdrawn: true}
	side := []*Fighter{primary, far, other, dead, joining, withdrawn}
	if got := skillTargets(side, primary, assets.Skill{NativePattern109: 3}, 1, false); !reflect.DeepEqual(got, []*Fighter{primary, far}) {
		t.Fatal("sparse row expansion", got)
	}
	if got := skillTargets(side, primary, assets.Skill{NativePattern109: 5}, 1, false); !reflect.DeepEqual(got, []*Fighter{primary, other, far}) {
		t.Fatal("sparse distance fallback", got)
	}
	if got := skillTargets(side, dead, assets.Skill{NativePattern109: 2}, 1, true); !reflect.DeepEqual(got, []*Fighter{dead, primary}) {
		t.Fatal("revival dead primary", got)
	}
	no := false
	s := assets.Skill{NativePattern109: 8, AreaAttack: &no}
	if got := skillTargets(side, primary, s, 1, false); len(got) != 1 {
		t.Fatal("disabled automatic area", got)
	}
	s.Targeting = []assets.SkillTargeting{{MinGrade: 1, MaxGrade: 10, Offsets: []assets.FormationOffset{{}, {Y: 2}}}}
	if got := skillTargets(side, primary, s, 1, false); !reflect.DeepEqual(got, []*Fighter{primary, far}) {
		t.Fatal("explicit SQL override", got)
	}
}

func TestAutomaticAreaAttackUsesNativePattern(t *testing.T) {
	b, actor := setup(1000)
	other := *b.Defenders[0]
	other.X = 1
	b.Defenders = append(b.Defenders, &other)
	r := lowest
	s := r.Skills[11001]
	s.NativePattern109 = 2
	r.Skills = map[uint16]assets.Skill{11001: s}
	a := Action{Actor: actor, Player: 7, Kind: "attack", Skill: 11001, TX: 2, TY: 2}
	if r.comboEligible(a) {
		t.Fatal("automatic AoE joined combo")
	}
	steps, _ := r.attack(b, []Action{a}, nil)
	if len(steps) != 1 || actor.SP != 40 || other.HP == 1000 || b.Defenders[0].HP == 1000 || len(steps[0].SkillUses) != 1 {
		t.Fatal("automatic area execution", steps)
	}
	p := steps[0].Packets[2]
	if len(p) != 32 || p[2] != 28 || p[9] != 2 {
		t.Fatal("native area record", p)
	}
}
