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
	"sync"
)

// ReadPicturePNG returns an independent image, normalized to origin (0,0).
func ReadPicturePNG(path string, rect *SpriteRect) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	m, err := png.Decode(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	return cropPicture(m, path, rect)
}

// cropPicture copies rect (or the whole image) to a new image at (0,0).
func cropPicture(m image.Image, path string, rect *SpriteRect) (*image.NRGBA, error) {
	bounds := m.Bounds()
	if rect != nil {
		bounds = image.Rect(rect.X, rect.Y, rect.X+rect.Width, rect.Y+rect.Height)
		if rect.Width <= 0 || rect.Height <= 0 || !bounds.In(m.Bounds()) {
			return nil, fmt.Errorf("picture rectangle exceeds PNG: %s", path)
		}
	}
	result := image.NewNRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(result, result.Bounds(), m, bounds.Min, draw.Src)
	return result, nil
}

type pictureReference struct {
	PNG         string      `json:"png"`
	LogicalPath string      `json:"logical_path"`
	Rect        *SpriteRect `json:"rect"`
}

var pictureCatalogs struct {
	sync.Mutex
	ByRoot map[string]map[string]pictureReference
}

// LoadPicture resolves the original logical PNG path through atlas metadata.
func LoadPicture(root, logical string) (*image.NRGBA, error) {
	pictureCatalogs.Lock()
	if pictureCatalogs.ByRoot == nil {
		pictureCatalogs.ByRoot = map[string]map[string]pictureReference{}
	}
	catalog, ok := pictureCatalogs.ByRoot[root]
	if !ok {
		raw, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		if err != nil {
			pictureCatalogs.Unlock()
			if !os.IsNotExist(err) {
				return nil, err
			}
			return ReadPicturePNG(filepath.Join(root, filepath.FromSlash(logical)), nil)
		}
		var doc struct {
			Files []struct {
				Pictures []pictureReference `json:"pictures"`
			} `json:"files"`
		}
		if err = json.Unmarshal(raw, &doc); err != nil {
			pictureCatalogs.Unlock()
			return nil, err
		}
		catalog = map[string]pictureReference{}
		for _, file := range doc.Files {
			for _, p := range file.Pictures {
				key := p.LogicalPath
				if key == "" {
					key = p.PNG
				}
				catalog[strings.ToLower(key)] = p
			}
		}
		pictureCatalogs.ByRoot[root] = catalog
	}
	p, ok := catalog[strings.ToLower(filepath.ToSlash(logical))]
	pictureCatalogs.Unlock()
	if !ok {
		return nil, os.ErrNotExist
	}
	clean := filepath.Clean(filepath.FromSlash(p.PNG))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("unsafe picture path")
	}
	if p.Rect == nil {
		return ReadPicturePNG(filepath.Join(root, clean), nil)
	}
	page, err := atlasPage(filepath.Join(root, clean))
	if err != nil {
		return nil, err
	}
	return cropPicture(page, filepath.Join(root, clean), p.Rect)
}

// atlasPages keeps the last few decoded atlas pages: neighbouring
// pictures (a map's objects, a form's parts) usually share one, and a
// 2048×2048 page takes far longer to decode than to crop.
var atlasPages struct {
	sync.Mutex
	paths []string
	pages []image.Image
}

const atlasPagesKept = 4

func atlasPage(path string) (image.Image, error) {
	atlasPages.Lock()
	defer atlasPages.Unlock()
	for i, p := range atlasPages.paths {
		if p == path {
			return atlasPages.pages[i], nil
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	m, err := png.Decode(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	atlasPages.paths = append(atlasPages.paths, path)
	atlasPages.pages = append(atlasPages.pages, m)
	if len(atlasPages.paths) > atlasPagesKept {
		atlasPages.paths, atlasPages.pages = atlasPages.paths[1:], atlasPages.pages[1:]
	}
	return m, nil
}
