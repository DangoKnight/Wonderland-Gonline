package world

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkerPrismRotation(t *testing.T) {
	var m Marker
	m.renderPrism(30, 1)
	previous := bytes.Clone(m.prism.Pix)
	for frame := 1; frame <= 12; frame++ {
		m.spin = markerSpin(time.Duration(frame) * time.Second / 60)
		m.renderPrism(30, 1)
		if bytes.Equal(previous, m.prism.Pix) {
			t.Fatalf("diamond stopped spinning at frame %d", frame)
		}
		previous = bytes.Clone(m.prism.Pix)
	}
	colors := map[color.NRGBA]bool{}
	for y := 0; y < markerSize; y++ {
		for x := 0; x < markerSize; x++ {
			c := m.prism.NRGBAAt(x, y)
			if c.A == 255 {
				colors[c] = true
			}
		}
	}
	if len(colors) < 3 {
		t.Fatal("diamond lacks distinct shaded facets")
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		preview := image.NewNRGBA(image.Rect(0, 0, markerSize*4, markerSize))
		draw.Draw(preview, preview.Bounds(), image.NewUniform(color.NRGBA{R: 30, G: 35, B: 25, A: 255}), image.Point{}, draw.Src)
		for i := 0; i < 4; i++ {
			m.spin = float64(i) * math.Pi / 8
			m.render(0)
			draw.Draw(preview, image.Rect(i*markerSize, 0, (i+1)*markerSize, markerSize), m.pixels, image.Point{}, draw.Over)
		}
		f, err := os.Create(filepath.Join(dir, "waypoint-spin.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, preview); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMarkerContinuousAnimation(t *testing.T) {
	var m Marker
	now := time.Unix(0, 0)
	at := image.Pt(142, 315)
	m.Show(at)
	if !m.frame(now) || m.At() != at {
		t.Fatal("marker did not start at its destination")
	}
	first := bytes.Clone(m.pixels.Pix)
	previous := first
	for i := 1; i <= 20; i++ {
		if !m.frame(now.Add(time.Duration(i) * time.Second / 60)) {
			t.Fatal("marker expired early")
		}
		if bytes.Equal(previous, m.pixels.Pix) {
			t.Fatalf("animation paused at frame %d", i)
		}
		previous = bytes.Clone(m.pixels.Pix)
	}
	if m.frame(now.Add(markerDuration)) || m.Shown() {
		t.Fatal("marker outlived its animation")
	}
	m.Show(at)
	if !m.frame(now.Add(time.Second)) || !bytes.Equal(first, m.pixels.Pix) {
		t.Fatal("repeated click did not restart animation")
	}
	m.Hide()
	if m.frame(now.Add(time.Second)) {
		t.Fatal("hidden marker rendered")
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		preview := image.NewNRGBA(image.Rect(0, 0, markerSize*4, markerSize))
		draw.Draw(preview, preview.Bounds(), image.NewUniform(color.NRGBA{R: 30, G: 35, B: 25, A: 255}), image.Point{}, draw.Src)
		for i := 0; i < 4; i++ {
			m.render(float64(i) / 4)
			draw.Draw(preview, image.Rect(i*markerSize, 0, (i+1)*markerSize, markerSize), m.pixels, image.Point{}, draw.Over)
		}
		f, err := os.Create(filepath.Join(dir, "waypoint.png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, preview); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMarkerDragRelease(t *testing.T) {
	var m Marker
	now := time.Unix(0, 0)
	m.Show(image.Pt(142, 315))
	m.SetHeld(true, now)
	if !m.frame(now.Add(3 * time.Second)) {
		t.Fatal("held marker expired")
	}
	held := bytes.Clone(m.pixels.Pix)
	m.frame(now.Add(3250 * time.Millisecond))
	if bytes.Equal(held, m.pixels.Pix) {
		t.Fatal("held prism did not bob")
	}
	for _, elapsed := range []time.Duration{3250 * time.Millisecond, 3750 * time.Millisecond} {
		m.frame(now.Add(elapsed))
		for x := 0; x < markerSize; x++ {
			if m.pixels.NRGBAAt(x, 0).A != 0 || m.pixels.NRGBAAt(x, markerSize-1).A != 0 {
				t.Fatal("bobbing prism clipped its canvas")
			}
		}
	}
	m.frame(now.Add(3 * time.Second))
	m.Show(image.Pt(162, 335))
	if !m.held || m.At() != image.Pt(162, 335) {
		t.Fatal("drag reaim lost held state")
	}
	m.SetHeld(false, now.Add(3*time.Second))
	if !m.frame(now.Add(3*time.Second)) || !bytes.Equal(held, m.pixels.Pix) {
		t.Fatal("release jumped instead of morphing from circle")
	}
	m.frame(now.Add(3*time.Second + markerMorphDuration))
	if bytes.Equal(held, m.pixels.Pix) {
		t.Fatal("circle did not morph into inward curves")
	}
	m.frame(now.Add(3*time.Second + markerDuration*9/10))
	for y := 0; y < markerSize; y++ {
		for x := 0; x < markerSize; x++ {
			if (x < markerSize/2-8 || x > markerSize/2+8) && m.pixels.NRGBAAt(x, y).A != 0 {
				t.Fatal("released strokes remained outside instead of sinking into center")
			}
		}
	}
	if m.frame(now.Add(3*time.Second + markerDuration)) {
		t.Fatal("released marker did not expire")
	}
}
