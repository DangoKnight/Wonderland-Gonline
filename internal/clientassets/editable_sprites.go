package clientassets

import (
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/clientfs"
)

const (
	editableMaxCanvasSide       = 1 << 16
	editableMaxCachedSheetBytes = 64 << 20
	EditableSpriteVersion       = 1
	EditableSheetSide           = 2048
	editableMaxSheetSide        = 4096
	editableMaxFrames           = 1 << 16
)

// EditableSprites is an ordinary JSON index into transparent PNG sheets.
// Frame rectangles, canvas sizes and anchors can all be changed by artists.
type EditableSprites struct {
	Version     int              `json:"version"`
	Sprites     []EditableSprite `json:"sprites"`
	root        string
	sheets      map[string]*image.NRGBA
	cachedBytes int
	indexed     map[int]*EditableSprite
}
type EditableSprite struct {
	Name       string            `json:"name"`
	Index      int               `json:"index"`
	Animations []SpriteAnimation `json:"animations"`
	Frames     []EditableFrame   `json:"frames"`
}
type SpriteAnimation struct {
	Name   string `json:"name"`
	Frames []int  `json:"frames"`
}
type EditableFrame struct {
	Name         string     `json:"name"`
	Sheet        string     `json:"sheet,omitempty"`
	Rect         SpriteRect `json:"rect"`
	CanvasWidth  int        `json:"canvas_width"`
	CanvasHeight int        `json:"canvas_height"`
	AnchorX      int        `json:"anchor_x"`
	AnchorY      int        `json:"anchor_y"`
}
type SpriteRect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (r SpriteRect) Bounds() image.Rectangle { return image.Rect(r.X, r.Y, r.X+r.Width, r.Y+r.Height) }

func OpenEditableSprites(path string) (*EditableSprites, error) {
	data, err := clientfs.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out EditableSprites
	if err = json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if out.Version != EditableSpriteVersion {
		return nil, fmt.Errorf("unsupported editable sprite version %d", out.Version)
	}
	out.root = filepath.Dir(path)
	out.sheets = map[string]*image.NRGBA{}
	out.indexed = map[int]*EditableSprite{}
	for i := range out.Sprites {
		sprite := &out.Sprites[i]
		if sprite.Index < 0 || sprite.Index >= editableMaxFrames || out.indexed[sprite.Index] != nil {
			return nil, fmt.Errorf("invalid or duplicate sprite index %d", sprite.Index)
		}
		out.indexed[sprite.Index] = sprite
		if len(sprite.Frames) > editableMaxFrames {
			return nil, fmt.Errorf("too many frames: %s", sprite.Name)
		}
		for _, f := range sprite.Frames {
			if f.Rect.X < 0 || f.Rect.Y < 0 || f.Rect.Width < 0 || f.Rect.Height < 0 || f.Rect.Width > editableMaxSheetSide || f.Rect.Height > editableMaxSheetSide || f.Rect.X > editableMaxSheetSide-f.Rect.Width || f.Rect.Y > editableMaxSheetSide-f.Rect.Height {
				return nil, fmt.Errorf("invalid frame rectangle: %s", sprite.Name)
			}
			if f.CanvasWidth < 0 || f.CanvasHeight < 0 || f.CanvasWidth > editableMaxCanvasSide || f.CanvasHeight > editableMaxCanvasSide || f.AnchorX < -editableMaxCanvasSide || f.AnchorX > editableMaxCanvasSide || f.AnchorY < -editableMaxCanvasSide || f.AnchorY > editableMaxCanvasSide {
				return nil, fmt.Errorf("negative canvas size: %s", sprite.Name)
			}
			if f.Rect.Width != 0 && f.Rect.Height != 0 {
				if !safeSheetPath(f.Sheet) {
					return nil, fmt.Errorf("invalid PNG path: %q", f.Sheet)
				}
			}
		}
		for _, animation := range sprite.Animations {
			for _, index := range animation.Frames {
				if index < -1 || index >= len(sprite.Frames) {
					return nil, fmt.Errorf("animation %s has invalid frame %d", animation.Name, index)
				}
			}
		}
	}
	return &out, nil
}
func safeSheetPath(path string) bool {
	clean := filepath.Clean(filepath.FromSlash(path))
	return path != "" && !filepath.IsAbs(clean) && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator)) && !strings.ContainsAny(path, "\\:") && strings.EqualFold(filepath.Ext(clean), ".png")
}

// FrameImage loads PNGs lazily; original files and palette restrictions are not
// involved. Changes become visible after restarting the client (cached sheets).
func (a *EditableSprites) FrameImage(f EditableFrame) (*image.NRGBA, error) {
	if f.Rect.Width == 0 || f.Rect.Height == 0 {
		return image.NewNRGBA(image.Rectangle{}), nil
	}
	if !safeSheetPath(f.Sheet) {
		return nil, fmt.Errorf("invalid PNG path")
	}
	sheet, ok := a.sheets[f.Sheet]
	if !ok {
		root, err := os.OpenRoot(a.root)
		if err != nil {
			return nil, err
		}
		defer root.Close()
		file, err := root.Open(filepath.FromSlash(f.Sheet))
		if err != nil {
			return nil, err
		}
		defer file.Close()
		config, err := png.DecodeConfig(file)
		if err != nil {
			return nil, err
		}
		if config.Width > editableMaxSheetSide || config.Height > editableMaxSheetSide {
			return nil, fmt.Errorf("PNG sheet exceeds %d pixels", editableMaxSheetSide)
		}
		if _, err = file.Seek(0, 0); err != nil {
			return nil, err
		}
		decoded, err := png.Decode(file)
		if err != nil {
			return nil, err
		}
		sheet = image.NewNRGBA(decoded.Bounds())
		draw.Draw(sheet, sheet.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
		if a.cachedBytes+len(sheet.Pix) > editableMaxCachedSheetBytes {
			a.ClearImageCache()
		}
		a.sheets[f.Sheet] = sheet
		a.cachedBytes += len(sheet.Pix)
	}
	rect := f.Rect.Bounds()
	if !rect.In(sheet.Bounds()) {
		return nil, fmt.Errorf("frame outside PNG sheet: %s", f.Sheet)
	}
	return sheet.SubImage(rect).(*image.NRGBA), nil
}

// NativeSpriteColor reproduces the original renderer's palette key rules at
// export time. Edited PNGs subsequently use normal RGBA, including opaque black.
func NativeSpriteColor(index byte, palette [256][3]uint8) (r, g, b, a byte) {
	if index == 0 {
		return 0, 0, 0, 0
	}
	rgb := palette[index]
	r, g, b = rgb[0], rgb[1], rgb[2]
	if r == 0 && b == 0 && g == 255 || r == 0 && g == 0 && b == 255 {
		return 0, 0, 0, 0
	}
	if r == 0 && g == 0 && b == 0 {
		g = 4
	}
	return r, g, b, 255
}

// ClearImageCache releases cached PNG sheets; frame metadata stays available.
func (a *EditableSprites) ClearImageCache() { a.sheets = map[string]*image.NRGBA{}; a.cachedBytes = 0 }

// Sprite retrieves the stable archive slot, independent of JSON array order.
func (a *EditableSprites) Sprite(index int) *EditableSprite { return a.indexed[index] }
