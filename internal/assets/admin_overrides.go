package assets

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const AdminMapsAsset = "map_overrides.json"
const AdminChestAsset = "chest_drops.json"
const maxChestRespawnSeconds = 24 * 60 * 60
const maxChestRewardWeight = math.MaxInt32

type ChestReward struct {
	Item   uint16 `json:"item_id"`
	Name   string `json:"name"`
	Count  byte   `json:"count"`
	Weight uint32 `json:"weight"`
}
type ChestPool struct {
	MapID          uint16        `json:"map_id"`
	Category       string        `json:"category"`
	RespawnSeconds int           `json:"respawn_seconds"`
	Rewards        []ChestReward `json:"rewards"`
}

func ParseChestPools(raw []byte, c *Catalog) ([]ChestPool, error) {
	var pools []ChestPool
	if err := json.Unmarshal(raw, &pools); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, pool := range pools {
		key := "category:" + strings.ToLower(strings.TrimSpace(pool.Category))
		if pool.MapID != 0 {
			key = "map:" + fmt.Sprint(pool.MapID)
		}
		if seen[key] || pool.RespawnSeconds < 1 || pool.RespawnSeconds > maxChestRespawnSeconds || len(pool.Rewards) == 0 || (pool.MapID == 0 && pool.Category == "") {
			return nil, errors.New("invalid chest pool")
		}
		seen[key] = true
		if pool.MapID != 0 {
			if _, ok := c.Maps[pool.MapID]; !ok {
				return nil, errors.New("unknown chest map")
			}
		}
		var total uint64
		for _, r := range pool.Rewards {
			if _, ok := c.Items[r.Item]; !ok || r.Count == 0 || r.Weight == 0 {
				return nil, errors.New("invalid chest reward")
			}
			total += uint64(r.Weight)
		}
		if total > maxChestRewardWeight {
			return nil, errors.New("chest weights too large")
		}
	}
	return pools, nil
}
