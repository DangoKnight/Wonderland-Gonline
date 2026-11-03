package hud

import (
	"image"
	"image/color"
	"testing"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/client/wlo/world"
)

func TestPetStatusNativeLayoutAndLiveStats(t *testing.T) {
	pics := picdb.New()
	add := func(name string, width, height int, ink color.RGBA) {
		im := image.NewRGBA(image.Rect(0, 0, width, height))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				im.SetRGBA(x, y, ink)
			}
		}
		pics.Add(name, im)
	}
	for _, name := range []string{"Word_Lv_1", "Word_Hp_1", "Word_Sp_1", "Pet_Percent_Exp"} {
		add(name, 1, 1, color.RGBA{255, 255, 255, 255})
	}
	for _, name := range []string{"Pet_Frame_Hp", "Pet_Frame_Sp"} {
		add(name, 27, 5, color.RGBA{0, 0, 255, 255})
	}
	for _, name := range []string{"Pet_Bmp_Hp", "Pet_Bmp_Sp"} {
		add(name, 25, 3, color.RGBA{255, 0, 0, 255})
	}
	add("Pet_Frame_Exp", 18, 4, color.RGBA{0, 0, 255, 255})
	add("Pet_Bmp_Exp", 16, 2, color.RGBA{255, 0, 0, 255})
	digits := image.NewRGBA(image.Rect(0, 0, 5, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 5; x++ {
			ink := color.RGBA{255, 255, 255, 255}
			if y/6 == 7 {
				ink = color.RGBA{255, 0, 0, 255}
			}
			if y/6 == 5 {
				ink = color.RGBA{0, 0, 255, 255}
			}
			digits.SetRGBA(x, y, ink)
		}
	}
	pics.Add("Pet_Num_Lv_White_1", digits)
	env := &seui.Env{Pics: pics, Screen: surface.New(800, 600)}
	input := &seui.Input{X: -1, Y: -1}
	seui.NewManager(env, input)
	p := newPetStatus(env, seui.NewForm(env))
	info := PetInfo{ID: 12032, Stats: world.Stats{Level: 3, HP: 50, MaxHP: 100, SP: 5, MaxSP: 20, EXP: 9, Formula: &login.Formula{ExpPower: 1}}}
	active := true
	p.Read = func() (PetInfo, bool) { return info, active }
	var portrait image.Point
	p.DrawPortrait = func(id uint16, x, y int) {
		if id != 12032 {
			t.Fatal(id)
		}
		portrait = image.Pt(x, y)
	}
	p.Refresh()
	p.Update(input)
	if portrait != image.Pt(158, 42) {
		t.Fatal("native portrait anchor", portrait)
	}
	// Coordinates/lengths authored from the native layout, independent of
	// production layout constants: level 3, HP 50%, SP 25%, EXP 75%.
	for _, sample := range []struct {
		x, y int
		want uint16
	}{
		{194, 50, 0xffff}, {191, 60, 0xf800}, {202, 60, 0xf800}, {203, 60, 0x001f},
		{196, 68, 0xf800}, {197, 68, 0x001f}, {158, 79, 0xf800}, {159, 79, 0x001f},
		{167, 77, 0xf800}, {172, 77, 0x001f},
	} {
		if got := env.Screen.Pix[sample.y*800+sample.x]; got != sample.want {
			t.Fatalf("pixel %d,%d: %04x want %04x", sample.x, sample.y, got, sample.want)
		}
	}
	info.Stats.HP = 80
	p.Refresh()
	p.Update(input)
	if env.Screen.Pix[60*800+210] != 0xf800 || env.Screen.Pix[60*800+211] != 0x001f {
		t.Fatal("live HP not reflected")
	}
	info.Stats.HP = 200
	p.Refresh()
	p.Update(input)
	if env.Screen.Pix[60*800+215] != 0xf800 || env.Screen.Pix[60*800+216] != 0x001f {
		t.Fatal("overfull bar escaped native frame")
	}
	active = false
	p.Refresh()
	if p.Visible {
		t.Fatal("rested pet panel remained visible")
	}
}
