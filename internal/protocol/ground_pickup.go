package protocol

const ObservedGroundPickupBytes = 12

// ObservedGroundPickup extends AC23:2 only on negotiated Gonline connections.
// Native AC23:2 has no collector identity: four bytes remove silently, while
// an extra byte requests a flight to the receiving client's own character.
type ObservedGroundPickup struct {
	Slot      uint16
	Collector uint32
	X, Y      uint16
}

func (p ObservedGroundPickup) Packet() []byte {
	return Builder{CommandInventory, InventoryPickup}.U16(p.Slot).U32(p.Collector).U16(p.X).U16(p.Y)
}

func ParseObservedGroundPickup(p []byte) (ObservedGroundPickup, bool) {
	if len(p) != ObservedGroundPickupBytes || p[0] != CommandInventory || p[1] != InventoryPickup {
		return ObservedGroundPickup{}, false
	}
	r := NewReader(p[2:])
	v := ObservedGroundPickup{Slot: r.U16(), Collector: r.U32(), X: r.U16(), Y: r.U16()}
	return v, v.Slot != 0 && v.Collector != 0
}
