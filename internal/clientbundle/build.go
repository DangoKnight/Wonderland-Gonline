// Package clientbundle compiles editable client data into a standard ZIP64
// bundle. PNG pages stay lossless; JSON is validated, compacted and deflated.
package clientbundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientimage"
)

const Version = 1
const ManifestName = "_bundle.json"
const maxImageSide = 1 << 16

type Dimensions struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}
type Contract struct {
	Version int                   `json:"version"`
	Images  map[string]Dimensions `json:"images"`
}
type Entry struct {
	SourceSHA256 string `json:"source_sha256"`
	Bytes        int64  `json:"bytes"`
}
type Manifest struct {
	Version int              `json:"version"`
	Files   map[string]Entry `json:"files"`
}
type Options struct {
	Source, Output, Contract string
	group                    string
}
type Result struct {
	Files, Reused int
	SourceBytes   int64
}

func included(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".json", ".wav", ".ogg", ".avi", ".bin", ".wli", ".wlt":
		return true
	}
	return false
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// Build replaces only a complete, validated bundle. Existing compressed entries
// are copied directly when source content has not changed, regardless of mtime.
func Build(o Options) (result Result, err error) {
	contract := Contract{Version: Version, Images: map[string]Dimensions{}}
	if o.Contract != "" {
		raw, e := os.ReadFile(o.Contract)
		if e == nil {
			if e = json.Unmarshal(raw, &contract); e != nil {
				return result, e
			}
			if contract.Version != Version || contract.Images == nil {
				return result, fmt.Errorf("unsupported asset contract")
			}
		} else if !os.IsNotExist(e) {
			return result, e
		}
	}
	files := []string{}
	sizes := map[string]int64{}
	images := map[string]Dimensions{}
	err = filepath.WalkDir(o.Source, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("asset symlinks are unsupported: %s", path)
		}
		if d.IsDir() {
			return nil
		}
		rel, e := filepath.Rel(o.Source, path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		if o.group != "" && strings.HasPrefix(filepath.Base(rel), ".") {
			return nil
		}
		if !included(rel) {
			return nil
		}
		if rel == ManifestName {
			return fmt.Errorf("reserved asset filename %s", rel)
		}
		info, e := d.Info()
		if e != nil {
			return e
		}
		sizes[rel] = info.Size()
		files = append(files, rel)
		if strings.HasSuffix(rel, clientimage.DescriptorSuffix) {
			m, e := clientimage.Open(strings.TrimSuffix(path, clientimage.DescriptorSuffix))
			if e != nil {
				return e
			}
			b := m.Bounds()
			logical := strings.TrimSuffix(rel, clientimage.DescriptorSuffix)
			images[logical] = Dimensions{b.Dx(), b.Dy()}
			if expected, ok := contract.Images[logical]; ok && expected != images[logical] {
				return fmt.Errorf("compiled image size changed: %s", logical)
			}
		}
		if strings.EqualFold(filepath.Ext(rel), ".png") {
			f, e := os.Open(path)
			if e != nil {
				return e
			}
			cfg, e := png.DecodeConfig(f)
			f.Close()
			if e != nil {
				return fmt.Errorf("%s: %w", rel, e)
			}
			dim := Dimensions{cfg.Width, cfg.Height}
			if dim.Width <= 0 || dim.Height <= 0 || dim.Width > maxImageSide || dim.Height > maxImageSide {
				return fmt.Errorf("invalid image dimensions: %s", rel)
			}
			if expected, ok := contract.Images[rel]; ok && expected != dim {
				return fmt.Errorf("%s: expected %dx%d, got %dx%d", rel, expected.Width, expected.Height, dim.Width, dim.Height)
			}
			images[rel] = dim
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	if len(files) == 0 {
		return result, fmt.Errorf("no client assets in %s", o.Source)
	}
	for path := range contract.Images {
		if _, ok := images[path]; !ok {
			return result, fmt.Errorf("required image missing: %s", path)
		}
	}
	if o.group != "" {
		selected := files[:0]
		for _, name := range files {
			if packGroup(name) == o.group {
				selected = append(selected, name)
			}
		}
		files = selected
	}
	sort.Strings(files)
	// Validate editable animation/frame references before touching the output.
	for _, name := range files {
		if filepath.Base(name) != "editable.json" {
			continue
		}
		editable, e := clientassets.OpenEditableSprites(filepath.Join(o.Source, filepath.FromSlash(name)))
		if e != nil {
			return result, fmt.Errorf("%s: %w", name, e)
		}
		for _, sprite := range editable.Sprites {
			for _, frame := range sprite.Frames {
				if frame.Rect.Width == 0 || frame.Rect.Height == 0 {
					continue
				}
				sheet := filepath.ToSlash(filepath.Join(filepath.Dir(name), frame.Sheet))
				dim, ok := images[sheet]
				if !ok || !frame.Rect.Bounds().In(image.Rect(0, 0, dim.Width, dim.Height)) {
					return result, fmt.Errorf("%s: sprite %d frame %s outside sheet %s", name, sprite.Index, frame.Name, sheet)
				}
			}
		}
	}
	previous := map[string]*zip.File{}
	old := Manifest{}
	z, e := zip.OpenReader(o.Output)
	if e == nil {
		defer z.Close()
		for _, f := range z.File {
			previous[f.Name] = f
		}
		if f := previous[ManifestName]; f != nil {
			r, e := f.Open()
			if e == nil {
				_ = json.NewDecoder(r).Decode(&old)
				r.Close()
			}
		}
		if old.Version != Version {
			old.Files = nil
		}
	}
	sourceHashes := map[string]string{}
	unchanged := z != nil && len(old.Files) == len(files)
	for _, name := range files {
		hash, e := hashFile(filepath.Join(o.Source, filepath.FromSlash(name)))
		if e != nil {
			return result, e
		}
		sourceHashes[name] = hash
		result.SourceBytes += sizes[name]
		f := previous[name]
		if f == nil || old.Files[name].SourceSHA256 != hash {
			unchanged = false
			continue
		}
		// Validate local header offsets before trusting an old raw entry. This also
		// recognizes bundles produced by the earlier ZIP64-copy implementation.
		if _, e = f.OpenRaw(); e != nil {
			delete(previous, name)
			unchanged = false
		}
	}
	if unchanged {
		result.Files = len(files)
		result.Reused = len(files)
		return result, nil
	}
	if err = os.MkdirAll(filepath.Dir(o.Output), 0o755); err != nil {
		return result, err
	}
	out, err := os.CreateTemp(filepath.Dir(o.Output), ".client-assets-*.tmp")
	if err != nil {
		return result, err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	defer out.Close()
	writer := zip.NewWriter(out)
	defer writer.Close()
	manifest := Manifest{Version: Version, Files: map[string]Entry{}}
	for _, name := range files {
		path := filepath.Join(o.Source, filepath.FromSlash(name))
		sourceHash := sourceHashes[name]
		var e error
		if prev, ok := old.Files[name]; ok && prev.SourceSHA256 == sourceHash && previous[name] != nil {
			copyFile := *previous[name]
			copyFile.Extra = withoutZIP64(copyFile.Extra)
			if e = writer.Copy(&copyFile); e != nil {
				return result, e
			}
			manifest.Files[name] = prev
			result.Reused++
			continue
		}
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetMode(0o644)
		var data []byte
		if strings.EqualFold(filepath.Ext(name), ".bin") || strings.HasSuffix(name, clientimage.DescriptorSuffix) {
			header.Method = zip.Deflate
		}
		if strings.EqualFold(filepath.Ext(name), ".json") {
			raw, e := os.ReadFile(path)
			if e != nil {
				return result, e
			}
			var compact bytes.Buffer
			if e = json.Compact(&compact, raw); e != nil {
				return result, fmt.Errorf("%s: %w", name, e)
			}
			data = compact.Bytes()
			header.Method = zip.Deflate
			if filepath.Base(name) == "manifest.json" {
				if e = validateReferences(name, data, images); e != nil {
					return result, e
				}
			}
		}
		entry, e := writer.CreateHeader(header)
		if e != nil {
			return result, e
		}
		count := int64(len(data))
		if data != nil {
			_, e = entry.Write(data)
		} else {
			f, openErr := os.Open(path)
			e = openErr
			if e == nil {
				count, e = io.Copy(entry, f)
				f.Close()
			}
		}
		if e != nil {
			return result, e
		}
		// Reject a source edited while it was being compiled.
		current, e := hashFile(path)
		if e != nil {
			return result, e
		}
		if current != sourceHash {
			return result, fmt.Errorf("asset changed during build: %s; rebuild", name)
		}
		manifest.Files[name] = Entry{sourceHash, count}
	}
	result.Files = len(files)
	raw, err := json.Marshal(manifest)
	if err != nil {
		return result, err
	}
	entry, err := writer.Create(ManifestName)
	if err != nil {
		return result, err
	}
	if _, err = entry.Write(raw); err != nil {
		return result, err
	}
	if err = writer.Close(); err != nil {
		return result, err
	}
	if err = out.Sync(); err != nil {
		return result, err
	}
	if err = out.Close(); err != nil {
		return result, err
	}
	if z != nil {
		if err = z.Close(); err != nil {
			return result, err
		}
	}
	contract.Images = images
	if o.Contract != "" {
		raw, err = json.MarshalIndent(contract, "", "  ")
		if err != nil {
			return result, err
		}
		if err = writeAtomic(o.Contract, append(raw, '\n')); err != nil {
			return result, err
		}
	}
	if err = os.Rename(tmp, o.Output); err != nil {
		return result, err
	}
	return result, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".asset-contract-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// Exported manifests locate their PNGs relative to their own directory. Logical
// names and provenance paths are deliberately not treated as physical payloads.
func validateReferences(name string, data []byte, images map[string]Dimensions) error {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	var walk func(any) error
	walk = func(value any) error {
		switch v := value.(type) {
		case []any:
			for _, x := range v {
				if err := walk(x); err != nil {
					return err
				}
			}
		case map[string]any:
			if png, ok := v["png"].(string); ok && png != "" {
				if !fs.ValidPath(png) || strings.ContainsAny(png, "\\:") {
					return fmt.Errorf("%s: unsafe PNG reference %q", name, png)
				}
				path := filepath.ToSlash(filepath.Join(filepath.Dir(name), png))
				dim, ok := images[path]
				if !ok {
					return fmt.Errorf("%s: missing PNG %s", name, png)
				}
				if r, ok := v["rect"].(map[string]any); ok {
					number := func(k string) (int, error) {
						f, ok := r[k].(float64)
						if !ok || float64(int(f)) != f || f < 0 {
							return 0, fmt.Errorf("%s: invalid rectangle", name)
						}
						return int(f), nil
					}
					x, e := number("x")
					if e != nil {
						return e
					}
					y, e := number("y")
					if e != nil {
						return e
					}
					w, e := number("width")
					if e != nil {
						return e
					}
					h, e := number("height")
					if e != nil {
						return e
					}
					if w <= 0 || h <= 0 || !image.Rect(x, y, x+w, y+h).In(image.Rect(0, 0, dim.Width, dim.Height)) {
						return fmt.Errorf("%s: rectangle outside %s", name, png)
					}
				}
			}
			for _, x := range v {
				if err := walk(x); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(doc)
}

// The central ZIP64 offset belongs to the previous bundle. Go's raw Copy keeps
// Extra verbatim and appends the new offset, so strip the old ZIP64 field first.
// Sizes remain in FileHeader's 64-bit members; Writer generates current values.
const zip64ExtraID = 0x0001
const zipExtraHeaderBytes = 4

func withoutZIP64(extra []byte) []byte {
	out := []byte{}
	for len(extra) >= zipExtraHeaderBytes {
		size := zipExtraHeaderBytes + int(binary.LittleEndian.Uint16(extra[2:]))
		if size > len(extra) {
			break
		}
		if binary.LittleEndian.Uint16(extra) != zip64ExtraID {
			out = append(out, extra[:size]...)
		}
		extra = extra[size:]
	}
	return out
}
