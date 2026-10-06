package spritepack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"wonderland-gonline/internal/clientassets"
)

const editableAtlasDirectory = "atlas"

type Optimization struct {
	Archive                   string
	BeforeSheets, AfterSheets int
	BeforeBytes, AfterBytes   int64
}

// OptimizeEditable applies the runtime atlas builder to editable sheets. Frame
// metadata and painted colors are retained; only sheet locations change. The
// original directory is replaced after every frame has been compared exactly.
func OptimizeEditable(dir, archive string) (result Optimization, err error) {
	result.Archive = archive
	old, err := clientassets.OpenEditableSprites(filepath.Join(dir, EditableFile))
	if err != nil {
		return result, err
	}
	sheets := map[string]bool{}
	for _, s := range old.Sprites {
		for _, f := range s.Frames {
			if f.Sheet != "" {
				sheets[filepath.FromSlash(f.Sheet)] = true
			}
		}
	}
	for sheet := range sheets {
		st, err := os.Stat(filepath.Join(dir, sheet))
		if err != nil {
			return result, err
		}
		result.BeforeBytes += st.Size()
	}
	result.BeforeSheets = len(sheets)
	temporary, err := os.MkdirTemp(filepath.Dir(dir), ".sprite-optimize-*")
	if err != nil {
		return result, err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(temporary)
		}
	}()
	pack, err := WriteDir(filepath.Join(temporary, "packs"), archive, func(sink PageSink) (*Pack, error) { return BuildEditable(dir, archive, sink) })
	if err != nil {
		return result, err
	}
	prepared := filepath.Join(temporary, "optimized")
	if err = os.Mkdir(prepared, 0755); err != nil {
		return result, err
	}
	// Keep preservation outputs and unrelated files byte-for-byte. Hard links
	// avoid copying gigabytes of unchanged decoded sprite data.
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(prepared, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if sheets[rel] || rel == EditableFile {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: expected regular file", path)
		}
		if err = os.Link(path, target); err == nil {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		closeErr := out.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		return result, err
	}
	if err = os.MkdirAll(filepath.Join(prepared, editableAtlasDirectory), 0755); err != nil {
		return result, err
	}
	for _, page := range pack.Pages {
		source := filepath.Join(temporary, "packs", archive, page)
		target := filepath.Join(prepared, editableAtlasDirectory, page)
		if _, err = os.Stat(target); err == nil {
			return result, fmt.Errorf("unreferenced file conflicts with optimized atlas: %s", target)
		} else if !os.IsNotExist(err) {
			return result, err
		}
		if err = os.Rename(source, target); err != nil {
			return result, err
		}
		st, err := os.Stat(target)
		if err != nil {
			return result, err
		}
		result.AfterBytes += st.Size()
	}
	result.AfterSheets = len(pack.Pages)
	byIndex := map[int]*Sprite{}
	for i := range pack.Sprites {
		byIndex[pack.Sprites[i].Index] = &pack.Sprites[i]
	}
	updated := clientassets.EditableSprites{Version: old.Version, Sprites: append([]clientassets.EditableSprite(nil), old.Sprites...)}
	// Deep-copy frames so old still supplies the pre-optimization pixel oracle.
	for i := range updated.Sprites {
		src := &updated.Sprites[i]
		built := byIndex[src.Index]
		if built == nil || len(built.Frames) != len(src.Frames) {
			return result, fmt.Errorf("sprite frame count changed: %s", src.Name)
		}
		src.Frames = append([]clientassets.EditableFrame(nil), src.Frames...)
		for j := range src.Frames {
			frame := &src.Frames[j]
			packed := built.Frames[j]
			frame.Sheet = ""
			if frame.Rect.Width > 0 && frame.Rect.Height > 0 {
				frame.Sheet = filepath.ToSlash(filepath.Join(editableAtlasDirectory, pack.Pages[packed.Page]))
			}
			frame.Rect.X, frame.Rect.Y = packed.Rect.X, packed.Rect.Y
		}
	}
	raw, err := json.MarshalIndent(updated, "", "  ")
	if err != nil {
		return result, err
	}
	if err = os.WriteFile(filepath.Join(prepared, EditableFile), append(raw, '\n'), 0644); err != nil {
		return result, err
	}
	optimized, err := clientassets.OpenEditableSprites(filepath.Join(prepared, EditableFile))
	if err != nil {
		return result, err
	}
	for i, src := range old.Sprites {
		for j, f := range src.Frames {
			before, err := old.FrameImage(f)
			if err != nil {
				return result, err
			}
			after, err := optimized.FrameImage(optimized.Sprites[i].Frames[j])
			if err != nil {
				return result, err
			}
			if !sameFramePixels(before, after) {
				return result, fmt.Errorf("optimization changed pixels: %s frame %d", src.Name, j)
			}
		}
	}
	if _, err = clientassets.PruneEmptyDirectories(prepared); err != nil {
		return result, err
	}
	backup := filepath.Join(temporary, "previous")
	if err = os.Rename(dir, backup); err != nil {
		return result, err
	}
	if err = os.Rename(prepared, dir); err != nil {
		if restoreErr := os.Rename(backup, dir); restoreErr != nil {
			cleanup = false
			return result, fmt.Errorf("publish failed: %v; restore failed: %v; original remains at %s", err, restoreErr, backup)
		}
		return result, err
	}
	return result, nil
}

// OptimizeEditableAll optimizes selected archives in place. It is a layout
// conversion for derived assets, not an alternative original-source exporter.
func OptimizeEditableAll(root string, archives []string, log func(string, ...any)) error {
	selected := map[string]bool{}
	for _, name := range archives {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			selected[name] = true
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if len(selected) > 0 && !selected[strings.ToLower(entry.Name())] {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(dir, EditableFile)); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		result, err := OptimizeEditable(dir, entry.Name())
		if err != nil {
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if log != nil {
			log("%s: %d -> %d sheets, %d -> %d bytes", entry.Name(), result.BeforeSheets, result.AfterSheets, result.BeforeBytes, result.AfterBytes)
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("no editable sprite archives selected")
	}
	return nil
}

// Subimages retain their atlas coordinates, stride and trailing pixels. Compare
// only each frame's visible rows, independent of its original sheet location.
func sameFramePixels(before, after *image.NRGBA) bool {
	if before.Rect.Dx() != after.Rect.Dx() || before.Rect.Dy() != after.Rect.Dy() {
		return false
	}
	for row := 0; row < before.Rect.Dy(); row++ {
		a := before.PixOffset(before.Rect.Min.X, before.Rect.Min.Y+row)
		b := after.PixOffset(after.Rect.Min.X, after.Rect.Min.Y+row)
		width := before.Rect.Dx() * 4
		if !bytes.Equal(before.Pix[a:a+width], after.Pix[b:b+width]) {
			return false
		}
	}
	return true
}
