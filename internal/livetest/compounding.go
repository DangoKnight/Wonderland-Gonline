package livetest

import (
	"fmt"
	"io"
	"slices"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const (
	compoundUnskilled             = "compound-unskilled"
	compoundJunior                = "compound-junior"
	compoundBooks                 = "compound-books"
	compoundFallback              = "compound-fallback"
	compoundFullBag               = "compound-full-bag"
	compoundAdvancement           = "compound-advancement"
	compoundMaterialCount         = 20
	compoundBookCount             = 10
	compoundMinimumFixtureRank    = game.AlchemyRankMinimum - game.AlchemyMinimumDelta
	compoundBulkMaterialFirstType = 31 // Native fish/fruit/ore/stone/clay/wood/textile/plant materials.
	compoundBulkMaterialLastType  = 38
	compoundAdvancementGrade      = 10
)

func compoundScenarios() []Scenario {
	common := []string{
		"Log in as LiveTester. Open Inventory and the native Compound window. Use the named bag slots in the prepared ingredient table below; do not use results as replacement ingredients. Record inventory and alchemy skill levels before the first attempt.",
		"Left-click each specified material in the Compound window bag. If a quantity prompt appears, select one unit. Confirm each ingredient appears in a recipe slot, then press Compound. Each accepted request must be AC23:14 with the complete slot list; the reply must remove one unit per selected slot (AC23:9), deliver one result (AC23:8), and show the result (AC23:13). The character must remain connected and the window must unlock for another attempt.",
	}
	finish := []string{
		"Repeat using the original ingredient slots while supplies remain. Preserve server-diagnostics.jsonl. For every committed 'compounding result' line check the selected tier/level, minimum material rank, primary base, book bonus, delta and ceiling. Negative deltas are valid. Do not treat one random outcome as evidence of a probability error.",
		"Log out and reconnect using the same run config. Consumed quantities, result items and skill progression must persist without duplicates. The observer may check the compounding pose but must receive no ingredient debits or result items; observer animation parity is a separate rendering observation.",
	}
	plan := func(id, description string, checks ...string) Scenario {
		steps := append([]string{}, common...)
		steps = append(steps, checks...)
		steps = append(steps, finish...)
		return Scenario{id, description, steps}
	}
	return []Scenario{
		plan(compoundUnskilled, "Primary at level zero without teaching a skill",
			"Select material slots 1 and 2 only. Logs must show tier 0, skill_level 0, book_bonus 0, and a delta from -4 through +4. Catastrophes must consume materials and deliver one junk item. Reconnect: Primary, Junior and Superior must remain unlearned."),
		plan(compoundJunior, "Junior chooses summed primary ranks and may drop secondaries",
			"Enable Use Junior Alchemy, then select slots 1, 2 and 3. Slot 1 has a different primary base from slots 2/3; their combined rank exceeds slot 1. Logs must show tier 1, skill_level 1, and the primary base of slots 2/3. The checked Junior option selects Junior even though Primary is level 30. Uncheck it to verify Primary is selected.",
			"For noncatastrophic outcomes, secondary_dropped=true requires only the chosen primary base; false requires the two original bases. Match result bases/rank against the read-only assets catalog or the native item tooltip. A rank fallback may lower the result but may not add or silently drop bases."),
		plan(compoundBooks, "Five inputs, highest book and Superior retention",
			"Select Superior Alchemy in the native tier selector. First submit material slots 1, 2 and 3 alone. Next include Book 1 in slot 4. Finally select materials 1, 2, 3 and 6 plus Book 4 in slot 5, for a five-input request. Bonuses are 0, 1 and 4. Each selected book loses one unit. aLogin permits only one book per recipe: never select slots 4 and 5 together. Highest-book behavior with multiple books is covered by automated server tests and can be checked separately in the Go client.",
			"Logs must show tier 2 and skill_level 1, despite Primary and Junior being level 30. Superior must not drop secondaries. For noncatastrophic rows, delta minus book_bonus must stay within -4..+5; rank_ceiling must equal max(1, base_rank + delta). The bonus shifts negative deltas too. Compare the formula per log row, not the outcomes of separate random attempts. Missing candidates must produce junk and still consume every selected book/material."),
		plan(compoundFallback, "Every attempt produces catastrophic junk when normal results cannot exist",
			"Select Superior Alchemy in the native tier selector. Select slots 1 and 2. Preparation verified that no matching normal item exists at any rank through the highest possible Superior ceiling. Every accepted attempt must therefore be catastrophic, including attempts whose initial catastrophe roll did not fire. Both materials must lose one unit; exactly one allowed junk item must arrive. Inputs must never be refunded because no normal result exists.",
			"Check result IDs against the allowed junk pool printed below. If the pool contains items with slot 1's primary base, the result must come from that preferred subset. Repeat at least three times and reconnect to verify consumption and junk are saved."),
		plan(compoundFullBag, "Reject before delivery when a full bag cannot fit a fresh result",
			"The inventory is completely occupied; selected material stacks each contain more than one unit. Select slots 1 and 2 and submit once. Neither cell is freed by the debit, so fresh AC23 delivery cannot fit. Expect a full-bag rejection with no success/result packets, no ingredient loss and no skill EXP. This rejected transaction is distinct from an accepted catastrophic outcome.",
			"Discard the single junk blocker in slot 50, then retry the same material slots. The accepted attempt must consume one unit from each and place exactly one result in the free cell. Do not discard the original materials. Record before/after quantities and reconnect."),
		plan(compoundAdvancement, "Primary advances from level 10 to 11 and retains progress",
			"Primary starts at level 10 one EXP below its Formula.dat advancement threshold. Select slots 1 and 2 and submit once. Any committed outcome, including junk, must advance Primary to level 11 with zero per-grade EXP. Capture the proficiency/grade replies. This verifies alchemy is not capped at the combat skill limit of 10.",
			"Submit a second attempt using the same material slots. Primary stays level 11 and gains one per-grade EXP. Reconnect and confirm grade 11/EXP 1 using the native skill display where available or the isolated database after stopping the server."),
	}
}

func compoundMaterialBases(it game.AlchemyItem) []uint16 {
	bases := []uint16{it.Bases[0]}
	for _, b := range it.Bases[1:] {
		if b != 0 && !slices.Contains(bases, b) {
			bases = append(bases, b)
		}
	}
	return bases
}

func compoundMaterials(a *assets.Catalog) []game.AlchemyItem {
	var items []game.AlchemyItem
	for _, it := range a.CompoundingCatalog().Items {
		if it.Eligible && a.Items[it.ID].Type >= compoundBulkMaterialFirstType && a.Items[it.ID].Type <= compoundBulkMaterialLastType && it.Rank >= compoundMinimumFixtureRank && game.AlchemyBookBonus(it.ID) == 0 && it.Bases[0] != 0 && a.Items[it.ID].StackLimit() >= compoundMaterialCount && len(compoundMaterialBases(it)) == 1 {
			items = append(items, it)
		}
	}
	slices.SortFunc(items, func(a, b game.AlchemyItem) int {
		if a.Rank != b.Rank {
			return a.Rank - b.Rank
		}
		return int(a.ID) - int(b.ID)
	})
	return items
}

func compoundPair(items []game.AlchemyItem) ([]game.AlchemyItem, error) {
	for i, first := range items {
		for _, second := range items[i+1:] {
			if first.Bases[0] == second.Bases[0] {
				return []game.AlchemyItem{first, second}, nil
			}
		}
	}
	return nil, fmt.Errorf("compounding fixture needs two stackable, single-base materials with the same primary base and rank >= %d", compoundMinimumFixtureRank)
}

func compoundJunk(a *assets.Catalog) []game.AlchemyItem {
	c := a.CompoundingCatalog()
	var items []game.AlchemyItem
	for _, id := range []uint16{game.AlchemyCommonStone, game.AlchemyStrawMushroom, game.AlchemyDirtyWater, game.AlchemyCrudeOil, game.AlchemySteamedBuns} {
		if it, ok := c.Items[id]; ok && it.Eligible {
			items = append(items, it)
		}
	}
	return items
}

// Select a pair with two bases absent below every possible normal ceiling.
// This examines definitions only; it does not roll or simulate an interaction.
func compoundMissingPair(a *assets.Catalog, items []game.AlchemyItem) ([]game.AlchemyItem, error) {
	superiorMaximumDelta := len(game.AlchemyDeltaWeights(game.AlchemySuperior, 0)) - 1 + game.AlchemyMinimumDelta
	lowest := map[[2]uint16]int{}
	for _, it := range a.CompoundingCatalog().Items {
		if !it.Eligible || it.Rank < game.AlchemyRankMinimum || game.AlchemyBookBonus(it.ID) != 0 {
			continue
		}
		bases := compoundMaterialBases(it)
		if len(bases) != 2 {
			continue
		}
		key := [2]uint16{bases[0], bases[1]}
		if rank, ok := lowest[key]; !ok || it.Rank < rank {
			lowest[key] = it.Rank
		}
	}
	for i, first := range items {
		for _, second := range items[i+1:] {
			if first.Bases[0] == second.Bases[0] {
				continue
			}
			primary, secondary := first, second
			if second.Rank > first.Rank {
				primary, secondary = second, first
			}
			ceiling := min(first.Rank, second.Rank) + superiorMaximumDelta
			rank, ok := lowest[[2]uint16{primary.Bases[0], secondary.Bases[0]}]
			if !ok || rank > ceiling {
				return []game.AlchemyItem{primary, secondary}, nil
			}
		}
	}
	return nil, fmt.Errorf("no guaranteed missing-result compounding pair in assets.db")
}

// Ensure the rank-sum scenarios can produce a real two-base result as well as
// junk. A purely impossible pair belongs in the dedicated fallback scenario.
func compoundRankSumTrio(a *assets.Catalog, materials []game.AlchemyItem) ([]game.AlchemyItem, error) {
	var results []game.AlchemyItem
	for _, it := range a.CompoundingCatalog().Items {
		if it.Eligible && it.Rank >= compoundMinimumFixtureRank && game.AlchemyBookBonus(it.ID) == 0 && len(compoundMaterialBases(it)) == 2 {
			results = append(results, it)
		}
	}
	slices.SortFunc(results, func(a, b game.AlchemyItem) int {
		if a.Rank != b.Rank {
			return a.Rank - b.Rank
		}
		return int(a.ID) - int(b.ID)
	})
	maximumDelta := len(game.AlchemyDeltaWeights(game.AlchemyJunior, 0)) - 1 + game.AlchemyMinimumDelta
	for _, result := range results {
		bases := compoundMaterialBases(result)
		var primary, secondary []game.AlchemyItem
		for _, it := range materials {
			if it.Rank+maximumDelta < result.Rank {
				continue
			}
			if it.Bases[0] == bases[0] {
				primary = append(primary, it)
			}
			if it.Bases[0] == bases[1] {
				secondary = append(secondary, it)
			}
		}
		for i, first := range primary {
			for _, second := range primary[i+1:] {
				for _, other := range secondary {
					if other.Rank < first.Rank+second.Rank {
						return []game.AlchemyItem{other, first, second}, nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("no stackable rank-sum trio with an attainable two-base normal result in assets.db")
}

func prepareCompounding(a *assets.Catalog, scenario string, c *game.Character) error {
	junk := compoundJunk(a)
	if len(junk) == 0 {
		return fmt.Errorf("no eligible catastrophic compounding rewards in assets.db")
	}
	materials := compoundMaterials(a)
	selected, err := compoundPair(materials)
	if scenario == compoundFallback {
		selected, err = compoundMissingPair(a, materials)
	}
	if err != nil {
		return err
	}
	if scenario == compoundJunior || scenario == compoundBooks {
		selected, err = compoundRankSumTrio(a, materials)
		if err != nil {
			return err
		}
	}
	c.Bag = game.Inventory{}
	// Explicitly exclude automatic creation skills in this isolated fixture.
	c.Skills = nil
	switch scenario {
	case compoundJunior, compoundFullBag:
		c.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: game.AlchemySkillMaximum}, {ID: game.AlchemyJuniorSkill, Grade: beginnerGrade}}
	case compoundBooks:
		c.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: game.AlchemySkillMaximum}, {ID: game.AlchemyJuniorSkill, Grade: game.AlchemySkillMaximum}, {ID: game.AlchemySuperiorSkill, Grade: beginnerGrade}}
	case compoundFallback:
		c.Skills = []game.LearnedSkill{{ID: game.AlchemySuperiorSkill, Grade: beginnerGrade}}
	case compoundAdvancement:
		c.Skills = []game.LearnedSkill{{ID: game.AlchemyPrimarySkill, Grade: compoundAdvancementGrade, EXP: game.AlchemyGradeEXP(compoundAdvancementGrade) - 1}}
	}
	for _, skill := range c.Skills {
		if _, ok := a.Skills[skill.ID]; !ok {
			return fmt.Errorf("alchemy skill %d missing from assets.db", skill.ID)
		}
	}
	for i, it := range selected {
		c.Bag[i] = game.Item{ID: it.ID, Count: compoundMaterialCount}
	}
	if scenario == compoundBooks {
		catalog := a.CompoundingCatalog()
		for i, id := range []uint16{game.AlchemyBookOne, game.AlchemyBookFour} {
			it, ok := catalog.Items[id]
			if !ok || !it.Eligible || a.Items[id].StackLimit() < compoundBookCount {
				return fmt.Errorf("stackable Alchemy Book %d missing or ineligible", id)
			}
			c.Bag[len(selected)+i] = game.Item{ID: id, Count: compoundBookCount}
		}
	}
	if scenario == compoundBooks {
		c.Bag[5] = game.Item{ID: selected[1].ID, Count: compoundMaterialCount}
	}
	if scenario == compoundFullBag {
		for i := len(selected); i < len(c.Bag); i++ {
			c.Bag[i] = game.Item{ID: junk[0].ID, Count: 1}
		}
	}
	return nil
}

func describeCompounding(w io.Writer, a *assets.Catalog, c game.Character) {
	fmt.Fprint(w, "### Prepared compounding inventory\n\n")
	fmt.Fprintln(w, "| Bag slot | Item ID | Name | Units | Rank | Material bases | Book bonus |\n| --- | --- | --- | --- | --- | --- | --- |")
	catalog := a.CompoundingCatalog()
	for i, item := range c.Bag {
		if item.Empty() {
			continue
		}
		it := catalog.Items[item.ID]
		fmt.Fprintf(w, "| %d | %d | %s | %d | %d | %v | %d |\n", i+1, item.ID, a.Items[item.ID].Name, item.Count, it.Rank, compoundMaterialBases(it), game.AlchemyBookBonus(item.ID))
	}
	tier, level, skill := c.AlchemySkill()
	fmt.Fprintf(w, "\nSelected tier: %d; level: %d; skill ID: %d (0 means unlearned). Prepared alchemy skills: %v.\n\n", tier, level, skill, c.Skills)
	fmt.Fprintln(w, "Available catastrophic rewards (ID, name, primary base):")
	for _, it := range compoundJunk(a) {
		fmt.Fprintf(w, "- %d: %s; base %d\n", it.ID, a.Items[it.ID].Name, it.Bases[0])
	}
	fmt.Fprint(w, "\nNative tier selection uses AC23:14 for Primary, AC23:87 for Junior (Use Junior Alchemy checked), and AC23:101 for Superior. Each accepted attempt awards 1 EXP to the selected learned skill; level 1 requires 14 EXP to advance.\n\nFor calculation evidence, keep debug lines whose message is `compounding result`. `delta` already includes the highest book; subtract `book_bonus` to recover the original roll. Initial catastrophes occur before rank selection and may leave delta/ceiling at zero: skip delta assertions for those rows. For a normal result, its rank may be below the ceiling because of candidate fallback.\n")
}
