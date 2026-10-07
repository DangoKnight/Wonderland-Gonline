package render

import (
	"fmt"
	"image"
	"image/color"
	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/surface"
)

// Invoked inside the single graphics-test game loop in framebuffer_test.go.
func exerciseRaster(d *Device) error {
	const w, h = 193, 117
	cpu := surface.New(w, h)
	gpu := d.NewSurface(w, h)
	defer gpu.Close()
	if gpu.Pix != nil {
		return fmt.Errorf("GPU target has a CPU shadow framebuffer")
	}
	check := func(label string) error {
		got := gpu.RGBA()
		want := cpu.RGBA()
		for i := range want.Pix {
			if got.Pix[i] != want.Pix[i] {
				return fmt.Errorf("%s byte %d: GPU %d CPU %d", label, i, got.Pix[i], want.Pix[i])
			}
		}
		return nil
	}
	if err := check("initial black"); err != nil {
		return err
	}
	source := surface.New(256, 256)
	for i := range source.Pix {
		source.Pix[i] = uint16(i)
	}
	// Covers every native color in an actual raster target, independent of the
	// framebuffer conversion shader.
	full := d.NewSurface(256, 256)
	full.Draw(0, 0, source, false)
	got := full.RGBA()
	want := source.RGBA()

	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			return fmt.Errorf("native raster color byte %d: %d != %d", i, got.Pix[i], want.Pix[i])
		}
	}
	alphaImage := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			alphaImage.SetNRGBA(x, y, color.NRGBA{R: byte(x), G: byte(x*17 + y), B: byte(x * y), A: byte(y)})
		}
	}
	reference := source.Clone()
	loadAlpha := func() (*image.NRGBA, error) { return alphaImage, nil }
	if err := reference.DrawSprite(0, 0, alphaImage, loadAlpha); err != nil {
		return err
	}
	if err := full.DrawSprite(0, 0, alphaImage, loadAlpha); err != nil {
		return err
	}
	got, want = full.RGBA(), reference.RGBA()
	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			return fmt.Errorf("alpha raster byte %d: GPU=%d CPU=%d", i, got.Pix[i], want.Pix[i])
		}
	}
	full.Close()

	draw := func(label string, op func(*surface.Surface)) error { op(cpu); op(gpu); return check(label) }
	for _, rect := range []image.Rectangle{image.Rect(0, 0, w, h), image.Rect(-10, 3, 27, 19), image.Rect(190, 114, 201, 133)} {
		if err := draw("fill", func(s *surface.Surface) { s.Fill(rect, 0x7e31) }); err != nil {
			return err
		}
	}
	for _, alpha := range []int{0, 1, 77, 128, 200, 254, 255} {
		if err := draw(fmt.Sprintf("alpha %d", alpha), func(s *surface.Surface) { s.FillAlpha(image.Rect(-2, 3, w-7, h+5), 0xf98b3d, alpha) }); err != nil {
			return err
		}
	}
	source.Key = 0x1234
	for _, transparent := range []bool{false, true} {
		for _, at := range []image.Point{{-7, -3}, {29, 17}, {190, 111}} {
			if err := draw("clipped blit", func(s *surface.Surface) { s.DrawRect(at.X, at.Y, image.Rect(16, 17, 75, 82), source, transparent) }); err != nil {
				return err
			}
		}
		for _, rect := range []image.Rectangle{image.Rect(-4, -7, 96, 74), image.Rect(55, 20, 284, 177), image.Rect(3, 4, 7, 9)} {
			if err := draw("stretch", func(s *surface.Surface) { s.DrawStretch(rect, source, transparent) }); err != nil {
				return err
			}
		}
	}
	for _, level := range []int{0, 1, 10, 32} {
		if err := draw("light", func(s *surface.Surface) { s.DrawLight(-7, 19, image.Rect(10, 10, 256, 140), source, level) }); err != nil {
			return err
		}
	}
	source.Fill(image.Rect(0, 0, source.W, source.H), 0x4321)
	if err := draw("revised CPU source", func(s *surface.Surface) { s.Draw(0, 0, source, false) }); err != nil {
		return err
	}
	sprite := image.NewNRGBA(image.Rect(10, 13, 43, 40))
	alphas := []uint8{0, 1, 64, 127, 128, 200, 254, 255}
	for y := sprite.Bounds().Min.Y; y < sprite.Bounds().Max.Y; y++ {
		for x := sprite.Bounds().Min.X; x < sprite.Bounds().Max.X; x++ {
			sprite.SetNRGBA(x, y, color.NRGBA{R: byte(x * 11), G: byte(y * 19), B: byte(x * y), A: alphas[(x+y)%len(alphas)]})
		}
	}
	for _, at := range []image.Point{{-7, 0}, {10, 17}, {179, 109}} {
		if err := draw("straight alpha sprite", func(s *surface.Surface) {
			_ = s.DrawSprite(at.X, at.Y, sprite, func() (*image.NRGBA, error) { return sprite, nil })
		}); err != nil {
			return err
		}
	}
	for _, at := range []image.Point{{-7, -3}, {2, 5}, {179, 109}} {
		if err := draw("scaled alpha pet sprite", func(s *surface.Surface) {
			_ = s.DrawSpriteScaled(at.X, at.Y, 2, sprite, func() (*image.NRGBA, error) { return sprite, nil })
		}); err != nil {
			return err
		}
	}
	indices := image.NewGray(image.Rect(4, 7, 68, 39))
	for i := range indices.Pix {
		indices.Pix[i] = byte(i)
	}
	var lut [256]uint16
	var opaque [256]bool
	for i := range lut {
		lut[i] = uint16(i * 179)
		opaque[i] = i%7 != 0
	}
	for _, at := range []image.Point{{-3, -7}, {56, 13}} {
		if err := draw("indexed sprite", func(s *surface.Surface) { s.DrawIndexed(at.X, at.Y, indices, indices, lut, opaque) }); err != nil {
			return err
		}
	}
	for _, at := range []image.Point{{-7, -3}, {2, 5}, {179, 109}} {
		if err := draw("scaled indexed pet sprite", func(s *surface.Surface) { s.DrawIndexedScaled(at.X, at.Y, 2, indices, indices, lut, opaque) }); err != nil {
			return err
		}
	}
	lut[2] = 0
	opaque[2] = true // Opaque native black must not become a color key.
	if err := draw("palette revision", func(s *surface.Surface) { s.DrawIndexed(11, 13, indices, indices, lut, opaque) }); err != nil {
		return err
	}
	// Freeze and zoom remain GPU-to-GPU operations.
	for _, s := range []*surface.Surface{cpu, gpu} {
		frozen := s.Clone()
		s.Fill(image.Rect(0, 0, w, h), 0x7fff)
		s.DrawRect(0, 0, image.Rect(2, 3, w-2, h-3), frozen, false)
		s.DrawStretch(image.Rect(-1, 0, w+1, h), frozen, false)
		frozen.Close()
	}
	if err := check("clone and zoom"); err != nil {
		return err
	}
	uploads := d.Uploads
	for _, s := range []*surface.Surface{cpu, gpu} {
		s.DrawSprite(0, 0, sprite, func() (*image.NRGBA, error) { return sprite, nil })
		s.DrawIndexed(2, 2, indices, indices, lut, opaque)
	}
	if d.Uploads != uploads {
		return fmt.Errorf("immutable sprite assets uploaded again")
	}
	if err := check("cached sprite redraw"); err != nil {
		return err
	}
	for _, width := range []int{88, 144} {
		gauge := surface.New(width, 31)
		for i := range gauge.Pix {
			gauge.Pix[i] = uint16(i * 33)
			if i%5 == 0 {
				gauge.Pix[i] = 0
			}
		}
		db := picdb.New()
		db.Entries = []picdb.Entry{{Image: gauge, Loaded: true, W: gauge.W, H: gauge.H}}
		for _, key := range []uint16{0, 0x1234} {
			gauge.Key = key
			for percent := 0; percent <= 100; percent++ {
				cpu.Fill(image.Rect(0, 0, w, h), 0x2357)
				gpu.Fill(image.Rect(0, 0, w, h), 0x2357)
				db.DrawGauge(cpu, 0, -3, 1, percent)
				db.DrawGauge(gpu, 0, -3, 1, percent)
				if err := check(fmt.Sprintf("gauge width %d percent %d key %x", width, percent, key)); err != nil {
					return err
				}
			}
		}
	}
	if err := exerciseTiles(d); err != nil {
		return err
	}
	if err := exerciseEviction(); err != nil {
		return err
	}
	return nil
}
