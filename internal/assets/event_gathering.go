package assets

// Event gathering uses EVE pool type/variant operands, independent of the
// continuous fishing/mining/woodcutting service's Economy.Gathering pools.
type EventGatheringRules struct {
	Timers []uint16             `json:"timers"`
	Pools  []EventGatheringPool `json:"pools"`
}
type EventGatheringPool struct {
	Type    uint16   `json:"type"`
	Variant uint32   `json:"variant"`
	Items   []uint16 `json:"items"`
}

func (c *Catalog) EventGatheringPool(kind uint16, variant uint32) []uint16 {
	for _, p := range c.EventGathering.Pools {
		if p.Type == kind && p.Variant == variant {
			return p.Items
		}
	}
	return nil
}
