package inventory

import (
	"image"
	"wonderland-gonline/internal/game"
)

// FUN_0035730c overlays native vehicle health/fuel at item Y+5 and Y+10.
const (
	vehicleHealthTop = 5
	vehicleFuelTop   = 10
)

func (f *Form) drawVehicleCondition(item game.Item, x, y int) {
	definition := f.State.Items[item.ID]
	if definition.Definition.Type != game.VehicleType {
		return
	}
	condition := definition.VehicleCondition(item)
	railName, healthName, fuelName := "icon_rail2", "icon_HP2", "icon_SP2"
	if game.BaseVehicleID(item.ID) != item.ID {
		railName, healthName, fuelName = "icon_rail3", "icon_HP3", "icon_SP3"
	}
	pics, screen := f.Env.Pics, f.Env.Screen
	draw := func(name string, value uint32, top int) {
		pics.Draw(screen, pics.Find(railName), x, top, true)
		bar := pics.Find(name)
		w, h := pics.Size(bar)
		width := int(uint64(w) * uint64(value) / uint64(max(1, condition.Maximum)))
		pics.DrawRect(screen, bar, x, top, image.Rect(0, 0, width, h), true)
	}
	draw(healthName, condition.Health, y+vehicleHealthTop)
	if condition.Fuel > 0 {
		draw(fuelName, condition.Fuel, y+vehicleFuelTop)
	}
}
