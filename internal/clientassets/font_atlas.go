package clientassets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"wonderland-gonline/internal/clientfs"
)

const fontAtlasASCIICharacters = 256
const fontAtlasNarrowWidth = 8
const fontAtlasWideWidth = 16
const fontAtlasOpaqueThreshold = 0x8000

// LoadFontAtlas reads the editable PNG export of TATPC1.TWN. Opaque pixels
// become set glyph bits; original encrypted font bytes are not required.
func LoadFontAtlas(directory string) (*Font, error) {
	var doc struct {
		Format string `json:"format"`
		Sheets []struct {
			Group   string `json:"group"`
			PNG     string `json:"png"`
			Count   int    `json:"count"`
			Columns int    `json:"columns"`
			Width   int    `json:"width"`
			Height  int    `json:"height"`
		} `json:"sheets"`
	}
	raw, err := clientfs.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if doc.Format != "bitmap-font" || len(doc.Sheets) != 2 {
		return nil, fmt.Errorf("invalid bitmap font manifest")
	}
	data := make([]byte, fontASCIISize+fontBig5Count*fontWideSize)
	seen := map[string]bool{}
	for _, sheet := range doc.Sheets {
		offset, count, width, rowBytes := 0, fontAtlasASCIICharacters, fontAtlasNarrowWidth, 1
		if sheet.Group == "wide" {
			offset, count, width, rowBytes = fontASCIISize, fontBig5Count, fontAtlasWideWidth, 2
		} else if sheet.Group != "ascii" {
			return nil, fmt.Errorf("unknown font sheet %q", sheet.Group)
		}
		if seen[sheet.Group] || sheet.Count != count || sheet.Width != width || sheet.Height != FontHeight || sheet.Columns <= 0 || filepath.Base(sheet.PNG) != sheet.PNG {
			return nil, fmt.Errorf("invalid %s font sheet", sheet.Group)
		}
		seen[sheet.Group] = true
		img, err := ReadPicturePNG(filepath.Join(directory, sheet.PNG), nil)
		if err != nil {
			return nil, err
		}
		bounds := img.Bounds()
		if bounds.Dx() != sheet.Columns*width || bounds.Dy() != ((count+sheet.Columns-1)/sheet.Columns)*FontHeight {
			return nil, fmt.Errorf("invalid font atlas dimensions")
		}
		for glyph := 0; glyph < count; glyph++ {
			x, y := glyph%sheet.Columns*width, glyph/sheet.Columns*FontHeight
			for row := 0; row < FontHeight; row++ {
				for column := 0; column < width; column++ {
					_, _, _, alpha := img.At(x+column, y+row).RGBA()
					if alpha >= fontAtlasOpaqueThreshold {
						data[offset+(glyph*FontHeight+row)*rowBytes+column/8] |= 0x80 >> (column % 8)
					}
				}
			}
		}
	}
	return &Font{data: data}, nil
}
