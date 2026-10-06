package server

import (
	"context"
	"sync"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
)

func TestCompoundingMultiInputBooksAndSQLSkills(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, id := range []uint16{game.AlchemyBookOne, game.AlchemyBookFour} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id}
		it := assets.NativeItem{Definition: s.Assets.Items[id]}
		it.Record[45], it.Record[136], it.Record[406], it.Record[407] = byte(game.AlchemyBookBonus(id)), 2, 1, 1
		s.Assets.NativeItems[id] = it
	}
	next := c.character.Clone()
	next.Bag[10] = game.Item{ID: 100, Count: 1}
	next.Bag[18] = game.Item{ID: game.AlchemyBookOne, Count: 1}
	next.Bag[19] = game.Item{ID: game.AlchemyBookFour, Count: 1}
	next.Skills = []game.LearnedSkill{{ID: game.AlchemySuperiorSkill, Grade: 1, EXP: 13}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 101, 5, 5, 9, 11, 19, 20}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[4].ID != 300 || !c.character.Bag[8].Empty() || !c.character.Bag[10].Empty() || !c.character.Bag[18].Empty() || !c.character.Bag[19].Empty() {
		t.Fatal("multi-input/book debit")
	}
	if c.character.Skills[0].Grade != 2 || c.character.Skills[0].EXP != 0 {
		t.Fatal("alchemy progression", c.character.Skills)
	}
	got := wires[0].packets(t)
	if len(got) < 8 || got[0][2] != 5 || got[4][2] != 20 || got[5][1] != 8 || got[6][1] != 13 {
		t.Fatal("five-input receipts", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || chars[0].Skills[0] != c.character.Skills[0] {
		t.Fatal("SQL result/progression", err)
	}
}
func TestCompoundingFreshSQLInventoryAndOwnership(t *testing.T) {
	s, players, _ := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	before := c.character.Bag
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	changed, err := s.Store.MutateOwnedCharacter(ctx, ref, func(next *game.Character) error { return next.Bag.Remove(5, 1) })
	if err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != changed.Bag || c.character.Bag != before {
		t.Fatal("stale SQL resources used", err)
	}
}
func TestCompoundingConcurrentReplay(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.dispatch(context.Background(), c, []byte{23, 14, 2, 5, 9}) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	successes := 0
	for _, p := range wires[0].packets(t) {
		if len(p) > 1 && p[0] == 23 && p[1] == 13 {
			successes++
		}
	}
	if successes != 1 || c.character.Bag[4].ID != 300 {
		t.Fatal("duplicate delivery", successes)
	}
}
func TestCompoundingCatastropheAndRandomFailureAtomicity(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	id := uint16(game.AlchemyCommonStone)
	s.Assets.Items[id] = game.ItemDefinition{ID: id}
	it := assets.NativeItem{Definition: s.Assets.Items[id]}
	it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 34, 1, 1
	s.Assets.NativeItems[id] = it
	s.alchemyRandom = func(int) (int, error) { return 0, nil }
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[4].ID != id || !c.character.Bag[8].Empty() {
		t.Fatal("catastrophe not delivered/consumed")
	}
	// Failed entropy must leave both SQL and session inventory intact.
	next := c.character.Clone()
	next.Bag = game.Inventory{}
	next.Bag[4] = game.Item{ID: 100, Count: 1}
	next.Bag[8] = game.Item{ID: 200, Count: 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.alchemyRandom = func(n int) (int, error) { return n, nil }
	before := c.character.Bag
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err == nil {
		t.Fatal("invalid random value accepted")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != before || c.character.Bag != before || wires[0].Len() != 0 {
		t.Fatal("random failure spent resources", err)
	}
}

func TestCompoundingUsesFreshSQLSkillTier(t *testing.T) {
	s, players, _ := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	if _, err := s.Store.MutateOwnedCharacter(ctx, ref, func(next *game.Character) error {
		next.Skills = []game.LearnedSkill{{ID: game.AlchemySuperiorSkill, Grade: 4}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, id := c.character.AlchemySkill(); id == game.AlchemySuperiorSkill {
		t.Fatal("fixture was not stale")
	}
	s.worldMu.Lock()
	execution, err := s.executeCompounding(ctx, c, []byte{5, 9}, false)
	s.worldMu.Unlock()
	if err != nil || execution.Outcome.Tier != game.AlchemySuperior || execution.Outcome.Level != 4 || c.character.Skills[0].EXP != 1 {
		t.Fatal("SQL skill tier ignored", execution.Outcome, err)
	}
}
func TestCompoundingNoCandidateConsumesForCatastrophe(t *testing.T) {
	s, players, wires := compoundFixture(t)
	c := players[0]
	ctx := context.Background()
	for id, it := range s.Assets.NativeItems {
		it.Record[45] = 10
		s.Assets.NativeItems[id] = it
	}
	id := uint16(game.AlchemyCommonStone)
	s.Assets.Items[id] = game.ItemDefinition{ID: id}
	it := assets.NativeItem{Definition: s.Assets.Items[id]}
	it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 34, 1, 1
	s.Assets.NativeItems[id] = it
	count := 0
	s.alchemyRandom = func(n int) (int, error) {
		count++
		if count == 1 {
			return n - 1, nil
		}
		return 0, nil
	}
	if err := s.dispatch(ctx, c, []byte{23, 14, 2, 5, 9}); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Bag != c.character.Bag || c.character.Bag[4].ID != id || !c.character.Bag[8].Empty() {
		t.Fatal("unavailable rank failed to consume inputs and persist junk", err)
	}
	success := false
	for _, p := range wires[0].packets(t) {
		if len(p) > 1 && p[0] == 23 && p[1] == 13 {
			success = true
		}
	}
	if !success {
		t.Fatal("catastrophic fallback did not reply with result")
	}
}

// Raw command bytes come from TRe_CompoundForm's Delphi string literals:
// 17 0e (Primary), 17 57 (Junior), 17 65 (Superior).
func TestCompoundingNativeTierSelectionAndLevelUp(t *testing.T) {
	for _, tc := range []struct {
		name  string
		code  byte
		skill uint16
	}{
		{"primary", 14, game.AlchemyPrimarySkill},
		{"junior", 87, game.AlchemyJuniorSkill},
		{"superior", 101, game.AlchemySuperiorSkill},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, players, wires := compoundFixture(t)
			c := players[0]
			ctx := context.Background()
			next := c.character.Clone()
			next.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: 1, EXP: 13}, {ID: game.AlchemyJuniorSkill, Grade: 1, EXP: 13}, {ID: game.AlchemySuperiorSkill, Grade: 1, EXP: 13}}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			wires[0].Reset()
			if err := s.dispatch(ctx, c, []byte{23, tc.code, 2, 5, 9}); err != nil {
				t.Fatal(err)
			}
			for _, sk := range c.character.Skills {
				if sk.ID == tc.skill {
					if sk.Grade != 2 || sk.EXP != 0 {
						t.Fatal("selected skill did not advance", sk)
					}
				} else if sk.Grade != 1 || sk.EXP != 13 {
					t.Fatal("unselected skill advanced", sk)
				}
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			for i, sk := range chars[0].Skills {
				if sk != c.character.Skills[i] {
					t.Fatal("progression not persisted")
				}
			}
			found := false
			for _, p := range wires[0].packets(t) {
				if len(p) == 12 && p[0] == 8 && p[1] == 1 && p[2] == 110 && p[4] == 2 && p[8] == byte(tc.skill) && p[9] == byte(tc.skill>>8) {
					found = true
				}
			}
			if !found {
				t.Fatal("missing native grade reply")
			}
		})
	}
}

func TestCompoundingUnlearnedNativeTierRejectsWithoutDebit(t *testing.T) {
	for _, code := range []byte{87, 101} {
		s, players, _ := compoundFixture(t)
		c := players[0]
		ctx := context.Background()
		next := c.character.Clone()
		next.Skills = nil
		if err := s.commit(ctx, c, next); err != nil {
			t.Fatal(err)
		}
		before := c.character.Bag
		if err := s.dispatch(ctx, c, []byte{23, code, 2, 5, 9}); err != nil {
			t.Fatal(err)
		}
		chars, err := s.Store.Characters(ctx, c.account.ID)
		if err != nil || chars[0].Bag != before || c.character.Bag != before {
			t.Fatal("unlearned tier consumed resources", err)
		}
	}
}
