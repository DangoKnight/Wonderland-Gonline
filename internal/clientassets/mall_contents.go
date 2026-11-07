package clientassets

import (
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

const mallContentsHeaderBytes = 2
const mallContentsItemBytes = 3

type MallReward struct {
	ItemID   uint16 `json:"item_id"`
	Quantity byte   `json:"quantity"`
}
type MallContents struct {
	Incomplete     bool         `json:"incomplete,omitempty"` // Featured rewards known; remaining props unresolved.
	CompanionIcons bool         `json:"companion_icons,omitempty"`
	ItemID         uint16       `json:"item_id"`
	Kind           byte         `json:"kind,omitempty"` // Native Lbd metadata; not a reward probability.
	Rewards        []MallReward `json:"rewards"`
}

func MallIncompleteContents() map[uint16]bool {
	var packs []MallContents
	if err := json.Unmarshal(mallContentsJSON, &packs); err != nil {
		panic(err)
	}
	result := map[uint16]bool{}
	for _, p := range packs {
		if p.Incomplete {
			result[p.ItemID] = true
		}
	}
	return result
}

// ParseMallContents follows FUN_001ada04: U16 pack count, U8 lengths,
// then [U16 pack ID, U8 kind, length * (U16 reward ID, U8 quantity)].
func ParseMallContents(raw []byte) ([]MallContents, error) {
	if len(raw) < mallContentsHeaderBytes {
		return nil, fmt.Errorf("truncated Lbd header")
	}
	count := int(binary.LittleEndian.Uint16(raw))
	offset := mallContentsHeaderBytes + count
	if offset > len(raw) {
		return nil, fmt.Errorf("truncated Lbd lengths")
	}
	result := make([]MallContents, 0, count)
	seen := map[uint16]bool{}
	for _, length := range raw[mallContentsHeaderBytes:offset] {
		size := mallContentsItemBytes * (1 + int(length))
		if offset+size > len(raw) {
			return nil, fmt.Errorf("truncated Lbd contents")
		}
		pack := MallContents{ItemID: binary.LittleEndian.Uint16(raw[offset:]), Kind: raw[offset+2]}
		if pack.ItemID == 0 || seen[pack.ItemID] {
			return nil, fmt.Errorf("invalid Lbd pack %d", pack.ItemID)
		}
		seen[pack.ItemID] = true
		for i := 0; i < int(length); i++ {
			p := offset + mallContentsItemBytes*(i+1)
			r := MallReward{binary.LittleEndian.Uint16(raw[p:]), raw[p+2]}
			if r.ItemID == 0 || r.Quantity == 0 {
				return nil, fmt.Errorf("invalid Lbd reward")
			}
			pack.Rewards = append(pack.Rewards, r)
		}
		result = append(result, pack)
		offset += size
	}
	if offset != len(raw) {
		return nil, fmt.Errorf("trailing Lbd bytes")
	}
	return result, nil
}

// Client display metadata only. It does not define server awards or odds.
//
//go:embed mall_contents.json
var mallContentsJSON []byte

//go:embed mall_fitting.json
var mallFittingJSON []byte

//go:embed mall_forging.json
var mallForgingJSON []byte

type MallForgeUpgrade struct {
	Next    uint16
	Scrolls byte
}

func DefaultMallForging() map[uint16]MallForgeUpgrade {
	var v map[uint16]MallForgeUpgrade
	if err := json.Unmarshal(mallForgingJSON, &v); err != nil {
		panic(err)
	}
	return v
}

//go:embed mall_vouchers.json
var mallVouchersJSON []byte

func DefaultMallVouchers() map[uint16]uint16 {
	var v map[uint16]uint16
	if err := json.Unmarshal(mallVouchersJSON, &v); err != nil {
		panic(err)
	}
	return v
}
func MallCompanionContents() map[uint16]bool {
	var packs []MallContents
	if err := json.Unmarshal(mallContentsJSON, &packs); err != nil {
		panic(err)
	}
	v := map[uint16]bool{}
	for _, p := range packs {
		if p.CompanionIcons {
			v[p.ItemID] = true
		}
	}
	return v
}

func DefaultMallFitting() map[uint16]uint16 {
	var entries []struct {
		BoxID  uint16 `json:"box_id"`
		ItemID uint16 `json:"item_id"`
	}
	if err := json.Unmarshal(mallFittingJSON, &entries); err != nil {
		panic(err)
	}
	result := map[uint16]uint16{}
	for _, e := range entries {
		result[e.BoxID] = e.ItemID
	}
	return result
}

func DefaultMallContents() map[uint16][]MallReward {
	var packs []MallContents
	if err := json.Unmarshal(mallContentsJSON, &packs); err != nil {
		panic(err)
	}
	result := make(map[uint16][]MallReward, len(packs))
	for _, p := range packs {
		result[p.ItemID] = p.Rewards
	}
	return result
}
