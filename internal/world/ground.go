package world

import (
	"sort"
	"sync"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// PickupRange is the C# 180-pixel reach for ground pickups.
const PickupRange = 180

// Reference: GameMap.onItemPickup, onItemDrop and the respawn loop in GameMap.Process.
// Dropped items live only in memory, like the C# ItemsDropped dictionary.
type ground struct {
	mu      sync.Mutex
	taken   map[uint16]map[byte]time.Time // Native slot to respawn time.
	dropped map[uint16]map[byte]droppedItem
}

type droppedItem struct {
	item game.Item
	x, y uint16
}

// GroundTarget is an item a character may pick up from a ground slot.
type GroundTarget struct {
	Slot    byte
	Item    game.Item
	X, Y    uint16
	native  *GroundItem
	dropped bool
}

func (g *ground) init() {
	g.taken = map[uint16]map[byte]time.Time{}
	g.dropped = map[uint16]map[byte]droppedItem{}
}

// GroundPacket lists available native items, then dropped items, as one AC23:4 packet.
// It returns nil when the map has nothing on the ground.
func (w *World) GroundPacket(mapID uint16) []byte {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryGroundItems}
	if m, ok := w.maps[mapID]; ok {
		for _, it := range m.Items {
			if _, taken := w.ground.taken[mapID][it.Slot]; !taken {
				p = groundItem(p, it.Slot, it.ItemID, it.X, it.Y)
			}
		}
	}
	for _, slot := range sortedSlots(w.ground.dropped[mapID]) {
		d := w.ground.dropped[mapID][slot]
		p = groundItem(p, slot, d.item.ID, d.x, d.y)
	}
	if len(p) == 2 {
		return nil
	}
	return p
}

// GroundAt resolves a pickup slot: dropped items first, then a native item by slot,
// then by click ID. A taken native item is not offered again before it respawns.
func (w *World) GroundAt(mapID uint16, slot byte) (GroundTarget, bool) {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	if d, ok := w.ground.dropped[mapID][slot]; ok {
		return GroundTarget{Slot: slot, Item: d.item, X: d.x, Y: d.y, dropped: true}, true
	}
	m, ok := w.maps[mapID]
	if !ok {
		return GroundTarget{}, false
	}
	var found *GroundItem
	for i := range m.Items {
		if m.Items[i].Slot == slot {
			found = &m.Items[i]
			break
		}
	}
	if found == nil {
		for i := range m.Items {
			if m.Items[i].ClickID == uint16(slot) {
				found = &m.Items[i]
				break
			}
		}
	}
	if found == nil {
		return GroundTarget{}, false
	}
	if _, taken := w.ground.taken[mapID][found.Slot]; taken {
		return GroundTarget{}, false
	}
	return GroundTarget{Slot: found.Slot, Item: game.Item{ID: found.ItemID, Count: 1}, X: found.X, Y: found.Y, native: found}, true
}

// Take removes a target after the picker's inventory change is durable.
func (w *World) Take(mapID uint16, t GroundTarget, now time.Time) {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	if t.dropped {
		delete(w.ground.dropped[mapID], t.Slot)
		return
	}
	if w.ground.taken[mapID] == nil {
		w.ground.taken[mapID] = map[byte]time.Time{}
	}
	w.ground.taken[mapID][t.Slot] = now.Add(t.native.Respawn)
}

// FreeGroundSlots reserves nothing; it lists n slots unused by dropped items and by
// native items' slots and click IDs, so a pickup cannot reach a neighboring node.
func (w *World) FreeGroundSlots(mapID uint16, n int) ([]byte, bool) {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	m := w.maps[mapID]
	var free []byte
	for slot := 1; slot <= 255 && len(free) < n; slot++ {
		if _, used := w.ground.dropped[mapID][byte(slot)]; used {
			continue
		}
		used := false
		if m != nil {
			for _, it := range m.Items {
				if int(it.Slot) == slot || int(it.ClickID) == slot {
					used = true
					break
				}
			}
		}
		if !used {
			free = append(free, byte(slot))
		}
	}
	return free, len(free) == n
}

// Drop places one unit of item in each slot and returns the AC23:4 broadcast.
func (w *World) Drop(mapID uint16, slots []byte, item game.Item, x, y uint16) []byte {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	if w.ground.dropped[mapID] == nil {
		w.ground.dropped[mapID] = map[byte]droppedItem{}
	}
	item.Count = 1
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryGroundItems}
	for _, slot := range slots {
		w.ground.dropped[mapID][slot] = droppedItem{item, x, y}
		p = groundItem(p, slot, item.ID, x, y)
	}
	return p
}

// Respawn restores native items whose time has come and returns one AC23:4 per item,
// grouped by map, as GameMap.Process broadcasts them.
func (w *World) Respawn(now time.Time) map[uint16][][]byte {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	out := map[uint16][][]byte{}
	for mapID, taken := range w.ground.taken {
		m := w.maps[mapID]
		for _, it := range m.Items {
			if at, ok := taken[it.Slot]; ok && !now.Before(at) {
				delete(taken, it.Slot)
				out[mapID] = append(out[mapID], groundItem(protocol.Builder{protocol.CommandInventory, protocol.InventoryGroundItems}, it.Slot, it.ItemID, it.X, it.Y))
			}
		}
	}
	return out
}

// InRange reports whether (x, y) is within PickupRange of the target.
func (t GroundTarget) InRange(x, y uint16) bool {
	dx, dy := int64(x)-int64(t.X), int64(y)-int64(t.Y)
	return dx*dx+dy*dy <= PickupRange*PickupRange
}

func sortedSlots(m map[byte]droppedItem) []byte {
	out := make([]byte, 0, len(m))
	for slot := range m {
		out = append(out, slot)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ClearDropped removes player drops without changing native ground respawns.
func (w *World) ClearDropped(mapID uint16) [][]byte {
	w.ground.mu.Lock()
	defer w.ground.mu.Unlock()
	var packets [][]byte
	for _, slot := range sortedSlots(w.ground.dropped[mapID]) {
		packets = append(packets, protocol.Builder{protocol.CommandInventory, protocol.InventoryPickup}.U16(uint16(slot)).U8(0))
	}
	delete(w.ground.dropped, mapID)
	return packets
}
