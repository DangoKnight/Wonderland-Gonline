package assets

import (
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func TestResourceRespawnOverridesArePerSource(t *testing.T) {
	c := &Catalog{Maps: map[uint16]Map{60000: {ID: 60000}}, ResourceRespawns: []ResourceRespawn{
		{Kind: game.ResourceCollectionActor, MapID: 60000, NodeID: 33, CooldownSeconds: 45},
		{Kind: game.ResourceCollectionActor, MapID: 60000, NodeID: 34, CooldownSeconds: 240},
		{Kind: game.ResourceGroundItem, MapID: 60000, NodeID: 33, CooldownSeconds: 15},
	}}
	if err := ValidateResourceRespawns(c.ResourceRespawns, c.Maps); err != nil {
		t.Fatal(err)
	}
	for _, r := range c.ResourceRespawns {
		if got := c.ResourceCooldown(r.Kind, r.MapID, r.NodeID, time.Minute); got != time.Duration(r.CooldownSeconds)*time.Second {
			t.Fatal("override aliased another source", r, got)
		}
	}
	if got := c.ResourceCooldown(game.ResourceCollectionActor, 60000, 35, 70*time.Second); got != 70*time.Second {
		t.Fatal("missing override changed authored timing", got)
	}
	for _, bad := range []ResourceRespawn{{Kind: 0, MapID: 60000, NodeID: 33, CooldownSeconds: 1}, {Kind: game.ResourceGroundItem, MapID: 60000, NodeID: 0, CooldownSeconds: 1}, {Kind: game.ResourceGroundItem, MapID: 60001, NodeID: 33, CooldownSeconds: 1}, {Kind: game.ResourceGroundItem, MapID: 60000, NodeID: 33, CooldownSeconds: 0}, c.ResourceRespawns[0]} {
		rows := append(append([]ResourceRespawn(nil), c.ResourceRespawns...), bad)
		if ValidateResourceRespawns(rows, c.Maps) == nil {
			t.Fatal("invalid or duplicate source accepted", bad)
		}
	}
}

func TestAuthoredCollectionCooldownIsNotFixedToThreeMinutes(t *testing.T) {
	c := &Catalog{EventGathering: EventGatheringRules{Timers: []uint16{11020}, Pools: []EventGatheringPool{{Type: 1, Variant: 1, Items: []uint16{40012}}}}}
	for _, seconds := range []byte{30, 180, 240} {
		ev := Event{Branches: []Branch{{Operations: []Operation{{Data: [21]byte{1, 1, 0, 6, 0, 1, 0, 0, 1}}, {Data: [21]byte{14, 12, 43, 1, 0, 0, 0, 0, seconds}}}}}}
		plan, ok := c.PlanEventGathering(&ev, 0)
		if !ok || plan.Timer != 11020 || plan.Cooldown != time.Duration(seconds)*time.Second {
			t.Fatal("native duration replaced", seconds, plan)
		}
	}
}
