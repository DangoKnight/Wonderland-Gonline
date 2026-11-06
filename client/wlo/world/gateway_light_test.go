package world

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

func TestGatewayLightDoesNotRequireSpriteDatabase(t *testing.T) {
	now := time.Unix(0, 0)
	screen := surface.New(gatewayWidth, gatewayHeight)
	w := &World{Env: &seui.Env{Screen: screen}, lightsFrom: now, Areas: []Area{{Light: LargeLight, LightAt: image.Pt(gatewayWidth/2, gatewayAnchorY)}}}
	w.drawLights(0, 0, now)
	for _, pixel := range screen.Pix {
		if pixel != 0 {
			return
		}
	}
	t.Fatal("procedural gateway did not draw without sprite resources")
}

func TestGatewayLightContinuousAnimation(t *testing.T) {
	p := newGatewayLight()
	p.render(0)
	first := append([]uint16(nil), p.pixels.Pix...)
	previous := first
	for frame := 1; frame <= 30; frame++ {
		p.render(time.Duration(frame) * time.Second / 60)
		if reflect.DeepEqual(previous, p.pixels.Pix) {
			t.Fatalf("gateway paused at frame %d", frame)
		}
		previous = append([]uint16(nil), p.pixels.Pix...)
	}
	p.render(gatewayPeriod)
	if !reflect.DeepEqual(first, p.pixels.Pix) {
		t.Fatal("gateway does not loop seamlessly")
	}
	for frame := 0; frame < 60; frame++ {
		p.render(time.Duration(frame) * gatewayPeriod / 60)
		for y := 0; y < gatewayHeight; y++ {
			for x := 0; x < gatewayWidth; x++ {
				if (x == 0 || y == 0 || x == gatewayWidth-1 || y == gatewayHeight-1) && p.pixels.Pix[y*gatewayWidth+x] != 0 {
					t.Fatalf("gateway light clipped at phase %d pixel %d,%d", frame, x, y)
				}
			}
		}
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		preview := surface.New(gatewayWidth*4, gatewayHeight)
		preview.Fill(image.Rect(0, 0, preview.W, preview.H), surface.RGB565(80, 90, 45))
		for frame := 0; frame < 4; frame++ {
			p.render(time.Duration(frame) * gatewayPeriod / 4)
			p.draw(preview, frame*gatewayWidth+gatewayWidth/2, gatewayAnchorY)
		}
		f, err := os.Create(filepath.Join(dir, "gateway.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, preview.RGBA()); err != nil {
			t.Fatal(err)
		}
	}
}
