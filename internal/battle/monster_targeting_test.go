package battle

import (
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestMonsterElementalTargetWeights(t *testing.T) {
	for _, tc := range []struct {
		actor, strong, weak byte
	}{
		{game.Earth, game.Water, game.Wind},
		{game.Water, game.Fire, game.Earth},
		{game.Fire, game.Wind, game.Water},
		{game.Wind, game.Earth, game.Fire},
	} {
		actor := &Fighter{Element: tc.actor}
		for _, target := range []struct {
			element byte
			weight  int
		}{{tc.strong, 4}, {tc.actor, 2}, {tc.weak, 1}, {0, 2}} {
			if got := monsterTargetWeight(actor, &Fighter{Element: target.element}); got != target.weight {
				t.Errorf("element %d against %d: weight %d, want %d", tc.actor, target.element, got, target.weight)
			}
		}
	}
	if got := monsterTargetWeight(&Fighter{}, &Fighter{Element: game.Fire}); got != 2 {
		t.Fatalf("untyped actor weight = %d", got)
	}
}

func TestMonsterWeightedTargetTickets(t *testing.T) {
	actor := &Fighter{Element: game.Fire}
	targets := []*Fighter{{Element: game.Wind}, {Element: game.Fire}, {Element: game.Water}}
	counts := map[*Fighter]int{}
	for ticket := 0; ticket < 7; ticket++ {
		r := lowest
		r.Next = func(lo, hi int) int {
			if lo != 0 || hi != 7 {
				t.Fatalf("roll bounds %d..%d", lo, hi)
			}
			return ticket
		}
		counts[r.monsterTarget(actor, targets)]++
	}
	for i, want := range []int{4, 2, 1} {
		if got := counts[targets[i]]; got != want {
			t.Errorf("target %d selected %d times, want %d", i, got, want)
		}
	}
}

func TestMonsterSkillsAndBasicAttacksTargetPetsEqually(t *testing.T) {
	for _, skill := range []uint16{0, 11001} {
		for _, reverse := range []bool{false, true} {
			counts := map[Kind]int{}
			for ticket := 0; ticket < 4; ticket++ {
				b, player := setup(1000)
				player.Element = game.Wind
				pet := &Fighter{Kind: Pet, Side: player.Side, X: 3, Y: player.Y, HP: 100, MaxHP: 100, Element: game.Wind}
				b.Attackers = append(b.Attackers, pet)
				if reverse {
					b.Attackers[0], b.Attackers[1] = b.Attackers[1], b.Attackers[0]
				}
				m := b.Defenders[0]
				m.Element = game.Wind
				m.SP, m.MaxSP = 100, 100
				m.Skills = [3]uint16{skill}
				r := lowest
				r.Next = func(lo, hi int) int {
					if hi == 4 {
						return ticket
					}
					return lo
				}
				if skill != 0 {
					a, ok := r.monsterSkill(b, m)
					if !ok {
						t.Fatal("no offensive skill")
					}
					if a.TX == player.X {
						counts[Player]++
					} else if a.TX == pet.X {
						counts[Pet]++
					} else {
						t.Fatal("unknown target", a)
					}
				} else {
					before := player.HP
					r.monster(b, m, nil)
					if player.HP < before {
						counts[Player]++
					} else if pet.HP < pet.MaxHP {
						counts[Pet]++
					} else {
						t.Fatal("no target damaged")
					}
				}
			}
			if counts[Player] != 2 || counts[Pet] != 2 {
				t.Fatalf("skill %d reverse %v: %v", skill, reverse, counts)
			}
		}
	}
}

func TestMonsterAttacksPreferElementalAdvantage(t *testing.T) {
	for _, mode := range []string{"basic", "skill", "effect"} {
		counts := map[Kind]int{}
		for ticket := 0; ticket < 5; ticket++ {
			b, player := setup(1000)
			player.Element = game.Water
			pet := &Fighter{Kind: Pet, Side: player.Side, X: 3, Y: player.Y, HP: 100, MaxHP: 100, Element: game.Wind}
			b.Attackers = append(b.Attackers, pet)
			m := b.Defenders[0]
			m.Element, m.SP = game.Fire, 100
			r := lowest
			if mode != "basic" {
				m.Skills = [3]uint16{11001}
			}
			if mode == "effect" {
				r.Skills = map[uint16]assets.Skill{11001: {ID: 11001, Effects: []assets.SkillEffect{{Target: assets.EffectEnemy, BlockActions: true}}}}
			}
			r.Next = func(lo, hi int) int {
				if hi == 5 {
					return ticket
				}
				return lo
			}
			if mode == "basic" {
				before := player.HP
				r.monster(b, m, nil)
				if player.HP < before {
					counts[Player]++
				} else if pet.HP < pet.MaxHP {
					counts[Pet]++
				} else {
					t.Fatal("no damage")
				}
			} else {
				a, ok := r.monsterSkill(b, m)
				if !ok {
					t.Fatal("no skill selected")
				}
				if a.TX == player.X {
					counts[Player]++
				} else if a.TX == pet.X {
					counts[Pet]++
				} else {
					t.Fatal("unknown target")
				}
			}
		}
		if counts[Player] != 1 || counts[Pet] != 4 {
			t.Fatalf("%s selection counts: %v", mode, counts)
		}
	}
}

func TestMonsterOffensiveTargetingSkipsUnavailablePets(t *testing.T) {
	for _, state := range []string{"dead", "joining", "withdrawn", "overkilled"} {
		for _, effect := range []bool{false, true} {
			b, player := setup(1000)
			m := b.Defenders[0]
			m.SP = 100
			m.Skills = [3]uint16{11001}
			pet := &Fighter{Kind: Pet, Side: player.Side, X: 3, Y: player.Y, HP: 100, MaxHP: 100, Element: game.Wind}
			switch state {
			case "dead":
				pet.HP = 0
			case "joining":
				pet.Joining = true
			case "withdrawn":
				pet.Withdrawn = true
			case "overkilled":
				pet.Overkilled = true
			}
			b.Attackers = append(b.Attackers, pet)
			r := lowest
			if effect {
				r.Skills = map[uint16]assets.Skill{11001: {ID: 11001, Effects: []assets.SkillEffect{{Target: assets.EffectEnemy, BlockActions: true}}}}
			}
			r.Next = func(lo, hi int) int { return hi - 1 }
			a, ok := r.monsterSkill(b, m)
			if !ok || a.TX != player.X || a.TY != player.Y {
				t.Fatalf("%s effect %v: %v %v", state, effect, a, ok)
			}
		}
	}
}
