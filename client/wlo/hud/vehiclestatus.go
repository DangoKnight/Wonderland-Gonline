package hud

import (
	"fmt"
	"image"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
)

// FUN_00262974 scales the item footprint to ten pixels per cell and places
// native health/fuel strips to its right, below the player status.
const (
	vehicleStatusLeft     = 5
	vehicleStatusTop      = 200
	vehicleIconCellPixels = 10
	vehicleGaugeGap       = 5
	vehicleGaugeTop       = 195
)

type VehicleInfo struct {
	Name                  string
	Icon                  uint16
	Columns, Rows         int
	Health, Maximum, Fuel uint32
}

type VehicleStatus struct {
	seui.Component
	Read    func() (VehicleInfo, bool)
	current VehicleInfo
}

func newVehicleStatus(env *seui.Env, owner seui.Control) *VehicleStatus {
	v := &VehicleStatus{}
	v.InitComponent(v, env, owner)
	v.Name = "VehicleStatus"
	v.Init("", vehicleStatusLeft, 0, 0, 0, 0, false, 40, 100, vehicleStatusTop)
	v.Visible = false
	return v
}

func (v *VehicleStatus) Refresh() {
	v.Visible = false
	if v.Read == nil {
		return
	}
	info, ok := v.Read()
	if !ok {
		return
	}
	v.current, v.Visible = info, true
	v.Width = max(1, info.Columns)*vehicleIconCellPixels + 50
	v.Height = max(1, info.Rows) * vehicleIconCellPixels
	hint := info.Name
	if info.Fuel > 0 {
		hint += fmt.Sprintf("\nFuel: %d%%", percent(info.Fuel, info.Maximum))
	}
	v.SetHint([]byte(hint))
}

func (v *VehicleStatus) Paint() {
	i := v.current
	cols, rows := max(1, i.Columns), max(1, i.Rows)
	pics, scr := v.Env.Pics, v.Env.Screen
	icon := pics.Find(strconv.Itoa(int(i.Icon)))
	pics.DrawStretch(scr, icon, image.Rect(vehicleStatusLeft, vehicleStatusTop, vehicleStatusLeft+cols*vehicleIconCellPixels, vehicleStatusTop+rows*vehicleIconCellPixels), true)
	x, y := cols*vehicleIconCellPixels+10, vehicleGaugeTop+rows*vehicleIconCellPixels/2
	rail := pics.Find("icon_rail2")
	draw := func(name string, value uint32, top int) {
		pics.Draw(scr, rail, x, top, true)
		bar := pics.Find(name)
		w, h := pics.Size(bar)
		filled := int(uint64(w) * uint64(min(value, i.Maximum)) / uint64(max(1, i.Maximum)))
		pics.DrawRect(scr, bar, x, top, image.Rect(0, 0, filled, h), true)
	}
	draw("icon_HP2", i.Health, y)
	if i.Fuel > 0 {
		draw("icon_SP2", i.Fuel, y+vehicleGaugeGap)
	}
}
