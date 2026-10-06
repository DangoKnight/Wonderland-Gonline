// Package clientassets decodes verified native client resource structures.
// It does not invent image or map-index semantics for undocumented bytes.
package clientassets

import (
	"fmt"
	"wonderland-gonline/internal/protocol"
)

// GroundPrefix is the initial terrain block read by aLogin FUN_004121a8.
// Cells are stored X-major: Cells[x*GridHeight+y], with 20-pixel spacing.
// Cell values are preserved; they are not treated as tile image identifiers.
type GroundPrefix struct {
	Width, Height         uint32
	Layers                []GroundLayer
	GridWidth, GridHeight uint16
	Cells                 []byte
	BytesRead             int
}

// GroundLayer contains the three words read for each initial scene layer.
// The client uses the first as a resource identifier; rendering order and
// placement semantics require further tracing.
type GroundLayer struct{ Resource, X, Y uint16 }

// DecodeGroundPrefix reads a record at a caller-supplied, independently known
// offset. It deliberately stops after the grid; it cannot discover map IDs or
// record boundaries without the client's companion archive index.
func DecodeGroundPrefix(data []byte) (GroundPrefix, error) {
	r := protocol.NewReader(data)
	g := GroundPrefix{Width: r.U32(), Height: r.U32()}
	count := r.U8()
	for i := 0; i < int(count); i++ {
		g.Layers = append(g.Layers, GroundLayer{r.U16(), r.U16(), r.U16()})
	}
	g.GridWidth, g.GridHeight = r.U16(), r.U16()
	if r.Err() != nil {
		return GroundPrefix{}, fmt.Errorf("Ground.MMG header: %w", r.Err())
	}
	// Native runtime arrays cap each axis at 900 cells. Validate before allocating.
	if g.Width == 0 || g.Height == 0 || g.GridWidth == 0 || g.GridHeight == 0 || g.GridWidth > 900 || g.GridHeight > 900 {
		return GroundPrefix{}, fmt.Errorf("Ground.MMG: invalid dimensions %dx%d / %dx%d", g.Width, g.Height, g.GridWidth, g.GridHeight)
	}
	n := int(g.GridWidth) * int(g.GridHeight)
	cells := r.Bytes(n)
	if r.Err() != nil {
		return GroundPrefix{}, fmt.Errorf("Ground.MMG cells: %w", r.Err())
	}
	g.Cells = append([]byte(nil), cells...)
	g.BytesRead = len(data) - r.Remaining()
	return g, nil
}

// Cell returns one raw native terrain value; false means outside the decoded grid.
func (g GroundPrefix) Cell(x, y int) (byte, bool) {
	if x < 0 || y < 0 || x >= int(g.GridWidth) || y >= int(g.GridHeight) {
		return 0, false
	}
	i := x*int(g.GridHeight) + y
	if i >= len(g.Cells) {
		return 0, false
	}
	return g.Cells[i], true
}
