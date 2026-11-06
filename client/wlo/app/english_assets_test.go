package app

import (
	"path/filepath"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientfs"
)

func TestIGGEnglishItemsPreserveNativeRecordsAndGameplayFields(t *testing.T) {
	c, _, _ := enteredClient(t)
	// Compare against the preserved extraction, not the compiled text projection.
	raw, err := clientfs.ReadFile(filepath.Join(c.Assets.Data, "item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	native, err := assets.ParseItemCatalogJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for id, before := range native {
		after, ok := c.items[id]
		if !ok {
			t.Fatal("translation removed item", id)
		}
		if after.Definition.Name != before.Definition.Name || after.Description != before.Description {
			changed++
		}
		after.Definition.Name, after.Description = before.Definition.Name, before.Description
		if !reflect.DeepEqual(after, before) {
			t.Fatal("English import changed native item or gameplay fields", id)
		}
	}
	// Name and description overrides can refer to the same inventory item.
	if changed < 2500 {
		t.Fatal("English overrides did not reach client item resources", changed)
	}
	// English IGG names win, even when a previous authored correction differs.
	if c.items[10034].Definition.Name != "Pure Iron Knife" {
		t.Fatal("verified English item name missing")
	}
	if c.npcTemplates[12016].Name != "Samurai" || c.npcTemplates[17738].Name != "Half Beast Ghost" {
		t.Fatal("NPC translations or IGG precedence missing")
	}
	if c.sceneNames[10001] != "Deck" {
		t.Fatal("scene name override missing")
	}
	if c.resources.skillCatalog.Definitions[10012].Name != "M. Special Attack" {
		t.Fatal("skill name override missing")
	}
	c.loadTalks()
	if c.talks[10007] != "This bottle has been broken into pieces, so nothing useful left." {
		t.Fatal("dialogue translation missing", c.talks[10007])
	}
	for id, want := range map[uint32]string{57166: "Breillat", 57171: "Priest in church", 57174: "Drawing", 57181: "Fire Element", 57184: "Rey"} {
		if c.talks[id] != want {
			t.Fatal("compiled Noa answer missing", id, c.talks[id])
		}
	}
}
