package hud

import (
	"image"
	"image/color"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

func TestVehicleStatusHealthAndFuelStrips(t *testing.T) {
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
	env := &seui.Env{Pics: pics, Screen: surface.New(800, 600)}
	seui.NewManager(env, &seui.Input{})
	v := newVehicleStatus(env, seui.NewForm(env))
	active := true
	v.Read = func() (VehicleInfo, bool) {
		return VehicleInfo{Name: "Raft", Columns: 4, Rows: 3, Health: 50, Maximum: 100, Fuel: 25}, active
	}
	v.Refresh()
	v.Paint()
	pixel := func(x, y int) uint16 { return env.Screen.Pix[y*800+x] }
	if !v.Visible || pixel(60, 212) != surface.RGB565(255, 0, 0) || pixel(75, 212) != surface.RGB565(128, 128, 128) || pixel(55, 217) != surface.RGB565(0, 0, 255) || pixel(65, 217) != surface.RGB565(128, 128, 128) {
		t.Fatal("health/fuel strip position or fill")
	}
	active = false
	v.Refresh()
	if v.Visible {
		t.Fatal("inactive vehicle HUD visible")
	}
}
