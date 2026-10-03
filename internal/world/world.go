// Package world derives map runtime state and per-character map views from native EVE data.
// Reference: wlo.pserver.core/Game/Maps/Map.cs (ReloadSpawns, SendMapInfo, LookupPortal).
package world

import (
	"sort"
	"strings"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// NPC is a spawned EVE actor. Coordinates are truncated to the wire width like the C# loader.
type NPC struct {
	ClickID  uint16
	Template uint32
	X, Y     uint16
	Name     string
}

// GroundItem is a native EVE item node. Slot is the AC23:4/23:2 ground slot.
type GroundItem struct {
	Slot    byte
	ClickID uint16
	ItemID  uint16
	X, Y    uint16
	Respawn time.Duration
}

type Map struct {
	ID    uint16
	NPCs  []NPC // Ascending click ID, the AC22:4 order.
	Items []GroundItem
	data  assets.Map
}

func newMap(m assets.Map) *Map {
	out := &Map{ID: m.ID, data: m}
	for _, n := range m.NPCs {
		// Corrupted entries have no click ID or no position.
		if n.ClickID == 0 || (n.X == 0 && n.Y == 0) {
			continue
		}
		out.NPCs = append(out.NPCs, NPC{ClickID: n.ClickID, Template: n.Template, X: uint16(n.X), Y: uint16(n.Y), Name: n.Name})
	}
	sort.SliceStable(out.NPCs, func(i, j int) bool { return out.NPCs[i].ClickID < out.NPCs[j].ClickID })
	slot := byte(1)
	for _, it := range m.Items {
		if it.ItemID == 0 || (it.X == 0 && it.Y == 0) {
			continue
		}
		g := GroundItem{Slot: slot, ClickID: it.ClickID, ItemID: uint16(it.ItemID), X: uint16(it.X), Y: uint16(it.Y), Respawn: 120 * time.Second}
		if it.ClickID > 0 && it.ClickID < 256 {
			g.Slot = byte(it.ClickID)
		}
		if it.Unknown[0] > 0 {
			g.Respawn = time.Duration(it.Unknown[0]) * time.Second
		}
		slot++
		out.Items = append(out.Items, g)
	}
	return out
}

// NPC returns a spawned actor by click ID.
func (m *Map) NPC(click uint16) (NPC, bool) { return m.npc(click) }

// DoorPortal is the portal an EVE "door" object links to (AC20.Recv1's door fallback).
func (m *Map) DoorPortal(click uint16) (uint16, bool) {
	if m == nil {
		return 0, false
	}
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			if len(n.Links) > 0 && strings.Contains(strings.ToLower(n.Name), "door") {
				return uint16(n.Links[0]), true
			}
			return 0, false
		}
	}
	return 0, false
}

func (m *Map) npc(click uint16) (NPC, bool) {
	for _, n := range m.NPCs {
		if n.ClickID == click {
			return n, true
		}
	}
	return NPC{}, false
}

// World is safe for concurrent use. Map definitions are immutable; ground state has its own lock.
type World struct {
	maps     map[uint16]*Map
	catalog  *assets.Catalog
	ground   ground
	monsters monsters
}

func New(c *assets.Catalog) *World {
	w := &World{maps: make(map[uint16]*Map, len(c.Maps)), catalog: c}
	w.ground.init()
	w.monsters.defeated = map[uint16]map[uint16]time.Time{}
	for id, m := range c.Maps {
		w.maps[id] = newMap(m)
	}
	return w
}

func (w *World) Map(id uint16) (*Map, bool) {
	m, ok := w.maps[id]
	return m, ok
}

// idleFrame holds prop frame 0 for Npc.dat types 6, 9 and 10; 0xFF animates actors.
func (w *World) idleFrame(template uint32) uint16 {
	if template <= 0xffff {
		if n, ok := w.catalog.NPCs[uint16(template)]; ok && (n.Type == 6 || n.Type == 9 || n.Type == 10) {
			return 0
		}
	}
	return 0xff
}

// questProp reports whether a prop's linked event is gated by a completed quest.
// Static props outside the template ranges below are classified by name in C#; those
// heuristics are not ported, so such props keep their idle frame.
func questPropOpened(m *Map, n NPC, c *game.Character) bool {
	if !(n.Template == 0 || n.Template >= 19000 || (n.Template >= 12000 && n.Template <= 12999)) {
		return false
	}
	for _, ev := range m.data.Events {
		if ev.ClickID != n.ClickID {
			continue
		}
		for _, br := range ev.Branches {
			if id := le16(br.Condition[:], 1); id > 0 {
				if q, ok := c.Quests[uint32(id)]; ok && q.State == game.Completed {
					return true
				}
			}
		}
		return false
	}
	return false
}

// NPCRecord is the 14-byte AC22:4 actor record.
func NPCRecord(click, state, x, y uint16, hidden bool) protocol.Builder {
	kind, duration := byte(1), uint32(0)
	if hidden {
		state, kind, duration = 0xffff, 2, 0x03e7fc18
	}
	return protocol.Builder{}.U16(click).U16(state).U16(x).U16(y).U8(kind).U32(duration).U8(0)
}

func groundItem(p protocol.Builder, slot byte, item, x, y uint16) protocol.Builder {
	return p.U8(3).U16(uint16(slot)).U32(uint32(item)).U16(x).U16(y).U32(0)
}

// MapInfo follows SendMapInfo for a character entering its current map. players lists
// the characters announced in the scene, including the entrant. It resets the view's
// scene state and records the actors concealed from this character.
func (w *World) MapInfo(c *game.Character, v *View, players []uint32) [][]byte {
	v.Reset()
	packets := [][]byte{{protocol.CommandInventory, protocol.InventorySceneBegin}}
	m, ok := w.maps[c.Map]
	if ok && len(m.NPCs) > 0 {
		list := protocol.Builder{protocol.CommandScene, protocol.SceneActorPosition}
		for _, n := range m.NPCs {
			concealed := !w.Visible(c, m.ID, n.ClickID)
			state := w.idleFrame(n.Template)
			if concealed {
				v.Hidden[n.ClickID] = true
			} else if questPropOpened(m, n, c) {
				state = 1
			}
			list = list.Bytes(NPCRecord(n.ClickID, state, n.X, n.Y, concealed))
		}
		packets = append(packets, list)
	}
	if items := w.GroundPacket(c.Map); items != nil {
		packets = append(packets, items)
	}
	for _, id := range players {
		packets = append(packets, protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode122}.U32(id), protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(id).U8(255), protocol.Builder{protocol.CommandInventory, protocol.InventoryWireCode76}.U32(id))
	}
	return append(packets, []byte{protocol.CommandInventory, protocol.InventorySceneComplete}, []byte{protocol.CommandEvent, protocol.EventResume})
}

func le16(b []byte, i int) uint16 { return uint16(b[i]) | uint16(b[i+1])<<8 }
