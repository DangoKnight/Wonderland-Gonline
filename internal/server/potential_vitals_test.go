package server

import (
	"context"
	"fmt"
	"testing"

	"wonderland-gonline/internal/game"
)

func TestPotentialPillVitalGainPreservesMissingHealthAndSP(t *testing.T) {
	for _, target := range []byte{0, 2} {
		for _, condition := range []string{"full", "partial", "low", "empty"} {
			for _, outcome := range []struct {
				name        string
				item, level uint16
				roll        int
			}{
				{"success", 34350, 4, 0}, {"failure", 34269, 4, 99}, {"protected failure", 34289, 10, 99},
			} {
				t.Run(fmt.Sprintf("target%d/%s/%s", target, condition, outcome.name), func(t *testing.T) {
					s, c, _ := potentialFixture(t, outcome.item, outcome.level, target)
					next := c.character.Clone()
					vitals := func(hp, sp int64) (int64, int64) {
						switch condition {
						case "partial":
							return max(0, hp-23), max(0, sp-17)
						case "empty":
							return 0, 0
						case "low":
							return 1, 1
						}
						return hp, sp
					}
					if target == 0 {
						next.RecalculateVitals(s.Assets.Items)
						hp, sp := vitals(int64(next.MaxHP), int64(next.MaxSP))
						next.HP, next.SP = uint32(hp), uint32(sp)
					} else {
						p := &next.Pets[1]
						p.Normalize(s.Assets.Items, false, s.petTemplate(p.ID))
						hp, sp := vitals(int64(p.MaxHP), int64(p.MaxSP))
						p.HP, p.SP = int32(hp), int32(sp)
					}
					if err := s.commit(context.Background(), c, next); err != nil {
						t.Fatal(err)
					}
					before := c.character.Clone()
					if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, target}, func() (int, error) { return outcome.roll, nil }); err != nil {
						t.Fatal(err)
					}
					saved, err := s.Store.Characters(context.Background(), c.account.ID)
					if err != nil {
						t.Fatal(err)
					}
					check := func(oldHP, oldSP, oldMaxHP, oldMaxSP, hp, sp, maxHP, maxSP int64) {
						wantHP := min(maxHP, max(0, oldHP+maxHP-oldMaxHP))
						wantSP := min(maxSP, max(0, oldSP+maxSP-oldMaxSP))
						if hp != wantHP || sp != wantSP {
							t.Fatalf("vitals %d/%d, want %d/%d", hp, sp, wantHP, wantSP)
						}
						if outcome.name == "success" && (maxHP <= oldMaxHP || maxSP <= oldMaxSP) {
							t.Fatal("fixture must gain both maxima")
						}
						if outcome.name == "failure" && (maxHP >= oldMaxHP || maxSP >= oldMaxSP) {
							t.Fatal("fixture must lose both maxima")
						}
					}
					if target == 0 {
						for _, c := range []game.Character{*c.character, saved[0]} {
							check(int64(before.HP), int64(before.SP), int64(before.MaxHP), int64(before.MaxSP), int64(c.HP), int64(c.SP), int64(c.MaxHP), int64(c.MaxSP))
						}
					} else {
						old := before.Pets[1]
						for _, p := range []game.Pet{c.character.Pets[1], saved[0].Pets[1]} {
							check(int64(old.HP), int64(old.SP), int64(old.MaxHP), int64(old.MaxSP), int64(p.HP), int64(p.SP), int64(p.MaxHP), int64(p.MaxSP))
						}
						if c.character.HP != before.HP || c.character.SP != before.SP {
							t.Fatal("pet pill changed player vitals")
						}
					}
				})
			}
		}
	}
}
