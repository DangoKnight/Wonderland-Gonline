package assets

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestAlchemyDecodeAndFirstMatch(t *testing.T) {
	// Independent encoded record layout from Compound2Dat/AlchemyManager.
	b := make([]byte, 65*3)
	for i, row := range [][3]uint16{{100, 200, 300}, {200, 100, 400}, {0, 200, 500}} {
		r := b[i*65:]
		binary.LittleEndian.PutUint16(r[11:], (row[0]+3)^0xFBBC)
		binary.LittleEndian.PutUint16(r[14:], (row[1]+3)^0xFBBC)
		binary.LittleEndian.PutUint16(r, (row[2]+3)^0xFBBC)
	}
	recipes, err := ParseAlchemyRecipes(b)
	if err != nil || len(recipes) != 2 {
		t.Fatal(recipes, err)
	}
	c := &Catalog{AlchemyRecipes: recipes}
	if c.CompoundResult(100, 200) != 300 || c.CompoundResult(200, 100) != 300 {
		t.Fatal("first symmetric match was not preserved")
	}
	if result, ok := c.AlchemyResult(200, 100); !ok || result != 300 {
		t.Fatal("recipe-only precedence", result, ok)
	}
	if result, ok := c.AlchemyResult(100, 100); ok || result != 0 {
		t.Fatal("recipe-only lookup used fallback", result, ok)
	}
	if c.CompoundResult(100, 100) != 100 || c.CompoundResult(500, 100) != 500 {
		t.Fatal("fallback")
	}
	for _, data := range [][]byte{b[:64], b[:66]} {
		if _, err := ParseAlchemyRecipes(data); err == nil {
			t.Fatal("accepted partial record")
		}
	}
	c.AlchemyRecipes = defaultAlchemyRecipes()
	if c.CompoundResult(27001, 27001) != 48001 || c.CompoundResult(27001, 27002) != 48002 {
		t.Fatal("default recipes")
	}
}

func TestNativeAlchemyRecipes(t *testing.T) {
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_DATA for native recipes")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join("../..", dir)
	}
	c, err := Load(dir, filepath.Join("../..", "data/item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.AlchemyRecipes) != 877 {
		t.Fatalf("recipe census: %d", len(c.AlchemyRecipes))
	}
	if c.CompoundResult(37206, 37207) != 20031 {
		t.Fatal("Compound2 file order changed")
	}
	if c.CompoundResult(27001, 27001) != 48001 {
		t.Fatal("default recipe precedence changed")
	}
	for _, id := range []uint16{37206, 37207, 20031} {
		if _, ok := c.Items[id]; !ok {
			t.Fatalf("missing native item %d", id)
		}
	}
}
