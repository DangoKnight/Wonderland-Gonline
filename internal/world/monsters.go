package world

import "wonderland-go/internal/game"

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// MonsterRespawn is how long a defeated overworld monster stays away.
const MonsterRespawn = 60 * time.Second

type monsters struct {
	mu       sync.Mutex
	defeated map[uint16]map[uint16]time.Time // Map to click ID to respawn time.
}

// hasLinkedEvent is HasLinkedEvent: an actor whose linked events have actions.
func (w *World) hasLinkedEvent(mapID, click uint16) bool {
	m, ok := w.maps[mapID]
	if !ok {
		return false
	}
	for _, n := range m.data.NPCs {
		if n.ClickID != click {
			continue
		}
		for _, id := range n.Events {
			if ev, ok := w.Event(mapID, uint16(id)); ok && hasActions(ev) {
				return true
			}
		}
		return false
	}
	return false
}

// templateName is the actor's Npc.dat name, which the C# NPC catalog supplies.
func (w *World) templateName(t uint32) string {
	if t > 0xffff {
		return ""
	}
	return w.catalog.NPCs[uint16(t)].Name
}

// Wild is QuestNpc.IsWildMonster: roaming encounter actors, and unlinked 17000-17999
// templates that are not services, pigs or townspeople by name.
func (w *World) Wild(mapID uint16, n NPC) bool {
	if n.Template == 0 {
		return false
	}
	if w.RoamingBattle(mapID, n.ClickID) {
		return true
	}
	if w.hasLinkedEvent(mapID, n.ClickID) || n.Template < 17000 || n.Template >= 18000 || n.Template == 17400 {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(w.templateName(n.Template)))
	for _, word := range []string{"shop", "store", "market", "keep", "storage", "bank", "exchanger", "doctor", "witch", "clinic", "hotel", "inn",
		"guidepost", "signpost", "statue", "pig", "villager", "citizen", "resident", "grandma", "grandmother", "grandfather", "elder", "mayor",
		"chief", "guard", "soldier", "knight", "merchant", "vendor", "trader", "peddler", "innkeeper", "waitress", "nurse", "priest", "monk",
		"clerk", "sailor", "captain", "chef", "cook", "maid", "blacksmith", "carpenter", "hunter", "miner", "guide", "girl", "boy", "kid",
		"child", "man", "woman", "lady", "sir", "robinson"} {
		if strings.Contains(lower, word) {
			return false
		}
	}
	return true
}

// SafeTown is IsSafeTownMap: towns and interiors have no random encounters unless the
// map has native field-encounter actors.
func (w *World) SafeTown(mapID uint16) bool {
	if m, ok := w.maps[mapID]; ok {
		for _, n := range m.data.NPCs {
			if w.RoamingBattle(mapID, n.ClickID) {
				return false
			}
		}
	}
	if mapID == game.MapID10000 || (mapID >= game.MapID10001 && mapID <= game.MapID10036) || mapID == game.MapID11094 || (mapID >= game.MapID60001 && mapID <= game.MapID60020) {
		return true
	}
	for town := uint16(11000); town <= 21000; town += 1000 {
		if mapID == town || (mapID > town && mapID <= town+35) {
			return true
		}
	}
	return false
}

// Defeat removes a wild monster from the map for everyone until it respawns.
func (w *World) Defeat(mapID, click uint16, now time.Time) {
	w.monsters.mu.Lock()
	defer w.monsters.mu.Unlock()
	if w.monsters.defeated[mapID] == nil {
		w.monsters.defeated[mapID] = map[uint16]time.Time{}
	}
	w.monsters.defeated[mapID][click] = now.Add(MonsterRespawn)
}

// Defeated reports a wild monster that has not respawned yet.
func (w *World) Defeated(mapID, click uint16) bool {
	w.monsters.mu.Lock()
	defer w.monsters.mu.Unlock()
	_, ok := w.monsters.defeated[mapID][click]
	return ok
}

// Revive returns, per map, the monsters whose respawn time has come, in click order.
func (w *World) Revive(now time.Time) map[uint16][]uint16 {
	w.monsters.mu.Lock()
	defer w.monsters.mu.Unlock()
	out := map[uint16][]uint16{}
	for mapID, clicks := range w.monsters.defeated {
		for click, at := range clicks {
			if !now.Before(at) {
				delete(clicks, click)
				out[mapID] = append(out[mapID], click)
			}
		}
		sort.Slice(out[mapID], func(i, j int) bool { return out[mapID][i] < out[mapID][j] })
	}
	return out
}
