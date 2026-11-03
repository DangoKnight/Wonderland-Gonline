package app

import (
	"os"
	"testing"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
)

func TestPetStatusActiveSelectionAndLivePackets(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.InventoryState.Pets[1] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Stats: world.Stats{Level: 3, HP: 100, MaxHP: 140, SP: 20, MaxSP: 40}}
	c.InventoryState.Pets[3] = inventory.UsePet{ID: 14162, Name: []byte("Roca"), Stats: world.Stats{Level: 5, HP: 80, MaxHP: 100, SP: 15, MaxSP: 25}}
	c.Inventory.Select(4) // A local bag selection must not select the HUD pet.
	c.dispatch([]byte{19, 1, 0, 47, 0, 0})
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame()
	info, ok := c.MainStatus.Pet.Read()
	if !ok || info.ID != 12032 || info.Stats.HP != 100 || !c.MainStatus.Pet.Visible {
		t.Fatal("HUD did not select active Robinson", info)
	}
	portrait := c.resources.petPortrait(c.npcTemplates[12032])
	if portrait == nil {
		t.Fatal("Robinson portrait unavailable")
	}
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if want := portrait.Pix[y*24+x]; want != 0 && c.Screen.Pix[(50+y)*800+146+x] != want {
				t.Fatalf("HUD portrait pixel missing at %d,%d", x, y)
			}
		}
	}
	c.dispatch([]byte{8, 2, 4, 2, 0, 25, 1, 90, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{8, 2, 4, 2, 0, 26, 1, 30, 0, 0, 0, 0, 0, 0, 0})
	c.Frame()
	info, _ = c.MainStatus.Pet.Read()
	if info.Stats.HP != 90 || info.Stats.SP != 30 {
		t.Fatal("native pet vitals not reflected", info)
	}
	// A screenshot with both player and pet general stats populated.
	if path := os.Getenv("PET_STATUS_SNAPSHOT"); path != "" {
		c.Stats.Level, c.Stats.HP, c.Stats.MaxHP, c.Stats.SP, c.Stats.MaxSP, c.Stats.Gold = 3, 183, 183, 103, 103, 64
		formula := c.Stats.Formula
		c.Stats.EXP = uint32(formula.TotalExp(3, false) + (formula.LevelExp(4, false)*89+99)/100)
		c.InventoryState.Pets[1].Stats.EXP = uint32(formula.TotalExp(3, false) + (formula.LevelExp(4, false)*98+99)/100)
		c.Frame()
		savePNG(t, path, c)
	}
	c.dispatch([]byte{19, 1, 82, 55, 0, 0})
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame()
	info, _ = c.MainStatus.Pet.Read()
	if info.ID != 14162 || info.Stats.Level != 5 {
		t.Fatal("replacement pet was stale", info)
	}
	c.dispatch([]byte{19, 2})
	c.Frame()
	if _, ok = c.MainStatus.Pet.Read(); ok || c.MainStatus.Pet.Visible {
		t.Fatal("rest left the HUD visible")
	}
	// Native AC15:4 companion snapshots can arrive before AC19 selection.
	c.showCompanion(c.World.Player.ID, 12032, nil)
	c.Frame()
	if info, ok = c.MainStatus.Pet.Read(); !ok || info.ID != 12032 {
		t.Fatal("companion snapshot not displayed")
	}
	c.InventoryState.Pets[1] = inventory.UsePet{}
	c.Frame()
	if c.MainStatus.Pet.Visible {
		t.Fatal("removed roster entry left stale HUD state")
	}
}
