package livetest

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/assetsql"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
)

func catalogFixture() *assets.Catalog {
	const rod, skill = 37105, 15993
	cells := make([]byte, 20*20)
	cells[8*20+8] = assets.FishingWaterCell
	return &assets.Catalog{
		Items:  map[uint16]game.ItemDefinition{21004: {ID: 21004, EquipSlot: 2}, 24004: {ID: 24004, EquipSlot: 5}, rod: {ID: rod, Type: 28}, raftItem: {ID: raftItem, CellWidth: 4, CellHeight: 3}},
		Skills: map[uint16]assets.Skill{skill: {}}, NPCs: map[uint16]assets.NPC{robinsonPet: {}},
		Maps:     map[uint16]assets.Map{starterBeachMap: {ID: starterBeachMap}, beachMap: {ID: beachMap, NPCs: []assets.MapNPC{{ClickID: robinsonActor, X: 200, Y: 200}}}},
		Terrains: map[uint16]assets.Terrain{starterBeachMap: {Width: 1600, Height: 1600, GridWidth: 80, GridHeight: 80, Cells: make([]byte, 80*80)}, beachMap: {Width: 400, Height: 400, GridWidth: 20, GridHeight: 20, Cells: cells}},
		Fishing:  assets.FishingRules{Enabled: true, Maps: []uint16{beachMap}, Rods: []assets.FishingRod{{ItemID: rod, MaxGrade: 6}}, Skills: []uint16{skill}, IntervalSeconds: 60, CatchRequirements: []uint32{14, 35}},
	}
}

func TestScenarioPrerequisites(t *testing.T) {
	a := catalogFixture()
	for _, plan := range Scenarios() {
		t.Run(plan.ID, func(t *testing.T) {
			c, err := Character(a, plan.ID, 10001, "LiveTester", false)
			if err != nil {
				t.Fatal(err)
			}
			observer, err := Character(a, plan.ID, 10002, "LiveObserver", true)
			if err != nil {
				t.Fatal(err)
			}
			if observer.Bag != (game.Inventory{}) || len(observer.Quests) != 0 {
				t.Fatal("observer inherited test rewards/progression")
			}
			if len(plan.Steps) == 0 {
				t.Fatal("missing manual acceptance checklist")
			}
			switch plan.ID {
			case "carnie-return", "native-commands":
				if c.Map != starterBeachMap || observer.Map != starterBeachMap || !a.Terrains[c.Map].Walkable(int(c.X), int(c.Y)) {
					t.Fatal("native travel test must start on walkable Starter Beach")
				}
			case "fishing", "fishing-skilled", "fishing-full-bag", "fishing-advancement":
				if !a.Terrains[c.Map].FishingShore(c.X, c.Y) {
					t.Fatal("not at shoreline")
				}
				learned := false
				for _, sk := range c.Skills {
					if sk.ID == a.Fishing.Skills[0] {
						learned = true
					}
				}
				if learned != (plan.ID != "fishing") {
					t.Fatal("incorrect skill prerequisite")
				}
				if plan.ID == "fishing-full-bag" {
					for _, item := range c.Bag {
						if item.Empty() {
							t.Fatal("bag not full")
						}
					}
				}
			case "raft-no-space":
				b := c.Bag
				if err := b.Add(game.Item{ID: raftItem}, 1, 1, a.Items); err == nil {
					t.Fatal("fragmented inventory accepted raft")
				}
			case "inventory-raft":
				cells, err := c.Bag.Occupancy(a.Items)
				if err != nil {
					t.Fatal(err)
				}
				n := 0
				for _, cell := range cells {
					if cell != 0 {
						n++
					}
				}
				if n != 12 || c.Bag[0].ID != raftItem {
					t.Fatal("incorrect raft footprint")
				}
			case "robinson-recovery":
				if c.Quests[robinsonRaftMark].State != game.Completed || c.Quests[robinsonDialogueMark].Step != 1 || len(c.Pets) != 0 {
					t.Fatal("invalid recovery checkpoint")
				}
			}
		})
	}
}

