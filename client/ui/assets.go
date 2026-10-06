package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	_ "golang.org/x/image/bmp"
	"wonderland-gonline/internal/clientassets"
)

// Assets loads front-end resources from an extracted client asset tree.
type Assets struct {
	Root     string
	SkinName string // active skin; "white" in the English client
	font     *clientassets.Font
	sprites  map[string]*Sprite
	names    map[string]string // lower-case relative path -> on-disk path
}

func NewAssets(root string) (*Assets, error) {
	raw, err := os.ReadFile(filepath.Join(root, "font", "TATPC1.TWN"))
	if err != nil {
		return nil, err
	}
	f, err := clientassets.DecodeFont(raw)
	if err != nil {
		return nil, err
	}
	a := &Assets{Root: root, SkinName: "white", font: f, sprites: map[string]*Sprite{}, names: map[string]string{}}
	// The original runs on Windows; resolve names case-insensitively.
	for _, dir := range []string{filepath.Join("menu", "Skins", "white"), filepath.Join("menu", "Skins", "default"), "pic"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			continue
		}
		for _, e := range entries {
			a.names[strings.ToLower(filepath.Join(dir, e.Name()))] = filepath.Join(root, dir, e.Name())
		}
	}
	return a, nil
}

func (a *Assets) path(rel string) string {
	if p, ok := a.names[strings.ToLower(rel)]; ok {
		return p
	}
	return filepath.Join(a.Root, rel)
}

// decodeFile decodes JPEGs with the client's IJG-exact decoder and other
// images with the standard library.
func decodeFile(path string) (image.Image, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m image.Image
	if strings.EqualFold(filepath.Ext(path), ".jpg") {
		m, err = clientassets.DecodeJPEG(raw)
	} else {
		m, _, err = image.Decode(bytes.NewReader(raw))
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Skin loads menu/Skins/<skin>/<name>, falling back to the default skin as
// the original's skin loader does. Bitmaps are colour-keyed; JPEGs are opaque.
func (a *Assets) Skin(name string, frames int) (*Sprite, error) {
	key := fmt.Sprintf("skin:%s:%d", name, frames)
	if s, ok := a.sprites[key]; ok {
		return s, nil
	}
	var m image.Image
	var err error
	for _, skin := range []string{a.SkinName, "default"} {
		if m, err = decodeFile(a.path(filepath.Join("menu", "Skins", skin, name))); err == nil {
			break
		}
	}
	if err != nil {
		return nil, err
	}
	s := newSprite(m, frames, filepath.Ext(name) != ".jpg")
	a.sprites[key] = s
	return s, nil
}

// Pic loads an opaque picture from pic/.
func (a *Assets) Pic(name string) (*Sprite, error) {
	key := "pic:" + name
	if s, ok := a.sprites[key]; ok {
		return s, nil
	}
	m, err := decodeFile(a.path(filepath.Join("pic", name)))
	if err != nil {
		return nil, err
	}
	s := newSprite(m, 1, false)
	a.sprites[key] = s
	return s, nil
}

// Text draws Big5 text with the TATPC1 font. (x, y) is the top-left of the
// first 15-pixel glyph cell. It returns the drawn width.
func (a *Assets) Text(dst *image.RGBA, x, y int, text []byte, c color.RGBA) int {
	c = quantize(c)
	cx := x
	for _, g := range a.font.Glyphs(text) {
		for gy := range clientassets.FontHeight {
			for gx := range g.Width {
				if g.Set(gx, gy) && image.Pt(cx+gx, y+gy).In(dst.Rect) {
					dst.SetRGBA(cx+gx, y+gy, c)
				}
			}
		}
		cx += g.Width
	}
	return cx - x
}

// Big5 encodes UTF-8 text for the client font.
func Big5(s string) []byte { return clientassets.Big5Text(s) }
