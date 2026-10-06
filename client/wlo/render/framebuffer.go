// Package render provides native-compatible GPU rasterization and CPU canvas
// presentation through Ebitengine. Render targets and asset textures stay on
// the GPU during normal play.
package render

import (
	"fmt"
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"wonderland-gonline/client/wlo/surface"
)

const (
	Auto                    = "auto"
	GPU                     = "gpu"
	CPU                     = "cpu"
	dirtyTileSide           = 32
	texturePixelBytes       = 4
	fullUploadDirtyFraction = 2
)

// The source stores a little-endian RGB565 word in opaque RG texels. Kage
// performs integer channel extraction and bit replication, preserving all
// 65,536 native colors. Nearest sampling keeps encoded words intact.
const rgb565Shader = `//kage:unit pixels
package main
func Fragment(dst vec4, src vec2, color vec4) vec4 {
 bytes := floor(imageSrc0At(src).rg * 255.0 + 0.5)
 word := bytes.x + bytes.y * 256.0
 r := floor(word / 2048.0)
 g := mod(floor(word / 32.0), 64.0)
 b := mod(word, 32.0)
 return vec4((r * 8.0 + floor(r / 4.0)) / 255.0,
             (g * 4.0 + floor(g / 16.0)) / 255.0,
             (b * 8.0 + floor(b / 4.0)) / 255.0, 1.0)
}
`

func Validate(mode string) error {
	switch mode {
	case "", Auto, GPU, CPU:
		return nil
	default:
		return fmt.Errorf("unknown renderer %q: choose auto, gpu or cpu", mode)
	}
}

// Presenter belongs to the window and owns the shared GPU rendering device.
// Call its methods on the Ebitengine game thread. Auto falls back only if the
// shader cannot compile; graphics-device initialization errors remain fatal.
type Presenter struct {
	mode          string
	device        *Device
	shader        *ebiten.Shader
	texture       *ebiten.Image
	previous      []uint16
	upload        []byte
	dirty         []image.Rectangle
	width, height int
	initialized   bool
	UploadedBytes int // Last frame's transfer size; unchanged frames upload zero.
}

func New(mode string) (*Presenter, error) {
	if err := Validate(mode); err != nil {
		return nil, err
	}
	p := &Presenter{mode: CPU}
	if mode == CPU {
		return p, nil
	}
	shader, err := ebiten.NewShader([]byte(rgb565Shader))
	if err != nil {
		if mode == GPU {
			return nil, fmt.Errorf("GPU framebuffer shader: %w", err)
		}
		log.Printf("GPU framebuffer shader unavailable, using CPU conversion: %v", err)
		return p, nil
	}
	device, err := NewDevice()
	if err != nil {
		shader.Deallocate()
		if mode == GPU {
			return nil, err
		}
		log.Printf("GPU rasterizer unavailable, using CPU: %v", err)
		return p, nil
	}
	p.device = device
	p.mode, p.shader = GPU, shader
	return p, nil
}

func (p *Presenter) Mode() string { return p.mode }

func (p *Presenter) Stats() DeviceStats {
	if p.device == nil {
		return DeviceStats{}
	}
	return p.device.Stats()
}

func (p *Presenter) Attach(s *surface.Surface) {
	if p.device != nil {
		p.device.Attach(s)
	}
}

func (p *Presenter) Close() {
	if p.device != nil {
		p.device.Close()
		p.device = nil
	}
	if p.texture != nil {
		p.texture.Deallocate()
		p.texture = nil
	}
	if p.shader != nil {
		p.shader.Deallocate()
		p.shader = nil
	}
	p.previous, p.upload, p.dirty = nil, nil, nil
	p.width, p.height, p.initialized = 0, 0, false
}

// Draw presents GPU targets directly. CPU buffers use dirty-region uploads
// with either GPU expansion or the previous bit-exact CPU expansion.
func (p *Presenter) Draw(dst *ebiten.Image, src *surface.Surface) {
	if t, ok := src.Backend.(*target); ok {
		p.UploadedBytes = 0
		dst.DrawImage(t.image, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
		return
	}
	if p.width != src.W || p.height != src.H {
		if p.texture != nil {
			p.texture.Deallocate()
		}
		p.width, p.height = src.W, src.H
		p.texture = ebiten.NewImage(src.W, src.H)
		p.previous = make([]uint16, src.W*src.H)
		p.upload = make([]byte, src.W*src.H*texturePixelBytes)
		p.initialized = false
	}
	p.dirty = changedRects(p.dirty[:0], src.Pix, p.previous, src.W, src.H, !p.initialized)
	p.UploadedBytes = 0
	for _, r := range p.dirty {
		data := p.upload[:r.Dx()*r.Dy()*texturePixelBytes]
		encode(data, src.Pix, src.W, r, p.mode == GPU)
		p.texture.SubImage(r).(*ebiten.Image).WritePixels(data)
		p.UploadedBytes += len(data)
		for y := r.Min.Y; y < r.Max.Y; y++ {
			start := y*src.W + r.Min.X
			copy(p.previous[start:start+r.Dx()], src.Pix[start:start+r.Dx()])
		}
	}
	p.initialized = true
	if p.mode == GPU {
		dst.DrawRectShader(src.W, src.H, p.shader, &ebiten.DrawRectShaderOptions{Images: [4]*ebiten.Image{p.texture}, Blend: ebiten.BlendCopy})
	} else {
		dst.DrawImage(p.texture, &ebiten.DrawImageOptions{Blend: ebiten.BlendCopy})
	}
}

func encode(dst []byte, src []uint16, stride int, r image.Rectangle, gpu bool) {
	i := 0
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for _, v := range src[y*stride+r.Min.X : y*stride+r.Max.X] {
			if gpu {
				dst[i], dst[i+1], dst[i+2] = byte(v), byte(v>>8), 0
			} else {
				c := surface.Expand(v)
				dst[i], dst[i+1], dst[i+2] = c.R, c.G, c.B
			}
			dst[i+3] = 255
			i += texturePixelBytes
		}
	}
}

// Adjacent dirty tiles are coalesced into horizontal strips. Busy frames use a
// single upload to avoid many driver calls. Sparse animations retain tile-sized
// transfers; unchanged frames do not expand or upload pixels.
func changedRects(out []image.Rectangle, pixels, old []uint16, w, h int, first bool) []image.Rectangle {
	full := image.Rect(0, 0, w, h)
	if first {
		return append(out, full)
	}
	area := 0
	for y := 0; y < h; y += dirtyTileSide {
		run := -1
		bottom := min(y+dirtyTileSide, h)
		for x := 0; x < w; x += dirtyTileSide {
			right := min(x+dirtyTileSide, w)
			changed := false
			for row := y; row < bottom && !changed; row++ {
				for col := x; col < right; col++ {
					i := row*w + col
					if pixels[i] != old[i] {
						changed = true
						break
					}
				}
			}
			if changed {
				area += (right - x) * (bottom - y)
				if run < 0 {
					run = x
				}
			} else if run >= 0 {
				out = append(out, image.Rect(run, y, x, bottom))
				run = -1
			}
		}
		if run >= 0 {
			out = append(out, image.Rect(run, y, w, bottom))
		}
	}
	if area*fullUploadDirtyFraction >= w*h {
		return append(out[:0], full)
	}
	return out
}
