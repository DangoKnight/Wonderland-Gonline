package inventory

import (
	"image"
	"image/color"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestInventoryVehicleHealthAndFuel(t *testing.T) {
	pics := picdb.New()
	for name, ink := range map[string]color.RGBA{"icon_HP2": {255, 0, 0, 255}, "icon_SP2": {0, 0, 255, 255}, "icon_rail2": {128, 128, 128, 255}} {
		im := image.NewRGBA(image.Rect(0, 0, 40, 5))
		for y := 0; y < 5; y++ {
			for x := 0; x < 40; x++ {
				im.SetRGBA(x, y, ink)
			}
		}
		pics.Add(name, im)
	}
	definition := assets.NativeItem{Definition: game.ItemDefinition{Type: game.VehicleType, CellWidth: 4, CellHeight: 3}}
	definition.Record[45] = 5
	f := &Form{State: &State{Items: map[uint16]assets.NativeItem{48010: definition, 48016: definition}}}
	f.Env = &seui.Env{Pics: pics, Screen: surface.New(200, 150)}
	item := game.Item{ID: 48010, Count: 1}
	item.SetVehicleProgress(7200, 3600)
	before := item
	f.drawItem(item, 10, 20, false)
	pixel := func(x, y int) uint16 { return f.Env.Screen.Pix[y*200+x] }
	if pixel(15, 27) != surface.RGB565(255, 0, 0) || pixel(35, 27) != surface.RGB565(128, 128, 128) || pixel(15, 32) != surface.RGB565(0, 0, 255) || pixel(25, 32) != surface.RGB565(128, 128, 128) {
		t.Fatal("inventory health/fuel overlays or fill amounts")
	}
	if item != before {
		t.Fatal("drawing mutated item condition")
	}
	item.SetVehicleProgress(10800, 0)
	f.Env.Screen.Fill(image.Rect(0, 0, 200, 150), 0)
	f.drawItem(item, 10, 20, false)
	if pixel(25, 27) != surface.RGB565(128, 128, 128) || pixel(15, 32) != 0 {
		t.Fatal("updated wear/fuel not reflected")
	}
	item.ID, item.Damage = 48016, 99
	f.Env.Screen.Fill(image.Rect(0, 0, 200, 150), 0)
	f.drawItem(item, 10, 20, false)
	if pixel(45, 27) != surface.RGB565(255, 0, 0) {
		t.Fatal("Robinson raft must show full health in inventory")
	}
}
