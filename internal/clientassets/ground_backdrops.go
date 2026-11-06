package clientassets

import (
	"fmt"
	"wonderland-gonline/internal/protocol"
)

const (
	groundSoundZoneBytes      = 6
	groundObjectBytes         = 8
	groundBackdropRegionBytes = 9
)

// GroundBackdrops follows the collision grid, sound zones and map objects.
// FUN_003f830c loads these bytes into TMap+0xc7038..; FUN_003fa0b8 draws
// each nine-byte record as a rectangle labelled with scene 59000+Selector.
// The preceding byte (usually 100) has unresolved semantics and is skipped.
type GroundBackdrops struct {
	Default byte
	Regions []GroundBackdropRegion
}

type GroundBackdropRegion struct {
	Selector                 byte
	Left, Top, Right, Bottom uint16
}

func (b GroundBackdrops) At(x, y uint16) byte {
	for _, r := range b.Regions {
		if x >= r.Left && x <= r.Right && y >= r.Top && y <= r.Bottom {
			return r.Selector
		}
	}
	return b.Default
}

func DecodeGroundBackdrops(data []byte) (GroundBackdrops, error) {
	g, err := DecodeGroundPrefix(data)
	if err != nil {
		return GroundBackdrops{}, err
	}
	r := protocol.NewReader(data[g.BytesRead:])
	r.Bytes(int(r.U16()) * groundSoundZoneBytes)
	r.Bytes(int(r.U16()) * groundObjectBytes)
	r.U8() // Unresolved native map-tail parameter, not part of backdrop selection.
	b := GroundBackdrops{Default: r.U8()}
	count := int(r.U8())
	if r.Err() != nil || count > r.Remaining()/groundBackdropRegionBytes {
		return GroundBackdrops{}, fmt.Errorf("Ground.MMG: truncated backdrop regions")
	}
	for range count {
		region := GroundBackdropRegion{r.U8(), r.U16(), r.U16(), r.U16(), r.U16()}
		if region.Left > region.Right || region.Top > region.Bottom {
			return GroundBackdrops{}, fmt.Errorf("Ground.MMG: inverted backdrop rectangle")
		}
		b.Regions = append(b.Regions, region)
	}
	return b, nil
}
