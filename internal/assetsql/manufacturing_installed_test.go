package assetsql

import (
	"os"
	"testing"
	"wonderland-go/internal/assets"
)

func TestInstalledManufacturingAndRebornDefinitions(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Manufacturing) == 0 || len(c.RebornClasses) != 6 {
		t.Fatal("missing manufacturing/classes")
	}
	if err = assets.ValidateManufacturing(c.Manufacturing, c.Items); err != nil {
		t.Fatal(err)
	}
	for _, class := range c.RebornClasses {
		def, ok := c.Items[class.CapeID]
		if !class.Enabled || !ok || def.Type != 16 || def.EquipSlot != 6 {
			t.Fatal("invalid WLRI cape", class, def)
		}
	}
	t.Logf("WLRI formulas: %d; enabled reborn classes: %d", len(c.Manufacturing), len(c.RebornClasses))
}
