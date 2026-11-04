package assets

import (
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
)

// Optional per-source overrides. Missing entries retain the native/default
// duration; reward pools, native timer IDs and other spots are unchanged.
type ResourceRespawn struct {
	Kind            game.ResourceKind `json:"kind"`
	MapID           uint16            `json:"map_id"`
	NodeID          uint16            `json:"node_id"`
	CooldownSeconds uint32            `json:"cooldown_seconds"`
}

func (c *Catalog) ResourceCooldown(kind game.ResourceKind, mapID, nodeID uint16, fallback time.Duration) time.Duration {
	for _, r := range c.ResourceRespawns {
		if r.Kind == kind && r.MapID == mapID && r.NodeID == nodeID {
			return time.Duration(r.CooldownSeconds) * time.Second
		}
	}
	return fallback
}

func ValidateResourceRespawns(rows []ResourceRespawn, maps map[uint16]Map) error {
	seen := map[[3]uint16]bool{}
	for _, r := range rows {
		key := [3]uint16{uint16(r.Kind), r.MapID, r.NodeID}
		_, known := maps[r.MapID]
		if !known || r.MapID == 0 || r.NodeID == 0 || r.CooldownSeconds == 0 || seen[key] || (r.Kind != game.ResourceCollectionActor && r.Kind != game.ResourceGroundItem && r.Kind != game.ResourceCollectionArea) {
			return fmt.Errorf("invalid or duplicate resource respawn %v", key)
		}
		seen[key] = true
	}
	return nil
}
