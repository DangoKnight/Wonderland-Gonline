package clientassets

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// ImageEntry is a native BMg/JMg directory record: one length byte, 23
// filename bytes, a UInt32 file offset and a UInt32 payload length.
type ImageEntry struct {
	Name         string
	Offset, Size uint32
}
type ImageArchive struct {
	source  io.ReaderAt
	Entries []ImageEntry
}

// ReadImageArchive validates the full directory before exposing any payload.
// Archive names are resource keys, never filesystem paths to extract blindly.
func ReadImageArchive(source io.ReaderAt, size int64) (*ImageArchive, error) {
	var header [2]byte
	if _, err := source.ReadAt(header[:], 0); err != nil {
		return nil, fmt.Errorf("image archive count: %w", err)
	}
	count := int(binary.LittleEndian.Uint16(header[:]))
	end := int64(2 + count*32)
	if count == 0 || end > size {
		return nil, fmt.Errorf("image archive: invalid directory size")
	}
	directory := make([]byte, count*32)
	if _, err := source.ReadAt(directory, 2); err != nil {
		return nil, fmt.Errorf("image archive directory: %w", err)
	}
	a := &ImageArchive{source: source}
	seen := map[string]bool{}
	for i := 0; i < count; i++ {
		r := directory[i*32 : (i+1)*32]
		n := int(r[0])
		if n < 1 || n > 23 {
			return nil, fmt.Errorf("image archive entry %d: invalid name length", i)
		}
		name := string(r[1 : 1+n])
		for _, c := range []byte(name) {
			if c < 32 || c > 126 || c == '/' || c == '\\' {
				return nil, fmt.Errorf("image archive entry %d: invalid resource name", i)
			}
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("image archive: duplicate resource %q", name)
		}
		seen[key] = true
		e := ImageEntry{Name: name, Offset: binary.LittleEndian.Uint32(r[24:]), Size: binary.LittleEndian.Uint32(r[28:])}
		if int64(e.Offset) < end || e.Size == 0 || int64(e.Offset)+int64(e.Size) > size {
			return nil, fmt.Errorf("image archive entry %q: payload outside archive", name)
		}
		a.Entries = append(a.Entries, e)
	}
	return a, nil
}

// Find is a case-insensitive resource lookup, retaining native directory order.
func (a *ImageArchive) Find(name string) (int, bool) {
	for i, e := range a.Entries {
		if strings.EqualFold(e.Name, name) {
			return i, true
		}
	}
	return -1, false
}

// Read returns a bounded, independent payload. The caller owns the open source.
func (a *ImageArchive) Read(index int) ([]byte, error) {
	if index < 0 || index >= len(a.Entries) {
		return nil, fmt.Errorf("image archive: index outside directory")
	}
	e := a.Entries[index]
	if e.Size > 64<<20 {
		return nil, fmt.Errorf("image archive: payload exceeds 64 MiB limit")
	}
	b := make([]byte, int(e.Size))
	if _, err := a.source.ReadAt(b, int64(e.Offset)); err != nil {
		return nil, fmt.Errorf("image archive resource %q: %w", e.Name, err)
	}
	return b, nil
}
