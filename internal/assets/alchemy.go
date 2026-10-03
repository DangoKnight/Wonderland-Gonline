package assets

import "fmt"

const (
	compoundRecordBytes       = 65
	compoundWordMask          = 0xFBBC
	compoundWordBias          = 3
	compoundOutputOffset      = 0
	compoundFirstInputOffset  = 11
	compoundSecondInputOffset = 14
)

// AlchemyRecipe is the two-input projection used by native AlchemyManager and
// AC23.Recv14. Manufacturing quantities, tools and later ingredients are not
// applied by that handler; manufacturing remains a separate pending subsystem.
type AlchemyRecipe struct{ Input1, Input2, Output uint16 }

// ParseAlchemyRecipes follows AlchemyManager.LoadFromCompoundDat. Keep file order
// because multiple outputs may share inputs and FindRecipe chooses the first.
func ParseAlchemyRecipes(data []byte) ([]AlchemyRecipe, error) {
	if len(data)%compoundRecordBytes != 0 {
		return nil, fmt.Errorf("compound data: invalid record length")
	}
	var recipes []AlchemyRecipe
	for off := 0; off < len(data); off += compoundRecordBytes {
		b := data[off : off+compoundRecordBytes]
		word := func(at int) uint16 { return (le.Uint16(b[at:]) ^ compoundWordMask) - compoundWordBias }
		r := AlchemyRecipe{word(compoundFirstInputOffset), word(compoundSecondInputOffset), word(compoundOutputOffset)}
		if r.Input1 != 0 && r.Input2 != 0 && r.Output != 0 {
			recipes = append(recipes, r)
		}
	}
	return recipes, nil
}

// Authored recipe IDs from AlchemyManager.InitializeRecipes. Rates are omitted:
// AC23.Recv14 never rolls for success, despite rates on the reference table.
func defaultAlchemyRecipes() []AlchemyRecipe {
	return []AlchemyRecipe{
		{27001, 27001, 48001}, {27002, 27001, 48002}, {27005, 27001, 27015},
		{27020, 27001, 46005}, {46005, 27001, 21001}, {46005, 46005, 21010},
		{28014, 28006, 30201}, {30201, 28015, 30202}, {30202, 28015, 30204}, {30205, 28012, 30203},
		{30001, 27001, 28020}, {30002, 27001, 28021}, {30003, 27001, 28022},
		{30015, 30016, 22001}, {30016, 30013, 22005}, {30018, 30013, 22010},
	}
}

// CompoundResult is native FindRecipe followed by Recv14's deterministic fallback:
// identical inputs reproduce the ID; different inputs reproduce the greater ID.
func (c *Catalog) CompoundResult(first, second uint16) uint16 {
	if result, ok := c.AlchemyResult(first, second); ok {
		return result
	}
	return max(first, second)
}

// AlchemyResult is FindRecipe's first symmetric match. AC40 requires a recipe;
// only AC23 applies a greater-input-ID fallback when none is available.
func (c *Catalog) AlchemyResult(first, second uint16) (uint16, bool) {
	for _, r := range c.AlchemyRecipes {
		if (r.Input1 == first && r.Input2 == second) || (r.Input1 == second && r.Input2 == first) {
			return r.Output, true
		}
	}
	return 0, false
}
