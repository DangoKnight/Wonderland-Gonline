package game

import (
	"errors"
	"testing"
)

func alchemyTestItem(id uint16, rank int, bases ...uint16) AlchemyItem {
	d := AlchemyItem{ID: id, Rank: rank, Eligible: true}
	copy(d.Bases[:], bases)
	return d
}
func highAlchemyRoll(n int) (int, error) { return n - 1, nil }
func rollsAlchemy(values ...int) AlchemyRoll {
	i := 0
	return func(n int) (int, error) {
		if i >= len(values) {
			return n - 1, nil
		}
		v := values[i]
		i++
		return v, nil
	}
}
func TestCompoundingRanksBasesAndBooks(t *testing.T) {
	items := map[uint16]AlchemyItem{
		1: alchemyTestItem(1, 3, 9), // Paper, first ingredient.
		2: alchemyTestItem(2, 2, 4), 3: alchemyTestItem(3, 2, 4),
		10: alchemyTestItem(10, 6, 4, 9), 11: alchemyTestItem(11, 10, 4, 9),
		12: alchemyTestItem(12, 6, 9, 4), 13: alchemyTestItem(13, 7, 4, 9),
		AlchemyBookOne:  alchemyTestItem(AlchemyBookOne, 1, 2),
		AlchemyBookFour: alchemyTestItem(AlchemyBookFour, 4, 2),
	}
	c := NewAlchemyCatalog(items)
	for _, tc := range []struct {
		tier    AlchemyTier
		ids     []uint16
		primary uint16
		delta   int
		result  uint16
	}{
		{AlchemyPrimary, []uint16{1, 2, 3}, 9, 4, 12},
		{AlchemyJunior, []uint16{1, 2, 3}, 4, 4, 10},
		{AlchemySuperior, []uint16{1, 2, 3}, 4, 5, 13},
		{AlchemyJunior, []uint16{1, 2, 3, AlchemyBookOne}, 4, 5, 13},
		{AlchemyJunior, []uint16{1, 2, 3, AlchemyBookOne, AlchemyBookFour}, 4, 8, 11},
	} {
		out, err := c.Compound(tc.ids, tc.tier, 10, highAlchemyRoll)
		if err != nil || out.BaseRank != 2 || out.PrimaryBase != tc.primary || out.Delta != tc.delta || out.ItemID != tc.result {
			t.Fatalf("%+v => %+v, %v", tc, out, err)
		}
	}
	// Book ranks and bases must not participate in either minimum or rank sums.
	out, err := c.Compound([]uint16{1, 2, 3, AlchemyBookOne, AlchemyBookFour}, AlchemyJunior, 10, highAlchemyRoll)
	if err != nil || out.BookBonus != 4 || out.RequiredBases[2] != 0 {
		t.Fatal(out, err)
	}
	if _, err := c.Compound([]uint16{1, AlchemyBookFour}, AlchemySuperior, 10, highAlchemyRoll); !errors.Is(err, ErrAlchemyIngredients) {
		t.Fatal("book treated as a material", err)
	}
}
func TestCompoundingSecondaryLossAndFallback(t *testing.T) {
	c := NewAlchemyCatalog(map[uint16]AlchemyItem{
		1: alchemyTestItem(1, 20, 1, 67), 2: alchemyTestItem(2, 20, 1),
		3: alchemyTestItem(3, 2, 1, 67), 4: alchemyTestItem(4, 21, 1),
	})
	// Primary/Junior can drop all secondary bases, Superior retains them.
	for _, tier := range []AlchemyTier{AlchemyPrimary, AlchemyJunior} {
		out, err := c.Compound([]uint16{1, 2}, tier, 10, rollsAlchemy(9999, 0))
		if err != nil || !out.SecondaryDropped || out.ItemID != 4 {
			t.Fatal(tier, out, err)
		}
	}
	out, err := c.Compound([]uint16{1, 2}, AlchemySuperior, 10, highAlchemyRoll)
	if err != nil || out.SecondaryDropped || out.ItemID != 1 || out.RankCeiling != 25 {
		t.Fatal("Superior lost secondary", out, err)
	}
	// A ceiling below every rank-20 candidate must descend all the way to rank 2.
	out, err = c.Compound([]uint16{1, 2}, AlchemySuperior, 10, rollsAlchemy(9999, 0))
	if err != nil || out.Delta != -4 || out.ItemID != 3 || out.ResultRank != 2 {
		t.Fatal("unbounded rank fallback", out, err)
	}
}
func TestCompoundingCatastrophePreservesBase(t *testing.T) {
	items := map[uint16]AlchemyItem{1: alchemyTestItem(1, 20, 34), 2: alchemyTestItem(2, 10, 34),
		AlchemyCommonStone: alchemyTestItem(AlchemyCommonStone, 1, 34), AlchemySteamedBuns: alchemyTestItem(AlchemySteamedBuns, 1, 6)}
	c := NewAlchemyCatalog(items)
	out, err := c.Compound([]uint16{1, 2}, AlchemyPrimary, 0, rollsAlchemy(0))
	if err != nil || !out.Catastrophic || out.ItemID != AlchemyCommonStone {
		t.Fatal("matching junk preference", out, err)
	}
	items[1] = alchemyTestItem(1, 20, 1)
	items[2] = alchemyTestItem(2, 10, 1)
	c = NewAlchemyCatalog(items)
	out, err = c.Compound([]uint16{1, 2}, AlchemyPrimary, 0, rollsAlchemy(0))
	if err != nil || !out.Catastrophic || out.ItemID != AlchemySteamedBuns {
		t.Fatal("fallback junk pool", out, err)
	}
}
func TestCompoundingCurveOrderingAndValidation(t *testing.T) {
	mean := func(tier AlchemyTier, level byte) float64 {
		sum, total := 0, 0
		for i, w := range AlchemyDeltaWeights(tier, level) {
			sum += (i + AlchemyMinimumDelta) * w
			total += w
		}
		return float64(sum) / float64(total)
	}
	for level := byte(0); level <= AlchemySkillMaximum; level++ {
		p, pd := AlchemyChances(AlchemyPrimary, level)
		j, jd := AlchemyChances(AlchemyJunior, level)
		s, sd := AlchemyChances(AlchemySuperior, level)
		if !(p > j && j > s && pd > jd && jd > sd) {
			t.Fatal("tier chance ordering", level)
		}
		if !(mean(AlchemyPrimary, level) < mean(AlchemyJunior, level) && mean(AlchemyJunior, level) < mean(AlchemySuperior, level)) {
			t.Fatal("tier rank ordering", level)
		}
		if level > 0 && mean(AlchemyPrimary, level) <= mean(AlchemyPrimary, level-1) {
			t.Fatal("level rank ordering")
		}
	}
	character := Character{Skills: []LearnedSkill{{ID: AlchemyPrimarySkill, Grade: 10}, {ID: AlchemyJuniorSkill, Grade: 8}, {ID: AlchemySuperiorSkill, Grade: 2}}}
	tier, level, id := character.AlchemySkill()
	if tier != AlchemySuperior || level != 2 || id != AlchemySuperiorSkill {
		t.Fatal("highest tier selection")
	}
	c := NewAlchemyCatalog(map[uint16]AlchemyItem{1: alchemyTestItem(1, 1, 4), 2: alchemyTestItem(2, 1, 4)})
	for _, roll := range []AlchemyRoll{func(n int) (int, error) { return n, nil }, func(int) (int, error) { return -1, nil }, func(int) (int, error) { return 0, errors.New("entropy failure") }} {
		if _, err := c.Compound([]uint16{1, 2}, AlchemyPrimary, 0, roll); err == nil {
			t.Fatal("invalid random source accepted")
		}
	}
	if _, err := c.Compound([]uint16{1, 99}, AlchemyPrimary, 0, highAlchemyRoll); !errors.Is(err, ErrAlchemyMaterial) {
		t.Fatal("unknown material accepted", err)
	}
}
func TestCompoundSlotsRollbackAndFootprints(t *testing.T) {
	bag := Inventory{{ID: 1, Count: 2}, {ID: 2, Count: 2}, {ID: 3, Count: 2}}
	before := bag
	if _, err := bag.CompoundSlots([]byte{1, 2, 1}, 4); err == nil || bag != before {
		t.Fatal("duplicate debit")
	}
	defs := map[uint16]ItemDefinition{1: {ID: 1}, 2: {ID: 2}, 3: {ID: 3}, 4: {ID: 4, CellWidth: 2, CellHeight: 2}}
	slot, err := bag.CompoundSlots([]byte{1, 2, 3}, 4, defs)
	if err != nil || slot != 4 || bag[0].Count != 1 || bag[1].Count != 1 || bag[2].Count != 1 || bag[3].ID != 4 {
		t.Fatal("multi-input rectangular delivery", slot, err, bag)
	}
}

