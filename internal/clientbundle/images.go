package clientbundle

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"wonderland-gonline/internal/clientimage"
)

// Image source validation runs before publishing any pack. The same contract is
// used for both source PNGs and compiled descriptors; data/ is never modified.
func (c *runtimeCompiler) validateImages(o Options) error {
	contract := Contract{Version: Version, Images: map[string]Dimensions{}}
	if o.Contract != "" {
		raw, err := os.ReadFile(o.Contract)
		if err == nil {
			if err = json.Unmarshal(raw, &contract); err != nil {
				return err
			}
			if contract.Version != Version || contract.Images == nil {
				return fmt.Errorf("unsupported asset contract")
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	images := map[string]Dimensions{}
	for _, name := range c.files {
		if !strings.EqualFold(filepath.Ext(name), ".png") {
			continue
		}
		f, err := os.Open(filepath.Join(c.source, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		cfg, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		dim := Dimensions{cfg.Width, cfg.Height}
		if dim.Width <= 0 || dim.Height <= 0 || dim.Width > maxImageSide || dim.Height > maxImageSide {
			return fmt.Errorf("invalid image dimensions: %s", name)
		}
		if expected, ok := contract.Images[name]; ok && expected != dim {
			return fmt.Errorf("%s: expected %dx%d, got %dx%d", name, expected.Width, expected.Height, dim.Width, dim.Height)
		}
		images[name] = dim
	}
	for name := range contract.Images {
		if _, ok := images[name]; !ok {
			return fmt.Errorf("required image missing: %s", name)
		}
	}
	for _, name := range c.files {
		if filepath.Base(name) != "manifest.json" {
			continue
		}
		if !(strings.HasPrefix(name, "media/") || strings.HasPrefix(name, "pictures/")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.source, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		if err = validateReferences(name, data, images); err != nil {
			return err
		}
	}
	c.imageDimensions = images
	return nil
}

func trimEmpty(m *image.NRGBA) image.Rectangle {
	b := m.Bounds()
	left, top, right, bottom := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			p := m.NRGBAAt(x, y)
			if p.R != 0 || p.G != 0 || p.B != 0 || p.A != 0 {
				left = min(left, x)
				top = min(top, y)
				right = max(right, x+1)
				bottom = max(bottom, y+1)
			}
		}
	}
	if right <= left || bottom <= top {
		return image.Rectangle{}
	}
	return image.Rect(left, top, right, bottom)
}
func normalized(m image.Image) *image.NRGBA {
	b := m.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(n, n.Bounds(), m, b.Min, draw.Src)
	return n
}

type pendingImage struct {
	image *image.NRGBA
	trim  image.Rectangle
	index int
}
type pagePlacement struct {
	descriptor int
	rect, page image.Rectangle
}

// compileImages groups small images by their source directory. Larger images
// remain independent tiled canvases, rather than decoding one enormous atlas.
func (c *runtimeCompiler) compileImages() error {
	groups := map[string][]string{}
	for _, name := range c.files {
		if !strings.HasSuffix(strings.ToLower(name), ".png") || !(strings.HasPrefix(name, "media/") || strings.HasPrefix(name, "pictures/")) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(name))
		groups[dir] = append(groups[dir], name)
	}
	dirs := make([]string, 0, len(groups))
	for dir := range groups {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	for _, dir := range dirs {
		names := groups[dir]
		group := "core"
		if strings.HasPrefix(dir, "pictures/") {
			group = "world"
		}
		// Upgrade existing directory caches without decoding every background again.
		// Subsequent edits invalidate only that image; small images still share pages.
		legacy := c.old["images:"+dir]
		signature, err := c.signature(names)
		if err != nil {
			return err
		}
		warm := legacy.Signature == signature
		small := []string{}
		for _, name := range names {
			dim := c.imageDimensions[name]
			if dim.Width <= clientimage.SmallImageSide && dim.Height <= clientimage.SmallImageSide {
				small = append(small, name)
				continue
			}
			key := "image:" + name
			if warm {
				c.warmImageCache(key, []string{name}, legacy)
			}
			if err := c.optimizeImageCache(key, []string{name}); err != nil {
				return err
			}
			if err := c.compileImageGroup(key, []string{name}, group); err != nil {
				return err
			}
		}
		if len(small) > 0 {
			key := "images-small:" + dir
			if warm {
				c.warmImageCache(key, small, legacy)
			}
			if err := c.optimizeImageCache(key, small); err != nil {
				return err
			}
			if err := c.compileImageGroup(key, small, group); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *runtimeCompiler) warmImageCache(key string, names []string, legacy runtimeCacheEntry) {
	if _, ok := c.old[key]; ok {
		return
	}
	signature, err := c.signature(names)
	if err != nil {
		return
	}
	outputs := map[string]string{}
	for _, name := range names {
		path := name + clientimage.DescriptorSuffix
		sum, ok := legacy.Outputs[path]
		if !ok {
			return
		}
		outputs[path] = sum
		m, err := clientimage.Open(filepath.Join(c.root, filepath.FromSlash(name)))
		if err != nil {
			return
		}
		for _, file := range m.Files() {
			sum, ok := legacy.Outputs[file]
			if !ok {
				return
			}
			outputs[file] = sum
		}
	}
	c.old[key] = runtimeCacheEntry{Signature: signature, Outputs: outputs}
}
func (c *runtimeCompiler) compileImageGroup(key string, names []string, group string) error {
	return c.generateBytes(key, names, func(write func(string, []byte) error) error {
		// Decode sequentially: keep only small pictures and one source canvas in RAM.
		descriptors := make([]clientimage.Descriptor, len(names))
		small := []pendingImage{}
		for i, name := range names {
			f, err := os.Open(filepath.Join(c.source, filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			img, err := png.Decode(f)
			f.Close()
			if err != nil {
				return err
			}
			m := normalized(img)
			d := clientimage.Descriptor{Version: clientimage.Version, Source: name, Width: m.Bounds().Dx(), Height: m.Bounds().Dy()}
			descriptors[i] = d
			trim := trimEmpty(m)
			if trim.Empty() {
				continue
			}
			if trim.Dx() <= clientimage.SmallImageSide && trim.Dy() <= clientimage.SmallImageSide {
				small = append(small, pendingImage{normalized(m.SubImage(trim)), trim, i})
				continue
			}
			for y := trim.Min.Y; y < trim.Max.Y; y += clientimage.TileSide {
				for x := trim.Min.X; x < trim.Max.X; x += clientimage.TileSide {
					rect := image.Rect(x, y, min(x+clientimage.TileSide, trim.Max.X), min(y+clientimage.TileSide, trim.Max.Y))
					sub := m.SubImage(rect).(*image.NRGBA)
					if trimEmpty(sub).Empty() {
						continue
					}
					data, err := clientimage.EncodeChunk(sub)
					if err != nil {
						return err
					}
					file := clientimage.ChunkPath(group, data)
					if err = write(file, data); err != nil {
						return err
					}
					descriptors[i].Tiles = append(descriptors[i].Tiles, clientimage.Tile{Rect: rect, Page: image.Rect(0, 0, rect.Dx(), rect.Dy()), File: file})
				}
			}
		}
		// Taller entries first makes shelf packing stable and reduces page padding.
		sort.SliceStable(small, func(i, j int) bool { return small[i].trim.Dy() > small[j].trim.Dy() })
		page := image.NewNRGBA(image.Rect(0, 0, clientimage.TileSide, clientimage.TileSide))
		placements := []pagePlacement{}
		x, y, rowHeight, usedWidth := 0, 0, 0, 0
		unique := map[string]pagePlacement{}
		flush := func() error {
			if len(placements) == 0 {
				return nil
			}
			data, err := clientimage.EncodeChunk(page.SubImage(image.Rect(0, 0, usedWidth, y+rowHeight)).(*image.NRGBA))
			if err != nil {
				return err
			}
			file := clientimage.ChunkPath(group, data)
			if err = write(file, data); err != nil {
				return err
			}
			for _, p := range placements {
				descriptors[p.descriptor].Tiles = append(descriptors[p.descriptor].Tiles, clientimage.Tile{Rect: p.rect, Page: p.page, File: file})
			}
			page = image.NewNRGBA(image.Rect(0, 0, clientimage.TileSide, clientimage.TileSide))
			placements = nil
			x, y, rowHeight, usedWidth = 0, 0, 0, 0
			unique = map[string]pagePlacement{}
			return nil
		}
		for _, s := range small {
			w, h := s.trim.Dx(), s.trim.Dy()
			var exact []byte
			for row := 0; row < h; row++ {
				exact = append(exact, s.image.Pix[row*s.image.Stride:row*s.image.Stride+w*4]...)
			}
			signature := fmt.Sprintf("%dx%d:%s", w, h, clientimage.Hash(exact))
			if p, ok := unique[signature]; ok {
				placements = append(placements, pagePlacement{s.index, s.trim, p.page})
				continue
			}
			if x+w > clientimage.TileSide {
				x = 0
				y += rowHeight
				rowHeight = 0
			}
			if y+h > clientimage.TileSide {
				if err := flush(); err != nil {
					return err
				}
			}
			rect := image.Rect(x, y, x+w, y+h)
			draw.Draw(page, rect, s.image, s.image.Bounds().Min, draw.Src)
			p := pagePlacement{s.index, s.trim, rect}
			placements = append(placements, p)
			unique[signature] = p
			x += w
			rowHeight = max(rowHeight, h)
			usedWidth = max(usedWidth, x)
		}
		if err := flush(); err != nil {
			return err
		}
		for _, d := range descriptors {
			data, err := clientimage.EncodeDescriptor(d)
			if err != nil {
				return err
			}
			if err = write(d.Source+clientimage.DescriptorSuffix, data); err != nil {
				return err
			}
		}
		return nil
	})
}

type optimizedImagePage struct{ file, sum string }

// Cached source pixels can be recompressed without re-decoding whole PNG maps.
// New addresses are published only with a complete set of rewritten descriptors.
func (c *runtimeCompiler) optimizeImageCache(key string, names []string) error {
	old, ok := c.old[key]
	if !ok || old.ImageCodec >= compiledImageCodecVersion {
		return nil
	}
	signature, err := c.signature(names)
	if err != nil {
		return err
	}
	if signature != old.Signature {
		return nil
	}
	for name, sum := range old.Outputs {
		if actual, err := hashFile(filepath.Join(c.root, filepath.FromSlash(name))); err != nil || actual != sum {
			return nil
		}
	}
	files := map[string]string{}
	outputs := map[string]string{}
	sorted := make([]string, 0, len(old.Outputs))
	for name := range old.Outputs {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		if !strings.HasSuffix(name, clientimage.ChunkSuffix) {
			continue
		}
		if c.recompressed == nil {
			c.recompressed = map[string]optimizedImagePage{}
		}
		if page, ok := c.recompressed[name]; ok {
			files[name] = page.file
			outputs[page.file] = page.sum
			continue
		}
		raw, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		optimized, err := clientimage.Optimize(raw)
		if err != nil {
			return err
		}
		group := "world"
		if strings.HasPrefix(name, "runtime/images/core/") {
			group = "core"
		}
		next := clientimage.ChunkPath(group, optimized)
		files[name] = next
		outputs[next] = digest(optimized)
		c.recompressed[name] = optimizedImagePage{file: next, sum: outputs[next]}
		if next != name {
			if err = writeAtomic(filepath.Join(c.root, filepath.FromSlash(next)), optimized); err != nil {
				return err
			}
		}
	}
	for _, name := range sorted {
		if !strings.HasSuffix(name, clientimage.DescriptorSuffix) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		data, err := clientimage.RemapDescriptor(raw, files)
		if err != nil {
			return err
		}
		if !bytes.Equal(raw, data) {
			if err = writeAtomic(filepath.Join(c.root, filepath.FromSlash(name)), data); err != nil {
				return err
			}
		}
		outputs[name] = digest(data)
	}
	c.old[key] = runtimeCacheEntry{Signature: signature, ImageCodec: compiledImageCodecVersion, Outputs: outputs}
	return nil
}
