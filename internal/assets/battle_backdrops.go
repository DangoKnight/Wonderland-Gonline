package assets

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"wonderland-gonline/internal/clientassets"
)

// BattleBackdrop is a map's native default and ordered position overrides.
// Native rectangles include their boundary; the first matching record wins.
// Scene IDs are 59000+selector. Unknown selectors are preserved for Legacy's
// missing-scene fallback rather than silently replaced during import.
type BattleBackdrop struct {
	Default byte
	Regions []BattleBackdropRegion
}

type BattleBackdropRegion struct {
	Selector                 byte
	Left, Top, Right, Bottom uint16
}

func (b BattleBackdrop) At(x, y uint16) byte {
	for _, r := range b.Regions {
		if x >= r.Left && x <= r.Right && y >= r.Top && y <= r.Bottom {
			return r.Selector
		}
	}
	return b.Default
}

func DecodeBattleBackdrop(raw []byte) (BattleBackdrop, error) {
	native, err := clientassets.DecodeGroundBackdrops(raw)
	if err != nil {
		return BattleBackdrop{}, err
	}
	b := BattleBackdrop{Default: native.Default}
	for _, r := range native.Regions {
		b.Regions = append(b.Regions, BattleBackdropRegion{r.Selector, r.Left, r.Top, r.Right, r.Bottom})
	}
	return b, nil
}

func ParseBattleBackdrops(raw []byte) (map[uint16]BattleBackdrop, error) {
	a, err := clientassets.ReadNamedArchive(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, err
	}
	values := map[uint16]BattleBackdrop{}
	for i, entry := range a.Entries {
		if !strings.HasSuffix(strings.ToLower(entry.Name), ".map") {
			continue
		}
		id, err := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(entry.Name), ".map"), 10, 16)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid backdrop map %q", entry.Name)
		}
		if _, ok := values[uint16(id)]; ok {
			return nil, fmt.Errorf("duplicate backdrop map %d", id)
		}
		data, err := a.Read(i)
		if err != nil {
			return nil, err
		}
		b, err := DecodeBattleBackdrop(data)
		if err != nil {
			return nil, fmt.Errorf("map %d: %w", id, err)
		}
		values[uint16(id)] = b
	}
	return values, ValidateBattleBackdrops(values)
}

func ValidateBattleBackdrops(values map[uint16]BattleBackdrop) error {
	for id, b := range values {
		if id == 0 {
			return fmt.Errorf("backdrop needs a map ID")
		}
		for _, r := range b.Regions {
			if r.Left > r.Right || r.Top > r.Bottom {
				return fmt.Errorf("map %d: inverted backdrop region", id)
			}
		}
	}
	return nil
}
