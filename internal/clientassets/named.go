package clientassets

import (
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// Named archives (Ground.MMG, Wem.MMG; aLogin FUN_0010f154) end with an
// index of 29-byte rows and a UInt16 row count. A row is a name length,
// up to 20 name bytes, then the payload's UInt32 offset and length.
// Payloads are stored in index order and cover the file up to the index.
const (
	namedIndexRowBytes = 29
	namedNameBytes     = 21
	namedCountBytes    = 2
)

type NamedArchive struct {
	source  io.ReaderAt
	Entries []ImageEntry
	byName  map[string]int
}

// ReadNamedArchive validates the index before exposing any payload.
func ReadNamedArchive(source io.ReaderAt, size int64) (*NamedArchive, error) {
	if size < namedCountBytes {
		return nil, fmt.Errorf("named archive: truncated")
	}
	var tail [namedCountBytes]byte
	if _, err := source.ReadAt(tail[:], size-namedCountBytes); err != nil {
		return nil, err
	}
	count := int(binary.LittleEndian.Uint16(tail[:]))
	start := size - namedCountBytes - int64(count*namedIndexRowBytes)
	if count == 0 || start < 0 {
		return nil, fmt.Errorf("named archive: invalid count")
	}
	index := make([]byte, count*namedIndexRowBytes)
	if _, err := source.ReadAt(index, start); err != nil {
		return nil, err
	}
	a := &NamedArchive{source: source, byName: map[string]int{}}
	var next int64
	for i := 0; i < count; i++ {
		row := index[i*namedIndexRowBytes:]
		n := int(row[0])
		if n >= namedNameBytes {
			return nil, fmt.Errorf("named archive entry %d: name too long", i)
		}
		e := ImageEntry{Name: string(row[1 : 1+n]),
			Offset: binary.LittleEndian.Uint32(row[namedNameBytes:]),
			Size:   binary.LittleEndian.Uint32(row[namedNameBytes+4:])}
		if int64(e.Offset) != next || e.Size == 0 || int64(e.Offset)+int64(e.Size) > start {
			return nil, fmt.Errorf("named archive entry %q: payload out of order", e.Name)
		}
		next = int64(e.Offset) + int64(e.Size)
		a.byName[strings.ToLower(e.Name)] = i
		a.Entries = append(a.Entries, e)
	}
	if next != start {
		return nil, fmt.Errorf("named archive: unindexed payload")
	}
	return a, nil
}

// Find returns an entry by name, case-insensitively.
func (a *NamedArchive) Find(name string) (int, bool) {
	i, ok := a.byName[strings.ToLower(name)]
	return i, ok
}

// Read returns an entry's bytes.
func (a *NamedArchive) Read(i int) ([]byte, error) {
	if i < 0 || i >= len(a.Entries) {
		return nil, fmt.Errorf("named archive: no entry %d", i)
	}
	e := a.Entries[i]
	b := make([]byte, e.Size)
	_, err := a.source.ReadAt(b, int64(e.Offset))
	return b, err
}