func TestAlchemyProgressNativeThirtyLevelCap(t *testing.T) {
	c := Character{Skills: []LearnedSkill{{ID: AlchemyPrimarySkill, Grade: 10, EXP: AlchemyGradeEXP(10) - 1}}}
	c.AddSkillEXP(AlchemyPrimarySkill, 1)
	if c.Skills[0].Grade != 11 || c.Skills[0].EXP != 0 {
		t.Fatal("alchemy stopped at combat cap", c.Skills)
	}
	c.Skills[0].Grade, c.Skills[0].EXP = 29, AlchemyGradeEXP(29)-1
	c.AddSkillEXP(AlchemyPrimarySkill, 1)
	if c.Skills[0].Grade != 30 || len(c.AddSkillEXP(AlchemyPrimarySkill, 1)) != 0 {
		t.Fatal("native alchemy cap", c.Skills)
	}
	if len(c.SetSkillGrade(AlchemySuperiorSkill, 30)) == 0 || len(c.SetSkillGrade(AlchemySuperiorSkill, 31)) != 0 {
		t.Fatal("alchemy grade assignment")
	}
}

func TestCompoundingUnavailableResultTriggersCatastrophe(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second AlchemyItem
	}{
		{"below every matching rank", alchemyTestItem(1, 10, 34), alchemyTestItem(2, 10, 34)},
		{"no matching bases", alchemyTestItem(1, 10, 34), alchemyTestItem(2, 10, 6)},
		{"more bases than any item", alchemyTestItem(1, 10, 34, 2, 3, 4, 5), alchemyTestItem(2, 10, 6)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewAlchemyCatalog(map[uint16]AlchemyItem{
				1: tc.first, 2: tc.second,
				AlchemyCommonStone: alchemyTestItem(AlchemyCommonStone, 9, 34),
				AlchemySteamedBuns: alchemyTestItem(AlchemySteamedBuns, 1, 6),
			})
			// Avoid the initial catastrophe roll, retain all bases, and roll -4.
			out, err := c.Compound([]uint16{1, 2}, AlchemySuperior, 0, rollsAlchemy(9999, 0))
			if err != nil || !out.Catastrophic || out.ItemID != AlchemyCommonStone {
				t.Fatal("unavailable result did not produce matching junk", out, err)
			}
		})
	}
}

