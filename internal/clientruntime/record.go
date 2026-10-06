// Package clientruntime defines versioned, build-generated client records.
// Editable JSON/PNG sources remain separate from these disposable runtime files.
package clientruntime

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io"
	"path/filepath"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/clientfs"
)

const Version = 1
const maxRecordBytes = 64 << 20
const recordMagic = "WLRT\x01\x00\x00\x00"
const GroundDir = "runtime/maps"
const EventDir = "runtime/events"
const EventIndex = "runtime/events/index.bin"
const ObjectsFile = "runtime/objects.bin"

type Ground struct {
	Name    string  `json:"name"`
	Terrain Terrain `json:"terrain"`
}
type Terrain struct {
	Width      uint32 `json:"width"`
	Height     uint32 `json:"height"`
	Layers     []clientassets.GroundLayer
	GridWidth  uint16   `json:"grid_width"`
	GridHeight uint16   `json:"grid_height"`
	CellsHex   string   `json:"cells_hex,omitempty"`
	Cells      []byte   `json:"-"`
	Zones      []Zone   `json:"unknown_triples"`
	Objects    []Object `json:"objects"`
}
type Zone struct {
	X      uint16 `json:"unknown_u16_0"`
	Y      uint16 `json:"unknown_u16_1"`
	Packed uint16 `json:"unknown_u16_2"`
}
type Object struct {
	Resource uint32 `json:"resource"`
	X        uint16 `json:"x"`
	Y        uint16 `json:"y"`
}

func MapPath(id uint16) string   { return filepath.Join(GroundDir, fmt.Sprintf("%d.bin", id)) }
func EventPath(id uint16) string { return filepath.Join(EventDir, fmt.Sprintf("%d.bin", id)) }

func Encode(v any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(recordMagic)
	err := gob.NewEncoder(&b).Encode(v)
	return b.Bytes(), err
}
func Read(path string, v any) error {
	f, err := clientfs.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Decode(f, v)
}
func Decode(r io.Reader, v any) error {
	var header [len(recordMagic)]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return err
	}
	if string(header[:]) != recordMagic {
		return fmt.Errorf("unsupported client runtime record")
	}
	return gob.NewDecoder(io.LimitReader(r, maxRecordBytes)).Decode(v)
}
