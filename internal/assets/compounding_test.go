package assets

import (
	"errors"
	"testing"
	"wonderland-gonline/internal/game"
)

func TestNativeAlchemyItemProjection(t *testing.T) {
	it := NativeItem{Definition: game.ItemDefinition{ID: 23085}}
	it.Record[45], it.Record[136], it.Record[138], it.Record[406], it.Record[407] = 20, 1, 67, 1, 1
	d := it.AlchemyItem()
	if d.Rank != 20 || d.Bases != [5]uint16{1, 67} || !d.Eligible {
		t.Fatal(d)
	}
	it.Record[123] = 4
	if it.AlchemyItem().Eligible {
		t.Fatal("restricted material")
	}
	it.Record[123] = 0
	it.Record[112] = 1
	if it.AlchemyItem().Eligible {
		t.Fatal("restricted item")
	}
	it.Record[112] = 0
	it.Record[406] = 2
	if it.AlchemyItem().Eligible {
		t.Fatal("native multi-cell exclusion")
	}
}

func TestCompoundingExcludesExoticInputsAndOutputs(t *testing.T) {
	c := Catalog{Items: map[uint16]game.ItemDefinition{}, NativeItems: map[uint16]NativeItem{}}
	for _, id := range []uint16{1, 2, 3} {
		def := game.ItemDefinition{ID: id, Type: 23}
		it := NativeItem{Definition: def}
		it.Record[45], it.Record[136], it.Record[406], it.Record[407] = 1, 1, 1, 1
		if id == 3 {
			it.Definition.Type = 10
			it.Record[45] = 5
		}
		c.Items[id], c.NativeItems[id] = it.Definition, it
	}
	catalog := c.CompoundingCatalog()
	roll := func(n int) (int, error) { return n - 1, nil }
	if _, err := catalog.Compound([]uint16{1, 3}, game.AlchemyPrimary, 0, roll); !errors.Is(err, game.ErrAlchemyMaterial) {
		t.Fatal("Exotic ingredient accepted", err)
	}
	out, err := catalog.Compound([]uint16{1, 2}, game.AlchemyPrimary, 0, roll)
	if err != nil || out.ItemID == 3 || out.ResultRank != 1 {
		t.Fatal("Exotic output selected instead of normal rank fallback", out, err)
	}
}
