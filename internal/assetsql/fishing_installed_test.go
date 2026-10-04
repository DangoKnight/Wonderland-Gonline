package assetsql

import (
	"os"
	"testing"
)

func TestInstalledFishingDefaultsAndShoreGeometry(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_ASSETS_DB")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB")
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Fishing.Enabled || len(c.Fishing.Rods) != 3 || len(c.Fishing.Rewards) != 27 {
		t.Fatal("WLRI fishing defaults", c.Fishing)
	}
	for _, mapID := range c.Fishing.Maps {
		terrain, ok := c.Terrains[mapID]
		if !ok {
			t.Fatalf("missing shore terrain %d", mapID)
		}
		shore := false
		for x := 0; x < int(terrain.Width) && !shore; x += 20 {
			for y := 0; y < int(terrain.Height); y += 20 {
				if terrain.FishingShore(uint16(x), uint16(y)) {
					shore = true
					break
				}
			}
		}
		if !shore {
			t.Fatalf("no eligible shore on seeded map %d", mapID)
		}
	}
	for _, reward := range c.Fishing.Rewards {
		if reward.ItemID == 26001 || (c.Items[reward.ItemID].EquipSlot >= 1 && c.Items[reward.ItemID].EquipSlot <= 6) {
			t.Fatal("equipment in fishing pool", reward)
		}
	}
}
