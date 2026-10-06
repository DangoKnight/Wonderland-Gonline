package clientassets

import (
	"container/list"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
)

// ReadPicturePNG returns an independent image, normalized to origin (0,0).
func ReadPicturePNG(path string, rect *SpriteRect) (*image.NRGBA, error) {
	if compiled, err := clientimage.Open(path); err == nil {
		region := compiled.Bounds()
		if rect != nil {
			region = rect.Bounds()
			if region.Empty() || !region.In(compiled.Bounds()) {
				return nil, fmt.Errorf("picture rectangle exceeds compiled image: %s", path)
			}
		}
		return compiled.NRGBA(region)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	f, err := clientfs.Open(path)
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
func pictureSource(root, logical string) (string, *SpriteRect, error) {
	pictureCatalogs.Lock()
	if pictureCatalogs.ByRoot == nil {
		pictureCatalogs.ByRoot = map[string]map[string]pictureReference{}
	}
	catalog, ok := pictureCatalogs.ByRoot[root]
	if !ok {
		raw, err := clientfs.ReadFile(filepath.Join(root, "manifest.json"))
		if err != nil {
			pictureCatalogs.Unlock()
			if !os.IsNotExist(err) {
				return "", nil, err
			}
			return filepath.Join(root, filepath.FromSlash(logical)), nil, nil
		}
		var doc struct {
			Files []struct {
				Pictures []pictureReference `json:"pictures"`
			} `json:"files"`
		}
		if err = json.Unmarshal(raw, &doc); err != nil {
			pictureCatalogs.Unlock()
			return "", nil, err
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
		return "", nil, os.ErrNotExist
	}
	clean := filepath.Clean(filepath.FromSlash(p.PNG))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("unsafe picture path")
	}
	return filepath.Join(root, clean), p.Rect, nil
}

// CompiledPicture resolves an existing logical name to a lightweight image view.
func CompiledPicture(root, logical string) (*clientimage.Image, error) {
	path, rect, err := pictureSource(root, logical)
	if err != nil {
		return nil, err
	}
	m, err := clientimage.Open(path)
	if err != nil {
		return nil, err
	}
	if rect != nil {
		return m.SubImage(rect.Bounds())
	}
	return m, nil
}
func LoadPicture(root, logical string) (*image.NRGBA, error) {
	path, rect, err := pictureSource(root, logical)
	if err != nil {
		return nil, err
	}
	if compiled, err := clientimage.Open(path); err == nil {
		region := compiled.Bounds()
		if rect != nil {
			region = rect.Bounds()
			if region.Empty() || !region.In(compiled.Bounds()) {
				return nil, fmt.Errorf("picture rectangle exceeds compiled image: %s", path)
			}
		}
		return compiled.NRGBA(region)
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if rect == nil {
		return ReadPicturePNG(path, nil)
	}
	page, err := atlasPage(path)
	if err != nil {
		return nil, err
	}
	return cropPicture(page, path, rect)
}

// Fallback editable PNG pages use the same byte-bounded LRU policy. Compiled
// images read only intersecting tiles and do not enter this whole-page cache.
const pictureAtlasCacheBytes = 64 << 20

type atlasCacheEntry struct {
	path  string
	image image.Image
	bytes int
}

var atlasPages = struct {
	sync.Mutex
	entries map[string]*list.Element
	order   *list.List
	bytes   int
}{entries: map[string]*list.Element{}, order: list.New()}

func imageBytes(m image.Image) int {
	switch p := m.(type) {
	case *image.NRGBA:
		return len(p.Pix)
	case *image.RGBA:
		return len(p.Pix)
	case *image.Paletted:
		return len(p.Pix) + len(p.Palette)*4
	case *image.Gray:
		return len(p.Pix)
	case *image.NRGBA64:
		return len(p.Pix)
	case *image.RGBA64:
		return len(p.Pix)
	default:
		return m.Bounds().Dx() * m.Bounds().Dy() * 8
	}
}
func atlasPage(path string) (image.Image, error) {
	atlasPages.Lock()
	defer atlasPages.Unlock()
	if entry := atlasPages.entries[path]; entry != nil {
		atlasPages.order.MoveToFront(entry)
		return entry.Value.(atlasCacheEntry).image, nil
	}
	f, err := clientfs.Open(path)
	if err != nil {
		return nil, err
	}
	m, err := png.Decode(f)
	f.Close()
	if err != nil {
		return nil, err
	}
	size := imageBytes(m)
	if size <= pictureAtlasCacheBytes {
		for atlasPages.bytes+size > pictureAtlasCacheBytes && atlasPages.order.Len() > 0 {
			e := atlasPages.order.Back()
			p := e.Value.(atlasCacheEntry)
			atlasPages.bytes -= p.bytes
			delete(atlasPages.entries, p.path)
			atlasPages.order.Remove(e)
		}
		atlasPages.entries[path] = atlasPages.order.PushFront(atlasCacheEntry{path, m, size})
		atlasPages.bytes += size
	}
	return m, nil
}