func TestPrepareIsolationCredentialsAndRefuseOverwrite(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.Database = filepath.Join(root, "original.db")
	cfg.AssetsDatabase = filepath.Join(root, "assets.db")
	sentinel := []byte("original gameplay data")
	if err := os.WriteFile(cfg.Database, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "run")
	report, err := Prepare(context.Background(), cfg, catalogFixture(), "robinson-recovery", output)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "Status: NOT RUN") || !strings.Contains(string(text), "packet/log lines") {
		t.Fatal("missing status/evidence instructions")
	}
	for _, path := range []string{report, filepath.Join(output, "config.json")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("insecure artifact", path, err)
		}
	}
	got, err := config.Load(filepath.Join(output, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Database == cfg.Database || got.AssetsDatabase != cfg.AssetsDatabase {
		t.Fatal("configuration isolation failed")
	}
	db, err := store.Open(got.Database)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	accounts, err := db.Accounts(context.Background())
	if err != nil || len(accounts) != 2 {
		t.Fatal("missing tester/observer", err)
	}
	for _, account := range accounts {
		marker := "Username: `" + account.Username + "`"
		at := strings.Index(string(text), marker)
		if at < 0 {
			t.Fatal("credentials missing")
		}
		suffix := string(text)[at:]
		password := strings.Split(strings.Split(suffix, "Password: `")[1], "`")[0]
		if len(account.Username) < 8 || len(account.Username) > 10 || len(password) < 8 || len(password) > 10 {
			t.Fatal("credentials exceed original client UI limits")
		}
		if _, err := db.Authenticate(context.Background(), account.Username, password); err != nil {
			t.Fatal("unusable native credentials", err)
		}
		chars, err := db.Characters(context.Background(), account.ID)
		if err != nil || len(chars) != 1 {
			t.Fatal("fixture not persisted", err)
		}
	}
	if _, err := Prepare(context.Background(), cfg, catalogFixture(), "fishing", output); err == nil {
		t.Fatal("overwrote existing run")
	}
	after, err := os.ReadFile(cfg.Database)
	if err != nil || !reflect.DeepEqual(after, sentinel) {
		t.Fatal("original database changed")
	}
}

func TestPrepareInvalidScenarioAndFailureLeavesNoRun(t *testing.T) {
	root := t.TempDir()
	cfg := config.Default()
	cfg.AssetsDatabase = filepath.Join(root, "assets.db")
	a := catalogFixture()
	a.Fishing.Enabled = false
	output := filepath.Join(root, "run")
	for _, id := range []string{"unknown", "fishing"} {
		if _, err := Prepare(context.Background(), cfg, a, id, output); err == nil {
			t.Fatal("accepted invalid fixture")
		}
		if _, err := os.Stat(output); !os.IsNotExist(err) {
			t.Fatal("left output on failure", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, cfg, catalogFixture(), "robinson-recovery", output); err == nil {
		t.Fatal("ignored canceled preparation")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("left partial database/credentials")
	}
}

func TestInstalledNativeScenarios(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB to a current imported SQL asset database")
	}
	a, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Scenarios() {
		t.Run(s.ID, func(t *testing.T) {
			if _, err := Character(a, s.ID, 10001, "LiveTester", false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCarnieFixtureDoesNotDependOnRobinsonIsland(t *testing.T) {
	a := catalogFixture()
	delete(a.Maps, beachMap)
	delete(a.Terrains, beachMap)
	for _, observer := range []bool{false, true} {
		c, err := Character(a, "carnie-return", 10001, "LiveTester", observer)
		if err != nil || c.Map != starterBeachMap {
			t.Fatal("Carnie fixture selected rescue island", err)
		}
	}
}

func TestFishingAdvancementFixtureAndPersistence(t *testing.T) {
	a := catalogFixture()
	cfg := config.Default()
	cfg.AssetsDatabase = filepath.Join(t.TempDir(), "assets.db")
	output := filepath.Join(t.TempDir(), "advancement")
	report, err := Prepare(context.Background(), cfg, a, "fishing-advancement", output)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(report)
	if err != nil || !strings.Contains(string(text), "grade 1, per-grade EXP 13") || !strings.Contains(string(text), "[14 35]") {
		t.Fatal("missing progress instructions", err)
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
	testers := 0
	for _, account := range accounts {
		chars, err := db.Characters(context.Background(), account.ID)
		if err != nil || len(chars) != 1 {
			t.Fatal("missing character", err)
		}
		for _, skill := range chars[0].Skills {
			if skill.ID == 15993 {
				testers++
				if chars[0].Name != "LiveTester" || skill.Grade != 1 || skill.EXP != 13 {
					t.Fatal("wrong advancement prerequisite", chars[0].Name, skill)
				}
			}
		}
	}
	if testers != 1 {
		t.Fatal("progress leaked to observer or missing tester", testers)
	}
	a.Fishing.CatchRequirements = nil
	if _, err := Character(a, "fishing-advancement", 10001, "LiveTester", false); err == nil {
		t.Fatal("accepted unknown grade threshold")
	}
}
