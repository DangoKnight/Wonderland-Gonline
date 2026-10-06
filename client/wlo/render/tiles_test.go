package render

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"

	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientimage"
)

func exerciseTiles(d *Device) error {
	dir, err := os.MkdirTemp("", "wlo-gpu-tiles-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Join(dir, "runtime", "images"), 0700); err != nil {
		return err
	}
	descriptor := clientimage.Descriptor{Version: clientimage.Version, Source: "test.png", Width: 551, Height: 319}
	for i, at := range []image.Point{{7, 11}, {245, 59}} {
		page := image.NewNRGBA(image.Rect(0, 0, 256, 256))
		for y := 0; y < 256; y++ {
			for x := 0; x < 256; x++ {
				page.SetNRGBA(x, y, color.NRGBA{R: byte(x*3 + i), G: byte(y * 7), B: byte(x * y), A: 255})
			}
		}
		data, err := clientimage.EncodeChunk(page)
		if err != nil {
			return err
		}
		name := fmt.Sprintf("runtime/images/page-%d.wlt", i)
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			return err
		}
		descriptor.Tiles = append(descriptor.Tiles, clientimage.Tile{Rect: image.Rectangle{Min: at, Max: at.Add(image.Pt(193, 187))}, Page: image.Rect(13, 9, 206, 196), File: name})
	}
	data, err := clientimage.EncodeDescriptor(descriptor)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "test.png")
	if err := os.WriteFile(path+clientimage.DescriptorSuffix, data, 0600); err != nil {
		return err
	}
	m, err := clientimage.Open(path)
	if err != nil {
		return err
	}
	src := surface.FromCompiled(m)
	for _, scaled := range []bool{false, true} {
		for _, transparent := range []bool{false, true} {
			for _, at := range []image.Point{{0, 0}, {-77, -31}, {19, 13}} {
				cpu := surface.New(219, 139)
				gpu := d.NewSurface(cpu.W, cpu.H)
				cpu.Fill(image.Rect(0, 0, cpu.W, cpu.H), 0x2377)
				gpu.Fill(image.Rect(0, 0, cpu.W, cpu.H), 0x2377)
				for _, s := range []*surface.Surface{cpu, gpu} {
					if scaled {
						s.DrawStretch(image.Rect(at.X, at.Y, at.X+177, at.Y+123), src, transparent)
					} else {
						s.DrawRect(at.X, at.Y, image.Rect(19, 23, 497, 300), src, transparent)
					}
				}
				got, want := gpu.RGBA(), cpu.RGBA()
				gpu.Close()
				for i := range want.Pix {
					if got.Pix[i] != want.Pix[i] {
						return fmt.Errorf("tiles scaled=%v transparent=%v at=%v byte %d GPU=%d CPU=%d", scaled, transparent, at, i, got.Pix[i], want.Pix[i])
					}
				}
			}
		}
	}
	src.Key = 0x21ee
	cpuKey, gpuKey := surface.New(219, 139), d.NewSurface(219, 139)
	for _, canvas := range []*surface.Surface{cpuKey, gpuKey} {
		canvas.Fill(image.Rect(0, 0, 219, 139), 0xffff)
		canvas.DrawStretch(image.Rect(-7, 1, 212, 139), src, true)
	}
	gotKey, wantKey := gpuKey.RGBA(), cpuKey.RGBA()
	gpuKey.Close()
	for i := range wantKey.Pix {
		if gotKey.Pix[i] != wantKey.Pix[i] {
			return fmt.Errorf("scaled nonzero key byte %d: GPU=%d CPU=%d", i, gotKey.Pix[i], wantKey.Pix[i])
		}
	}
	src.Key = 0
	// Same parity when the original map remains a loose large CPU source.
	loose := surface.New(551, 319)
	for i := range loose.Pix {
		loose.Pix[i] = uint16(i * 59)
	}
	for _, scaled := range []bool{false, true} {
		cpu := surface.New(219, 139)
		gpu := d.NewSurface(cpu.W, cpu.H)
		for _, s := range []*surface.Surface{cpu, gpu} {
			s.Fill(image.Rect(0, 0, s.W, s.H), 0x7fff)
			if scaled {
				s.DrawStretch(image.Rect(-11, -23, 297, 201), loose, true)
			} else {
				s.DrawRect(-3, -7, image.Rect(230, 119, 551, 319), loose, true)
			}
		}
		got, want := gpu.RGBA(), cpu.RGBA()
		gpu.Close()
		for i := range want.Pix {
			if got.Pix[i] != want.Pix[i] {
				return fmt.Errorf("loose tiled image scaled=%v byte %d GPU=%d CPU=%d", scaled, i, got.Pix[i], want.Pix[i])
			}
		}
	}
	// Missing offscreen data must stay unopened in GPU rendering.
	descriptor.Tiles = append(descriptor.Tiles, clientimage.Tile{Rect: image.Rect(501, 291, 531, 311), Page: image.Rect(0, 0, 30, 20), File: "runtime/images/missing.wlt"})
	data, err = clientimage.EncodeDescriptor(descriptor)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path+clientimage.DescriptorSuffix, data, 0600); err != nil {
		return err
	}
	m, err = clientimage.Open(path)
	if err != nil {
		return err
	}
	called := false
	if err := m.VisitTiles(image.Rect(0, 0, 219, 139), func(v clientimage.TileView) error {
		if v.Path == filepath.Join(dir, "runtime", "images", "missing.wlt") {
			called = true
		}
		return nil
	}); err != nil {
		return err
	}
	if called {
		return fmt.Errorf("visited offscreen page")
	}
	gpu := d.NewSurface(219, 139)
	defer gpu.Close()
	gpu.Draw(0, 0, surface.FromCompiled(m), false)
	return nil
}

func exerciseEviction() error {
	d, err := NewDevice()
	if err != nil {
		return err
	}
	defer d.Close()
	d.cacheLimit = 4 << 10
	cpu := surface.New(64, 64)
	gpu := d.NewSurface(64, 64)
	for i := 0; i < 64; i++ {
		sprite := surface.New(16, 16)
		sprite.Fill(image.Rect(0, 0, 16, 16), uint16(i*977))
		x, y := (i%4)*16, (i/4%4)*16
		cpu.Draw(x, y, sprite, false)
		gpu.Draw(x, y, sprite, false)
		if d.CacheBytes() > d.cacheLimit {
			return fmt.Errorf("texture cache exceeded budget")
		}
	}
	got, want := gpu.RGBA(), cpu.RGBA()
	for i := range want.Pix {
		if got.Pix[i] != want.Pix[i] {
			return fmt.Errorf("eviction changed queued draw at byte %d", i)
		}
	}
	gpu.Close()
	d.Close()
	if d.Stats().TextureBytes != 0 || d.Stats().Textures != 0 || d.Stats().Targets != 0 {
		return fmt.Errorf("GPU resources leaked after close: %+v", d.Stats())
	}
	return nil
}
