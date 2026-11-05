package clientruntime

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"wonderland-gonline/internal/clientassets"
)

const BattleBackdropsFile = "runtime/battle_backdrops.bin"

// Offline export projection. Raw native bytes are never shipped in this record.
type BackdropExport struct {
	Entries []struct {
		Name string
		Hex  string `json:"decoded_hex"`
	} `json:"entries"`
}

func (d BackdropExport) Decode() (map[uint16]clientassets.GroundBackdrops, error) {
	values := map[uint16]clientassets.GroundBackdrops{}
	for _, e := range d.Entries {
		if e.Hex == "" {
			continue
		} // Older synthetic/minimal exports have no backdrop metadata.
		id, err := strconv.ParseUint(strings.TrimSuffix(strings.ToLower(e.Name), ".map"), 10, 16)
		if err != nil || id == 0 {
			return nil, fmt.Errorf("invalid backdrop map %q", e.Name)
		}
		if _, ok := values[uint16(id)]; ok {
			return nil, fmt.Errorf("duplicate backdrop map %d", id)
		}
		raw, err := hex.DecodeString(e.Hex)
		if err != nil {
			return nil, err
		}
		b, err := clientassets.DecodeGroundBackdrops(raw)
		if err != nil {
			return nil, fmt.Errorf("map %d: %w", id, err)
		}
		values[uint16(id)] = b
	}
	return values, nil
}