func TestCompoundingBooksShiftEveryDelta(t *testing.T) {
	items := map[uint16]AlchemyItem{
		1: alchemyTestItem(1, 20, 34), 2: alchemyTestItem(2, 20, 34),
	}
	books := []uint16{AlchemyBookOne, AlchemyBookTwo, AlchemyBookThree, AlchemyBookFour}
	for _, id := range books {
		items[id] = alchemyTestItem(id, AlchemyBookBonus(id), 2)
	}
	for rank := 16; rank <= 29; rank++ {
		id := uint16(100 + rank)
		items[id] = alchemyTestItem(id, rank, 34)
	}
	c := NewAlchemyCatalog(items)
	for tier := AlchemyPrimary; tier <= AlchemySuperior; tier++ {
		for _, level := range []byte{0, AlchemySkillMaximum} {
			position := 0
			for index, weight := range AlchemyDeltaWeights(tier, level) {
				baseDelta := index + AlchemyMinimumDelta
				for _, book := range append([]uint16{0}, books...) {
					ids := []uint16{1, 2}
					if book != 0 {
						ids = append(ids, book)
					}
					// The same weighted-roll position must yield the same base
					// delta, shifted by the book for negative and positive rolls.
					out, err := c.Compound(ids, tier, level, rollsAlchemy(9999, position, 0))
					wantDelta := baseDelta + AlchemyBookBonus(book)
					if err != nil || out.Catastrophic || out.Delta != wantDelta || out.RankCeiling != 20+wantDelta || out.ResultRank != 20+wantDelta {
						t.Fatalf("tier=%d level=%d base delta=%d book=%d: %+v, %v", tier, level, baseDelta, book, out, err)
					}
				}
				position += weight
			}
		}
	}
}
