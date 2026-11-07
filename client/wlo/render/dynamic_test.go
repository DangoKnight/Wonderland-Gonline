package render

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"wonderland-gonline/client/wlo/surface"
)

// Runs inside the graphics test's game loop. Queue many versions before the
// readback, so cache reuse must preserve old draws as well as the latest image.
func exerciseDynamicTextures() error {
	d, err := NewDevice()
	if err != nil {
		return err
	}
	defer d.Close()
	d.cacheLimit = 8 << 10
	cpu, gpu := surface.New(128, 16), d.NewSurface(128, 16)
	defer gpu.Close()
	cpu.Fill(image.Rect(0, 0, 128, 16), 0x31c8)
	gpu.Fill(image.Rect(0, 0, 128, 16), 0x31c8)
	native := surface.New(2, 2)
	painted := image.NewNRGBA(image.Rect(5, 7, 9, 9))
	identity := new(int)
	var revision uint64
	loads := 0
	load := func() (*image.NRGBA, error) { loads++; return painted, nil }
	for frame := 0; frame < 32; frame++ {
		for i := range native.Pix {
			native.Pix[i] = uint16(frame*977 + i*79)
		}
		native.Revision++
		alpha := []uint8{0, 128, 255}[frame%3]
		for y := painted.Rect.Min.Y; y < painted.Rect.Max.Y; y++ {
			for x := painted.Rect.Min.X; x < painted.Rect.Max.X; x++ {
				painted.SetNRGBA(x, y, color.NRGBA{R: byte(frame * 7), G: 177, B: 31, A: alpha})
			}
		}
		revision++
		for _, s := range []*surface.Surface{cpu, gpu} {
			s.Draw(frame*4, 0, native, false)
			if err := s.DrawSprite(frame*4, 4, surface.DynamicSprite{Identity: identity, Revision: revision}, load); err != nil {
				return err
			}
		}
	}
	stats := d.Stats()
	if stats.Textures != 2 || stats.TextureBytes != 48 {
		return fmt.Errorf("dynamic revisions retained stale textures: %+v", stats)
	}
	check := func(label string) error {
		got, want := gpu.RGBA(), cpu.RGBA()
		for i := range want.Pix {
			if got.Pix[i] != want.Pix[i] {
				return fmt.Errorf("%s byte %d GPU=%d CPU=%d", label, i, got.Pix[i], want.Pix[i])
			}
		}
		return nil
	}
	if err := check("ordered dynamic updates and alpha changes"); err != nil {
		return err
	}
	before := loads
	if err := gpu.DrawSprite(0, 8, surface.DynamicSprite{Identity: identity, Revision: revision}, load); err != nil {
		return err
	}
	_ = cpu.DrawSprite(0, 8, surface.DynamicSprite{Identity: identity, Revision: revision}, load)
	if loads != before+1 || d.Uploads != stats.Uploads {
		return fmt.Errorf("unchanged revision decoded or uploaded again")
	}
	if err := check("unchanged dynamic revision"); err != nil {
		return err
	}
	// A failed update must remain retryable without replacing the resident entry.
	revision++
	failure := errors.New("image temporarily unavailable")
	if err := gpu.DrawSprite(8, 8, surface.DynamicSprite{Identity: identity, Revision: revision}, func() (*image.NRGBA, error) { return nil, failure }); !errors.Is(err, failure) {
		return fmt.Errorf("dynamic failure lost: %v", err)
	}
	// Same identity, new dimensions: retire the old storage and update accounting.
	painted = image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for i := range painted.Pix {
		painted.Pix[i] = 255
	}
	for _, s := range []*surface.Surface{cpu, gpu} {
		if err := s.DrawSprite(8, 8, surface.DynamicSprite{Identity: identity, Revision: revision}, load); err != nil {
			return err
		}
	}
	if d.Stats().Textures != 2 || d.CacheBytes() != 144 {
		return fmt.Errorf("resized dynamic accounting: %+v", d.Stats())
	}
	if err := check("retry and resized dynamic image"); err != nil {
		return err
	}
	// Force eviction, then reload a mutable source at its unchanged revision.
	d.cacheLimit = 512
	for i := 0; i < 8; i++ {
		source := surface.New(8, 8)
		source.Fill(image.Rect(0, 0, 8, 8), uint16(i*977))
		for _, s := range []*surface.Surface{cpu, gpu} {
			s.Draw(i*8, 8, source, false)
		}
	}
	for _, s := range []*surface.Surface{cpu, gpu} {
		if err := s.DrawSprite(80, 8, surface.DynamicSprite{Identity: identity, Revision: revision}, load); err != nil {
			return err
		}
	}
	if d.CacheBytes() > d.cacheLimit {
		return fmt.Errorf("mutable cache exceeded limit")
	}
	return check("dynamic eviction preserves queued draws")
}
