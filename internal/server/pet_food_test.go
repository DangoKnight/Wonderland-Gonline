package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func setupStatFood(t *testing.T, s *Server, c *Session) {
	t.Helper()
	if s.Assets.NPCs == nil {
		s.Assets.NPCs = map[uint16]assets.NPC{}
	}
	s.Assets.Items[27001] = game.ItemDefinition{ID: 27001, Type: 20, EquipSlot: 8}
	s.Assets.PetFoods = map[uint16]assets.PetFood{27001: {Targets: []uint32{14156}}}
	s.Assets.NPCs[14156] = assets.NPC{ID: 14156, Name: "Xaolan", Type: 4, Element: game.Earth, Stats: [5]uint16{10, 10, 10, 10, 10}}
	s.World = world.New(s.Assets)
	next := c.character.Clone()
	next.Pets = []game.Pet{game.NewPet(14156, "Xaolan", 1, s.petTemplate(14156), s.Assets.Items)}
	next.Pets[0].HP -= 20
	next.Pets[0].SP -= 10
	next.Bag[8] = game.Item{ID: 27001, Count: 50}
	c.pets = newPetRoster()
	c.pets.register(14156)
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
}

func TestPetStatFoodUsesStartupGrowthFormulaForEachPoint(t *testing.T) {
	for _, formula := range []game.PetGrowthFormula{game.PetGrowthBaseStats, game.PetGrowthCombatStats} {
		t.Run(string(formula), func(t *testing.T) {
			s, c, _, _ := mallFixture(t)
			cfg := config.Default()
			cfg.PetGrowthFormula = formula
			s = New(cfg, s.Store, s.Assets, s.Log)
			setupStatFood(t, s, c)
			// Startup policy remains authoritative until restart.
			s.Config.PetGrowthFormula = game.PetGrowthBaseStats
			if formula == game.PetGrowthBaseStats {
				s.Config.PetGrowthFormula = game.PetGrowthCombatStats
			}
			p := c.character.Pets[0]
			template := s.petTemplate(p.ID)
			calls := 0
			_, err := s.usePetStatFoodWithRoll(context.Background(), c, 9, 2, 1, func(n int) (int, error) {
				want := 50 // Known template's five attributes are each 10.
				if formula == game.PetGrowthCombatStats {
					full := p.Combat(s.Assets.Items, template)
					want = int(full.ATK + full.DEF + full.MAT + full.MDF + full.SPD)
				}
				if n != want {
					t.Fatalf("point %d: %s weights %d != %d", calls, formula, n, want)
				}
				p.Base.Agility++
				calls++
				return n - 1, nil
			})
			if err != nil || calls != 2 || c.character.Pets[0].Base != p.Base || c.character.Pets[0].StatFoodUses != 2 || c.character.Bag[8].Count != 48 {
				t.Fatal("food ignored growth mode", err, calls)
			}
		})
	}
}

func TestPetStatFoodCapBatchVitalsAndNativeReplies(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	setupStatFood(t, s, c)
	next := c.character.Clone()
	next.Pets[0].StatFoodUses = 1199
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	before := c.character.Pets[0]
	wire.Reset()
	if handled, err := s.usePetStatFoodWithRoll(context.Background(), c, 9, 50, 1, func(n int) (int, error) {
		if n != 50 {
			t.Fatal(n)
		}
		return 10, nil
	}); err != nil || !handled {
		t.Fatal(handled, err)
	}
	p := c.character.Pets[0]
	if p.StatFoodUses != 1200 || p.Base.Constitution != before.Base.Constitution+1 || c.character.Bag[8].Count != 49 || p.MaxHP-p.HP != 20 || p.MaxSP-p.SP != 10 {
		t.Fatal("wrong food gain or cap", p)
	}
	packets := wire.packets(t)
	if !bytes.Equal(packets[0], []byte{23, 9, 9, 1}) || !contains(packets, []byte{23, 15}) {
		t.Fatal("native food replies", packets)
	}
	state := c.character.Clone()
	if _, err := s.usePetStatFoodWithRoll(context.Background(), c, 9, 1, 1, func(int) (int, error) { t.Fatal("capped pet rolled"); return 0, nil }); err != nil || !reflect.DeepEqual(state, *c.character) {
		t.Fatal("cap bypass", err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Pets[0].StatFoodUses != 1200 || saved[0].Pets[0].Base != p.Base || saved[0].Bag[8].Count != 49 {
		t.Fatal("food result not durable", err)
	}
}

func TestPetStatFoodRejectionsAndRollback(t *testing.T) {
	s, c, wire, path := mallFixture(t)
	setupStatFood(t, s, c)
	before := c.character.Clone()
	wire.Reset()
	for _, target := range []byte{0, 2, 4} {
		if _, err := s.usePetStatFood(context.Background(), c, 9, 1, target); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("invalid target consumed", target, err)
		}
	}
	s.Assets.PetFoods[27001] = assets.PetFood{Targets: []uint32{17914}}
	if _, err := s.usePetStatFood(context.Background(), c, 9, 1, 1); err != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("wrong pet consumed", err)
	}
	s.Assets.PetFoods[27001] = assets.PetFood{Targets: []uint32{14156}}
	wire.Reset()
	failure := errors.New("RNG failed")
	if _, err := s.usePetStatFoodWithRoll(context.Background(), c, 9, 2, 1, func(int) (int, error) { return 0, failure }); !errors.Is(err, failure) || wire.Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("RNG failure committed", err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec("CREATE TRIGGER reject_pet_food BEFORE INSERT ON character_items BEGIN SELECT RAISE(ABORT,'reject food'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.usePetStatFoodWithRoll(context.Background(), c, 9, 1, 1, func(int) (int, error) { return 0, nil }); err == nil || wire.Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed SQL published food gain", err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Pets[0].StatFoodUses != 0 || saved[0].Pets[0].Base != before.Pets[0].Base || saved[0].Bag[8].Count != 50 {
		t.Fatal("rollback lost food or changed pet", err)
	}
}

func TestPetStatFoodLegacyAndTargetedRequests(t *testing.T) {
	for _, request := range [][]byte{{23, 15, 9, 1, 1, 0}, {67, 1, 121, 105}, {23, 96, 9}} {
		s, c, wire, _ := mallFixture(t)
		setupStatFood(t, s, c)
		next := c.character.Clone()
		next.Pets[0].Battle = true
		next.ActivePet = 14156
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		wire.Reset()
		if err := s.dispatch(context.Background(), c, request); err != nil {
			t.Fatal(err)
		}
		if c.character.Pets[0].StatFoodUses != 1 || c.character.Bag[8].Count != 49 {
			t.Fatal("native request not fed", request)
		}
	}
}

func TestPetStatFoodWisdomPreservesSPDeficit(t *testing.T) {
	s, c, _, _ := mallFixture(t)
	setupStatFood(t, s, c)
	before := c.character.Pets[0]
	if _, err := s.usePetStatFoodWithRoll(context.Background(), c, 9, 1, 1, func(int) (int, error) { return 30, nil }); err != nil {
		t.Fatal(err)
	}
	p := c.character.Pets[0]
	if p.Base.Wisdom != before.Base.Wisdom+1 || p.SP-before.SP != p.MaxSP-before.MaxSP || p.MaxSP-p.SP != 10 || p.HP != before.HP {
		t.Fatal("food SP gain incorrect", p)
	}
}
