package assets

import (
	"encoding/json"
	"fmt"
)

// ParsePetVouchers is PetVoucherManager.Load. Reject the whole table on an
// invalid mapping; a partial configuration must never consume a voucher.
func ParsePetVouchers(data []byte) (map[uint16]uint16, error) {
	var entries []struct {
		Item uint16 `json:"item_id"`
		Pet  uint16 `json:"pet_id"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("empty pet voucher table")
	}
	result := map[uint16]uint16{}
	for _, e := range entries {
		if e.Item == 0 || e.Pet == 0 || result[e.Item] != 0 {
			return nil, fmt.Errorf("invalid pet voucher mapping")
		}
		result[e.Item] = e.Pet
	}
	return result, nil
}
