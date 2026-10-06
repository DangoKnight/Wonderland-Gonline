package game

import (
	"errors"
	"fmt"
	"slices"
	"sort"
)

var ErrAlchemyIngredients = errors.New("alchemy needs two to five distinct available ingredients, including at least two materials")
var ErrAlchemyMaterial = errors.New("item cannot be used for alchemy")
var ErrAlchemyResult = errors.New("alchemy result is unavailable in the item catalog")

// AlchemyItem is a typed, read-only projection of the SQL item catalog.
type AlchemyItem struct {
	ID       uint16
	Rank     int
	Bases    [AlchemyBaseLimit]uint16
	Eligible bool
}
type AlchemyCatalog struct {
	Items   map[uint16]AlchemyItem
	results map[[AlchemyBaseLimit]uint16]map[int][]uint16
}
type AlchemyRoll func(maxExclusive int) (int, error)
type AlchemyOutcome struct {
	ItemID                                              uint16
	Tier                                                AlchemyTier
	Level                                               byte
	BaseRank, BookBonus, Delta, RankCeiling, ResultRank int
	PrimaryBase                                         uint16
	RequiredBases                                       [AlchemyBaseLimit]uint16
	Catastrophic, SecondaryDropped                      bool
}

func alchemySignature(bases [AlchemyBaseLimit]uint16) [AlchemyBaseLimit]uint16 {
	// Primary remains ordered; other bases form a set.
	secondary := make([]uint16, 0, AlchemyBaseLimit-1)
	for _, b := range bases[1:] {
		if b != 0 && b != bases[0] && !slices.Contains(secondary, b) {
			secondary = append(secondary, b)
		}
	}
	slices.Sort(secondary)
	out := [AlchemyBaseLimit]uint16{bases[0]}
	copy(out[1:], secondary)
	return out
}
func NewAlchemyCatalog(items map[uint16]AlchemyItem) *AlchemyCatalog {
	c := &AlchemyCatalog{Items: items, results: map[[AlchemyBaseLimit]uint16]map[int][]uint16{}}
	for id, it := range items {
		if !it.Eligible || it.Rank < AlchemyRankMinimum || it.Bases[0] == 0 || AlchemyBookBonus(id) > 0 {
			continue
		}
		sig := alchemySignature(it.Bases)
		if c.results[sig] == nil {
			c.results[sig] = map[int][]uint16{}
		}
		c.results[sig][it.Rank] = append(c.results[sig][it.Rank], id)
	}
	for _, ranks := range c.results {
		for _, ids := range ranks {
			slices.Sort(ids)
		}
	}
	return c
}
func checkedAlchemyRoll(roll AlchemyRoll, n int) (int, error) {
	if roll == nil || n <= 0 {
		return 0, fmt.Errorf("invalid alchemy random source")
	}
	r, err := roll(n)
	if err != nil {
		return 0, err
	}
	if r < 0 || r >= n {
		return 0, fmt.Errorf("alchemy random value %d outside [0,%d)", r, n)
	}
	return r, nil
}
func weightedAlchemyRoll(weights []int, roll AlchemyRoll) (int, error) {
	total := 0
	for _, w := range weights {
		total += w
	}
	r, err := checkedAlchemyRoll(roll, total)
	if err != nil {
		return 0, err
	}
	for i, w := range weights {
		if r < w {
			return i, nil
		}
		r -= w
	}
	return 0, fmt.Errorf("invalid alchemy weights")
}
func (c *AlchemyCatalog) Compound(ids []uint16, tier AlchemyTier, level byte, roll AlchemyRoll) (AlchemyOutcome, error) {
	out := AlchemyOutcome{Tier: tier, Level: min(level, AlchemySkillMaximum)}
	if len(ids) < AlchemyMinimumMaterials || len(ids) > AlchemyMaximumIngredients || tier > AlchemySuperior {
		return out, ErrAlchemyIngredients
	}
	materials := []AlchemyItem{}
	for _, id := range ids {
		it, ok := c.Items[id]
		if !ok || !it.Eligible {
			return out, ErrAlchemyMaterial
		}
		if book := AlchemyBookBonus(id); book > 0 {
			out.BookBonus = max(out.BookBonus, book)
			continue
		}
		if it.Rank < AlchemyRankMinimum || it.Bases[0] == 0 {
			return out, ErrAlchemyMaterial
		}
		materials = append(materials, it)
	}
	if len(materials) < AlchemyMinimumMaterials {
		return out, ErrAlchemyIngredients
	}
	out.BaseRank = materials[0].Rank
	out.PrimaryBase = materials[0].Bases[0]
	primarySums := map[uint16]int{}
	allSums := map[uint16]int{}
	order := []uint16{}
	for _, it := range materials {
		out.BaseRank = min(out.BaseRank, it.Rank)
		primarySums[it.Bases[0]] += it.Rank
		seen := map[uint16]bool{}
		for _, b := range it.Bases {
			if b != 0 && !seen[b] {
				seen[b] = true
				allSums[b] += it.Rank
				if !slices.Contains(order, b) {
					order = append(order, b)
				}
			}
		}
	}
	if tier != AlchemyPrimary {
		for _, it := range materials {
			b := it.Bases[0]
			if primarySums[b] > primarySums[out.PrimaryBase] {
				out.PrimaryBase = b
			}
		}
	}
	cat, drop := AlchemyChances(tier, level)
	r, err := checkedAlchemyRoll(roll, AlchemyChanceScale)
	if err != nil {
		return out, err
	}
	if r < cat {
		return c.catastrophe(out, roll)
	}
	out.RequiredBases[0] = out.PrimaryBase
	if len(allSums) > 1 && drop > 0 {
		r, err = checkedAlchemyRoll(roll, AlchemyChanceScale)
		if err != nil {
			return out, err
		}
		out.SecondaryDropped = r < drop
	}
	if !out.SecondaryDropped {
		// Stronger secondary bases first; ties retain ingredient/base order.
		sort.SliceStable(order, func(i, j int) bool { return allSums[order[i]] > allSums[order[j]] })
		index := 1
		for _, b := range order {
			if b != out.PrimaryBase {
				if index >= AlchemyBaseLimit {
					return c.catastrophe(out, roll)
				}
				out.RequiredBases[index] = b
				index++
			}
		}
	}
	weights := AlchemyDeltaWeights(tier, level)
	delta, err := weightedAlchemyRoll(weights, roll)
	if err != nil {
		return out, err
	}
	out.Delta = delta + AlchemyMinimumDelta + out.BookBonus
	out.RankCeiling = max(AlchemyRankMinimum, out.BaseRank+out.Delta)
	ranks := c.results[alchemySignature(out.RequiredBases)]
	for rank := out.RankCeiling; rank >= AlchemyRankMinimum; rank-- {
		pool := ranks[rank]
		if len(pool) == 0 {
			continue
		}
		pick, err := checkedAlchemyRoll(roll, len(pool))
		if err != nil {
			return out, err
		}
		out.ItemID = pool[pick]
		out.ResultRank = rank
		return out, nil
	}
	return c.catastrophe(out, roll)
}

// Catastrophe is also the fallback when the required bases or rank cannot
// produce an item. It preserves the normal junk pool and primary-base preference.
func (c *AlchemyCatalog) catastrophe(out AlchemyOutcome, roll AlchemyRoll) (AlchemyOutcome, error) {
	out.Catastrophic = true
	pool := []uint16{}
	preferred := []uint16{}
	for _, id := range []uint16{AlchemyCommonStone, AlchemyStrawMushroom, AlchemyDirtyWater, AlchemyCrudeOil, AlchemySteamedBuns} {
		it, ok := c.Items[id]
		if !ok || !it.Eligible {
			continue
		}
		pool = append(pool, id)
		if it.Bases[0] == out.PrimaryBase {
			preferred = append(preferred, id)
		}
	}
	if len(preferred) > 0 {
		pool = preferred
	}
	if len(pool) == 0 {
		return out, ErrAlchemyResult
	}
	pick, err := checkedAlchemyRoll(roll, len(pool))
	if err != nil {
		return out, err
	}
	out.ItemID = pool[pick]
	out.ResultRank = c.Items[out.ItemID].Rank
	return out, nil
}
