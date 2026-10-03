package game

import (
	"errors"
	"wonderland-go/internal/protocol"
)

const BagSize = 50

var ErrInventoryFull = errors.New("inventory full")
var ErrInvalidItem = errors.New("invalid item or slot")

type Item struct {
	ID       uint16                  `json:"id"`
	Count    byte                    `json:"count"`
	Damage   byte                    `json:"damage"`
	Metadata [ItemMetadataBytes]byte `json:"metadata"`
}
type Inventory [BagSize]Item

func (i Item) Empty() bool { return i.ID == 0 || i.Count == 0 }
func (i Item) compatible(other Item) bool {
	return i.ID == other.ID && i.Damage == other.Damage && i.Metadata == other.Metadata
}

// Addition is one slot's share of a grant, as reported by additive AC23:5.
type Addition struct{ Slot, Count byte }

// Add plans the whole grant before changing the bag. maxStack is supplied from Item.dat.
func (b *Inventory) Add(item Item, count int, maxStack byte) error {
	_, e := b.Grant(item, count, maxStack)
	return e
}

// Grant is Add that also reports the changed slots: existing stacks first, then empty slots.
func (b *Inventory) Grant(item Item, count int, maxStack byte) ([]Addition, error) {
	if item.ID == 0 || count < 1 || count > BagSize*MaxItemStack || maxStack < 1 || maxStack > MaxItemStack {
		return nil, ErrInvalidItem
	}
	next := *b
	var adds []Addition
	for j := range next {
		if count > 0 && !next[j].Empty() && next[j].compatible(item) && next[j].Count < maxStack {
			n := min(count, int(maxStack-next[j].Count))
			next[j].Count += byte(n)
			count -= n
			adds = append(adds, Addition{byte(j + 1), byte(n)})
		}
	}
	for j := range next {
		if count > 0 && next[j].Empty() {
			n := min(count, int(maxStack))
			next[j] = item
			next[j].Count = byte(n)
			count -= n
			adds = append(adds, Addition{byte(j + 1), byte(n)})
		}
	}
	if count > 0 {
		return nil, ErrInventoryFull
	}
	*b = next
	return adds, nil
}

// AdditionPacket is Inventory.SendAddedItems: AC23:5 records carrying only the added counts.
func (b Inventory) AdditionPacket(adds []Addition) []byte {
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryItems}
	for _, a := range adds {
		item := b[a.Slot-1]
		p = p.U8(a.Slot).U16(item.ID).U8(a.Count).U8(item.Damage).Bytes(item.Metadata[:])
	}
	return p
}

func (b *Inventory) Remove(slot byte, count byte) error {
	if slot < 1 || slot > BagSize || count == 0 {
		return ErrInvalidItem
	}
	i := &b[slot-1]
	if i.Empty() || i.Count < count {
		return ErrInvalidItem
	}
	i.Count -= count
	if i.Count == 0 {
		*i = Item{}
	}
	return nil
}

// Move follows Inventory.MoveItem: it moves as much of count as the destination holds
// and reports the moved amount. Occupied destinations must be a compatible stack.
func (b *Inventory) Move(from, to, count, maxStack byte) (byte, error) {
	if from < 1 || from > BagSize || to < 1 || to > BagSize || from == to || count == 0 || maxStack < 1 || maxStack > MaxItemStack {
		return 0, ErrInvalidItem
	}
	src, dst := b[from-1], b[to-1]
	if src.Empty() || (!dst.Empty() && !dst.compatible(src)) {
		return 0, ErrInvalidItem
	}
	capacity := maxStack
	if !dst.Empty() {
		capacity = maxStack - min(dst.Count, maxStack)
	}
	moved := min(count, src.Count, capacity)
	if moved == 0 {
		return 0, ErrInvalidItem
	}
	if dst.Empty() {
		b[to-1] = src
		b[to-1].Count = moved
	} else {
		b[to-1].Count += moved
	}
	return moved, b.Remove(from, moved)
}

// Packet serializes the authentic 31-byte records used by AC23:5 and AC30:5.
func (b Inventory) Packet(action, sub byte) []byte {
	p := protocol.Builder{action, sub}
	for index, item := range b {
		if item.Empty() {
			continue
		}
		p = p.U8(byte(index + 1)).U16(item.ID).U8(item.Count).U8(item.Damage).Bytes(item.Metadata[:])
	}
	return p
}

// Transfer is atomic for callers holding exclusive ownership of both bags.
func Transfer(from, to *Inventory, slot, count, maxStack byte) error {
	if from == to || slot < 1 || slot > BagSize {
		return ErrInvalidItem
	}
	a, b := *from, *to
	item := a[slot-1]
	if e := a.Remove(slot, count); e != nil {
		return e
	}
	if e := b.Add(item, int(count), maxStack); e != nil {
		return e
	}
	*from, *to = a, b
	return nil
}

// ItemChange adds (Count > 0) or removes (Count < 0) an item by ID.
type ItemChange struct {
	ID    uint16
	Count int
}

// QuestItemResult lists the client updates for an applied change set: AC23:9 removals
// (Count is the amount removed) in slot order, then one additive AC23:5.
type QuestItemResult struct {
	Removed, Added []Addition
}

// ApplyQuestItems follows Inventory.TryApplyQuestItems: changes are simulated in order,
// so a hand-in can free the slot its reward needs. Removals take any copies of the ID.
// Nothing changes unless every step succeeds. limit reports an item's stack size.
func (b *Inventory) ApplyQuestItems(changes []ItemChange, limit func(uint16) (byte, bool)) (QuestItemResult, error) {
	next := *b
	for _, change := range changes {
		maxStack, ok := limit(change.ID)
		if !ok || change.ID == 0 || change.Count == 0 || change.Count < -BagSize*MaxItemStack || change.Count > BagSize*MaxItemStack {
			return QuestItemResult{}, ErrInvalidItem
		}
		if change.Count > 0 {
			if _, e := next.Grant(Item{ID: change.ID}, change.Count, maxStack); e != nil {
				return QuestItemResult{}, e
			}
			continue
		}
		needed := -change.Count
		for i := range next {
			if needed > 0 && next[i].ID == change.ID && !next[i].Empty() {
				take := min(needed, int(next[i].Count))
				next[i].Count -= byte(take)
				needed -= take
				if next[i].Count == 0 {
					next[i] = Item{}
				}
			}
		}
		if needed > 0 {
			return QuestItemResult{}, ErrInvalidItem
		}
	}
	// A slot holding a different item is cleared first; the rest is a count delta.
	var r QuestItemResult
	for i := range next {
		before, after := b[i], next[i]
		held := 0
		if !before.Empty() {
			held = int(before.Count)
			if before.ID != after.ID || (!after.Empty() && !before.compatible(after)) {
				r.Removed = append(r.Removed, Addition{byte(i + 1), before.Count})
				held = 0
			}
		}
		if delta := int(after.Count) - held; delta < 0 {
			r.Removed = append(r.Removed, Addition{byte(i + 1), byte(-delta)})
		} else if delta > 0 {
			r.Added = append(r.Added, Addition{byte(i + 1), byte(delta)})
		}
	}
	*b = next
	return r, nil
}
