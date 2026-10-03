package spritepack

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
)

// WriteDir builds a pack into dir/archive. Pages and pack.json are written
// to a temporary directory first; an existing pack directory is replaced
// only after the new one is complete. A non-pack directory is never
// replaced.
func WriteDir(dir, archive string, build func(PageSink) (*Pack, error)) (*Pack, error) {
	final := filepath.Join(dir, archive)
	if entries, err := os.ReadDir(final); err == nil && len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(final, FileName)); err != nil {
			return nil, fmt.Errorf("%s exists and is not a sprite pack", final)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(dir, "."+archive+".tmp-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	write := func(name string, img image.Image) error {
		f, err := os.Create(filepath.Join(tmp, name))
		if err != nil {
			return err
		}
		enc := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err = enc.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}
	sink := func(p Page) error {
		if err := write(PageName(p.Index), p.RGBA); err != nil {
			return err
		}
		if p.Indices != nil {
			return write(IndexPageName(p.Index), p.Indices)
		}
		return nil
	}
	p, err := build(sink)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(tmp, FileName), data, 0o644); err != nil {
		return nil, err
	}
	if err = os.RemoveAll(final); err != nil {
		return nil, err
	}
	if err = os.Rename(tmp, final); err != nil {
		return nil, err
	}
	return p, nil
}
