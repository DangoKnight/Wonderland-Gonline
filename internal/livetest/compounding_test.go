package livetest

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func TestCompoundingScenarioStateAndPersistence(t *testing.T) {
	a := catalogFixture()
	for _, plan := range compoundScenarios() {
		t.Run(plan.ID, func(t *testing.T) {
			c, err := Character(a, plan.ID, 10001, "LiveTester", false)
			if err != nil {
				t.Fatal(err)
			}
			if c.Map != starterBeachMap || c.Bag[0].Count != compoundMaterialCount || c.Bag[1].Count != compoundMaterialCount {
				t.Fatal("wrong location/material prerequisites", c.Map, c.Bag)
			}
			tier, level, skill := c.AlchemySkill()
			projection := a.CompoundingCatalog()
			first, second := projection.Items[c.Bag[0].ID], projection.Items[c.Bag[1].ID]
			switch plan.ID {
			case compoundUnskilled:
				if tier != game.AlchemyPrimary || level != 0 || skill != 0 || len(c.Skills) != 0 {
					t.Fatal("unskilled fixture learned alchemy")
				}
			case compoundJunior, compoundBooks:
				third := projection.Items[c.Bag[2].ID]
				if first.Bases[0] == second.Bases[0] || second.Bases[0] != third.Bases[0] || first.Rank >= second.Rank+third.Rank {
					t.Fatal("fixture cannot distinguish first ingredient from rank sums")
				}
				out, err := projection.Compound([]uint16{first.ID, second.ID, third.ID}, tier, level, func(n int) (int, error) { return n - 1, nil })
				if err != nil || out.Catastrophic || out.SecondaryDropped {
					t.Fatal("rank-sum fixture has no reachable normal result", out, err)
				}
				if level != 1 || plan.ID == compoundJunior && tier != game.AlchemyJunior || plan.ID == compoundBooks && tier != game.AlchemySuperior {
					t.Fatal("tier/level selection prerequisite")
				}
				if plan.ID == compoundBooks && (c.Bag[3].ID != game.AlchemyBookOne || c.Bag[4].ID != game.AlchemyBookFour || c.Bag[3].Count != compoundBookCount) {
					t.Fatal("missing five-input book setup")
				}
			case compoundFallback:
				// Every original delta, including the highest ceiling, must be junk.
				position := 0
				for _, weight := range game.AlchemyDeltaWeights(tier, level) {
					calls := 0
					out, err := projection.Compound([]uint16{first.ID, second.ID}, tier, level, func(n int) (int, error) {
						calls++
						if calls == 1 {
							return n - 1, nil
						}
						if calls == 2 {
							return position, nil
						}
						return 0, nil
					})
					if err != nil || !out.Catastrophic {
						t.Fatal("fallback fixture permits normal result", out, err)
					}
					position += weight
				}
			case compoundFullBag:
				if err := c.Bag.Add(game.Item{ID: game.AlchemyCommonStone}, 1, 1, a.Items); err == nil {
					t.Fatal("full bag has room")
				}
				if tier != game.AlchemyJunior || level != 1 {
					t.Fatal("full-bag skill prerequisite")
				}
			case compoundAdvancement:
				if tier != game.AlchemyPrimary || level != 10 || c.Skills[0].EXP != game.AlchemyGradeEXP(10)-1 {
					t.Fatal("advancement fixture not one attempt before 11")
				}
			}
			cfg := config.Default()
			cfg.AssetsDatabase = filepath.Join(t.TempDir(), "assets.db")
			output := filepath.Join(t.TempDir(), "run")
			report, err := Prepare(context.Background(), cfg, a, plan.ID, output)
			if err != nil {
				t.Fatal(err)
			}
			text, err := os.ReadFile(report)
			if err != nil || !strings.Contains(string(text), "Prepared compounding inventory") || !strings.Contains(string(text), "compounding result") || !strings.Contains(string(text), "Status: NOT RUN") {
				t.Fatal("missing actionable acceptance evidence", err)
			}
			db, err := store.Open(filepath.Join(output, "wonderland.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			accounts, err := db.Accounts(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			tester := false
			for _, account := range accounts {
				chars, err := db.Characters(context.Background(), account.ID)
				if err != nil || len(chars) != 1 {
					t.Fatal("missing persisted fixture", err)
				}
				if chars[0].Name == "LiveTester" {
					tester = true
					if chars[0].Bag != c.Bag || len(chars[0].Skills) != len(c.Skills) {
						t.Fatal("fixture not persisted")
					}
					for i, sk := range c.Skills {
						if chars[0].Skills[i] != sk {
							t.Fatal("skill prerequisite not persisted")
						}
					}
				} else if chars[0].Bag != (game.Inventory{}) {
					t.Fatal("observer received ingredients")
				}
			}
			if !tester {
				t.Fatal("no tester")
			}
		})
	}
}

func TestCompoundingMissingPrerequisitesLeaveNoRun(t *testing.T) {
	for _, scenario := range []string{compoundUnskilled, compoundBooks, compoundFallback, compoundAdvancement} {
		t.Run(scenario, func(t *testing.T) {
			a := catalogFixture()
			switch scenario {
			case compoundUnskilled:
				delete(a.NativeItems, game.AlchemyCommonStone)
				delete(a.NativeItems, game.AlchemySteamedBuns)
			case compoundBooks:
				delete(a.NativeItems, game.AlchemyBookFour)
			case compoundFallback:
				delete(a.NativeItems, 102)
			case compoundAdvancement:
				delete(a.Skills, game.AlchemyPrimarySkill)
			}
			output := filepath.Join(t.TempDir(), "run")
			cfg := config.Default()
			if _, err := Prepare(context.Background(), cfg, a, scenario, output); err == nil {
				t.Fatal("accepted missing prerequisites")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("left partial run")
			}
		})
	}
}

func TestCompoundingFixturesExcludeLuckyBags(t *testing.T) {
	a := catalogFixture()
	for _, id := range []uint16{27012, 27013} {
		it := a.NativeItems[100]
		it.Definition = game.ItemDefinition{ID: id, Type: 20, Name: "Lucky bag"}
		it.Record[45], it.Record[136], it.Record[123] = 5, 72, 19
		a.Items[id], a.NativeItems[id] = it.Definition, it
	}
	for _, plan := range compoundScenarios() {
		c, err := Character(a, plan.ID, 10001, "LiveTester", false)
		if err != nil {
			t.Fatal(err)
		}
		for _, it := range c.Bag {
			if it.ID == 27012 || it.ID == 27013 {
				t.Fatal("fixture selected native lucky bag", plan.ID)
			}
		}
	}
}
