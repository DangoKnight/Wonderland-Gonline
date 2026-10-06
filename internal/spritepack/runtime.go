package spritepack

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientruntime"
)

const RuntimeFile = "runtime.bin"
const RuntimeSpritesDir = "records"

type RuntimeSprite struct {
	Sprite  Sprite
	Palette *[256][3]uint8
	Indices [][]byte
}

func RuntimeSpritePath(index int) string {
	return filepath.Join(RuntimeSpritesDir, fmt.Sprintf("%d.bin", index))
}
func ReadRuntime(dir string) (*Pack, error) {
	var p Pack
	if err := clientruntime.Read(filepath.Join(dir, RuntimeFile), &p); err != nil {
		return nil, err
	}
	return &p, p.Validate()
}
func ReadRuntimeSprite(dir string, p *Pack, index int) (*RuntimeSprite, error) {
	var r RuntimeSprite
	if err := clientruntime.Read(filepath.Join(dir, RuntimeSpritePath(index)), &r); err != nil {
		return nil, err
	}
	check := *p
	check.Sprites = []Sprite{r.Sprite}
	if r.Sprite.Index != index {
		return nil, fmt.Errorf("runtime sprite index mismatch")
	}
	if err := check.Validate(); err != nil {
		return nil, err
	}
	if len(r.Indices) != len(r.Sprite.Frames) {
		return nil, fmt.Errorf("runtime sprite frame count mismatch")
	}
	for i, ix := range r.Indices {
		f := r.Sprite.Frames[i]
		if len(ix) != 0 && (r.Palette == nil || len(ix) != f.Rect.W*f.Rect.H) {
			return nil, fmt.Errorf("runtime sprite indices mismatch")
		}
	}
	return &r, nil
}

// CompileRuntime matches edited PNGs once at build time, rather than opening
// large lossless native exports in every client process. Sheets stay untouched.
func CompileRuntime(dir, archive string, write func(string, any) error) error {
	p, err := EditablePack(dir, archive)
	if err != nil {
		return err
	}
	e, err := clientassets.OpenEditableSprites(filepath.Join(dir, EditableFile))
	if err != nil {
		return err
	}
	// Validate every reference even when a sprite has no lossless original.
	configs := make([]image.Config, len(p.Pages))
	for i, name := range p.Pages {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			return err
		}
		if cfg.Width > MaxPageSide || cfg.Height > MaxPageSide {
			return fmt.Errorf("sprite sheet exceeds page limit: %s", name)
		}
		configs[i] = cfg
	}
	for _, sprite := range p.Sprites {
		for _, f := range sprite.Frames {
			if f.Rect.W == 0 || f.Rect.H == 0 {
				continue
			}
			cfg := configs[f.Page]
			if !image.Rect(f.Rect.X, f.Rect.Y, f.Rect.X+f.Rect.W, f.Rect.Y+f.Rect.H).In(image.Rect(0, 0, cfg.Width, cfg.Height)) {
				return fmt.Errorf("sprite %s frame outside PNG page", sprite.Name)
			}
		}
	}
	n, err := clientassets.OpenSpriteJSON(filepath.Join(dir, NativeFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if n != nil {
		defer n.Close()
	}
	for _, s := range p.Sprites {
		r := RuntimeSprite{Sprite: s, Indices: make([][]byte, len(s.Frames))}
		var original *clientassets.Sprite
		if n != nil && s.Index < len(n.Archive.Entries) {
			original, err = n.Archive.Sprite(s.Index)
			if err != nil {
				return err
			}
		}
		if original != nil {
			src := e.Sprite(s.Index)
			r.Palette = &original.Palette
			for i, f := range src.Frames {
				if i >= len(original.Frames) {
					continue
				}
				img, err := e.FrameImage(f)
				if err != nil {
					return err
				}
				ix := MatchIndices(img, original.Frames[i], r.Palette)
				if ix != nil {
					r.Indices[i] = tightIndices(ix)
				}
			}
		}
		if err := write(RuntimeSpritePath(s.Index), r); err != nil {
			return err
		}
	}
	for i := range p.Sprites {
		s := &p.Sprites[i]
		s.Frames = nil
		s.Animations = nil
		s.Palette = ""
	}
	return write(RuntimeFile, p)
}
func tightIndices(img *image.Gray) []byte {
	b := img.Bounds()
	out := make([]byte, b.Dx()*b.Dy())
	for y := 0; y < b.Dy(); y++ {
		copy(out[y*b.Dx():], img.Pix[y*img.Stride:y*img.Stride+b.Dx()])
	}
	return out
}
