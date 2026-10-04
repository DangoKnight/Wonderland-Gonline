package world

import (
	"sort"
	"sync/atomic"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
)

// View is what one client has been told about its current scene. A session owns it
// and the server serializes access. AdminActors, Hidden, Markers, Props and Actors reset on each map
// entry (Player.CurMap and SendMapInfo in C#); Marks spans the whole login.
type View struct {
	AdminActors map[uint16]bool  // Explicit live administrator show/hide decisions.
	Hidden      map[uint16]bool  // HiddenNpcClickIDs
	Markers     map[uint32]byte  // SentQuestMinimapMarkers, keyed (kind-1)<<16 | target
	Props       map[uint16]int32 // NativePropStates on mechanism maps
	Actors      map[uint16]bool  // NativeActorVisibility on mechanism maps
	Marks       map[uint16]bool  // SentNotebookMarks
	// Team is the player's team size beyond itself; the server updates it
	// from another session's commands, so it is atomic.
	Team atomic.Int32
}

func NewView() *View {
	v := &View{Marks: map[uint16]bool{}}
	v.Reset()
	return v
}

// Reset clears the scene state for a new map entry.
func (v *View) Reset() {
	v.AdminActors = map[uint16]bool{}
	v.Hidden, v.Markers, v.Props, v.Actors = map[uint16]bool{}, map[uint32]byte{}, map[uint16]int32{}, map[uint16]bool{}
}

// Mechanism maps keep authored prop and actor state for the current visit.
func Mechanism(mapID uint16) bool {
	switch mapID {
	case game.MapID11157, game.MapID11158, game.MapID11159, game.MapID11166, game.MapID12380, game.MapID60014, game.MapID12523:
		return true
	}
	return false
}

// coordinates is GetNpcCoordinates: the spawned actor, else its EVE record.
func (w *World) coordinates(mapID, click uint16) (uint16, uint16) {
	m, ok := w.maps[mapID]
	if !ok {
		return 0, 0
	}
	if n, ok := w.NPC(mapID, click); ok {
		return n.X, n.Y
	}
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			return uint16(n.X), uint16(n.Y)
		}
	}
	return 0, 0
}

func (w *World) template(mapID, click uint16) uint32 {
	m, ok := w.maps[mapID]
	if !ok {
		return 0
	}
	if n, ok := m.npc(click); ok {
		return n.Template
	}
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			return n.Template
		}
	}
	return 0
}

// HideActor is PreEventInterpreter.SendActorHide.
func (w *World) HideActor(v *View, mapID, click uint16) []byte {
	v.Hidden[click] = true
	x, y := w.coordinates(mapID, click)
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorPosition}.Bytes(NPCRecord(click, 0, x, y, true))
}

// ShowActor is PreEventInterpreter.SendActorShow, using any authored prop frame.
func (w *World) ShowActor(v *View, mapID, click uint16) []byte {
	delete(v.Hidden, click)
	x, y := w.coordinates(mapID, click)
	state := w.idleFrame(w.template(mapID, click))
	if s, ok := v.Props[click]; ok {
		state = uint16(s)
	}
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorPosition}.Bytes(NPCRecord(click, state, x, y, false))
}

// Marker is SendMinimapMarker: AC22:12, sent only when the icon changes unless forced.
func Marker(v *View, kind byte, target uint16, icon byte, force bool) []byte {
	key := uint32(kind-1)<<16 | uint32(target)
	if sent, ok := v.Markers[key]; ok && sent == icon && !force {
		return nil
	}
	v.Markers[key] = icon
	return protocol.Builder{protocol.CommandScene, protocol.SceneActorAction, kind}.U16(target).U8(icon)
}

// QuestUpdate is NotebookManager.SendQuestUpdate: a completion flag toggles AC24:5;
// a journal mark is replaced with AC24:4 then AC24:1 while it is active.
func (w *World) QuestUpdate(v *View, mark uint32, q game.Quest) [][]byte {
	flag, ok := w.catalog.Marks[uint16(mark)]
	if mark > 0xffff || !ok {
		return nil
	}
	enabled := q.State == game.InProgress && q.Step > 0
	if flag != 0 {
		if flag > 2000 {
			return nil
		}
		on := byte(0)
		if enabled {
			on = 1
		}
		return [][]byte{protocol.Builder{protocol.CommandQuest, protocol.QuestFlag}.U16(flag).U8(on)}
	}
	out := [][]byte{protocol.Builder{protocol.CommandQuest, protocol.QuestMark}.U16(uint16(mark))}
	delete(v.Marks, uint16(mark))
	if enabled && len(v.Marks) < 200 {
		out = append(out, protocol.Builder{protocol.CommandQuest, protocol.QuestMarkState}.U16(uint16(mark)).U8(byte(min(q.Step, 255))))
		v.Marks[uint16(mark)] = true
	}
	return out
}

// Journal is NotebookManager.SendQuestJournal: remove every sent mark, list active
// journal marks (AC24:6), then all 2000 completion bits as index/bits pairs (AC24:7).
func (w *World) Journal(c *game.Character, v *View) [][]byte {
	var out [][]byte
	for _, id := range sortedMarks(v.Marks) {
		out = append(out, protocol.Builder{protocol.CommandQuest, protocol.QuestMark}.U16(id))
	}
	v.Marks = map[uint16]bool{}
	var completed [250]byte
	active := protocol.Builder{protocol.CommandQuest, protocol.QuestActiveMarks}
	ids := make([]uint32, 0, len(c.Quests))
	for id := range c.Quests {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	slot := byte(0)
	for _, id := range ids {
		q := c.Quests[id]
		flag, ok := w.catalog.Marks[uint16(id)]
		if id > 0xffff || !ok || q.State != game.InProgress || q.Step <= 0 {
			continue
		}
		if flag != 0 {
			if flag <= 2000 {
				completed[(flag-1)/8] |= 1 << ((flag - 1) % 8)
			}
		} else if slot < 200 {
			slot++
			active = active.U8(slot).U16(uint16(id)).U8(byte(min(q.Step, 255)))
			v.Marks[uint16(id)] = true
		}
	}
	if slot != 0 {
		out = append(out, active)
	}
	flags := protocol.Builder{protocol.CommandQuest, protocol.QuestWireCode7}
	for i, bits := range completed {
		flags = flags.U8(byte(i + 1)).U8(bits)
	}
	return append(out, flags)
}

func sortedMarks(m map[uint16]bool) []uint16 {
	out := make([]uint16, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Sync is QuestManager.SyncPerPlayerNpcVisibility for the character's current map:
// replay completed one-time events, re-evaluate PreEvent visibility and prop frames,
// then refresh minimap markers. Companion despawns are not ported.
func (w *World) Sync(c *game.Character, v *View, force bool) [][]byte {
	mapID := c.Map
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	var out [][]byte
	// ReplayActorVisibility: completed events leave their chest open or actor removed.
	for _, ev := range m.data.Events {
		for _, br := range ev.Branches {
			id := uint32(DecodeCond(br.Condition).W1)
			if q, done := c.Quests[id]; id == 0 || !done || q.State != game.Completed {
				continue
			}
			chest, despawn := -1, -1
			for i, o := range br.Operations {
				op := DecodeOp(o)
				if op.Code == ActionActor && op.D2 == 5 && chest < 0 {
					chest = i
				}
				if op.Code == ActionActor && op.D2 == 2 && despawn < 0 {
					despawn = i
				}
			}
			if chest >= 0 {
				target := ev.ClickID
				if op := DecodeOp(br.Operations[chest]); op.D1 > 0 {
					target = op.D1
				}
				out = append(out, protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(target).U8(1))
			} else if despawn >= 0 {
				target := ev.ClickID
				if op := DecodeOp(br.Operations[despawn]); op.D1 > 0 {
					target = op.D1
				}
				out = append(out, w.HideActor(v, mapID, target))
			}
		}
	}
	// EvaluateMapPreEvents: every actor the map or its PreEvents can address.
	ids := map[uint16]bool{}
	for _, n := range m.data.NPCs {
		ids[n.ClickID] = true
	}
	for _, ev := range m.data.PreEvents {
		if ev.ClickID > 0 {
			ids[ev.ClickID] = true
		}
		for _, br := range ev.Branches {
			for _, a := range br.Operations {
				if a.Data[0] == 2 {
					if target := le16(a.Data[:], 1); target > 0 {
						ids[target] = true
					}
				}
			}
		}
	}
	for _, q := range w.catalog.QuestVisibility {
		if q.Map != mapID {
			continue
		}
		for _, id := range q.Spawn {
			ids[id] = true
		}
		for _, id := range q.Despawn {
			ids[id] = true
		}
		for _, step := range q.Steps {
			for _, id := range step.Spawn {
				ids[id] = true
			}
			for _, id := range step.Despawn {
				ids[id] = true
			}
		}
	}
	for id := range v.Hidden {
		ids[id] = true
	}
	switch mapID {
	case game.MapID12000:
		for _, id := range []uint16{10, 14, 15, 16, 17, 18, 19, 20, 28, 29, 31, 32, 33, 34, 35, 36} {
			ids[id] = true
		}
	case game.MapID12001:
		ids[1], ids[2], ids[3] = true, true, true
	}
	order := make([]uint16, 0, len(ids))
	for id := range ids {
		order = append(order, id)
	}
	sort.Slice(order, func(i, j int) bool { return order[i] < order[j] })
	for _, id := range order {
		visible, hidden := w.visible(c, v, mapID, id), v.Hidden[id]
		if !visible && (force || !hidden) {
			out = append(out, w.HideActor(v, mapID, id))
		} else if visible && (force || hidden) {
			out = append(out, w.ShowActor(v, mapID, id))
		}
	}
	for _, ev := range m.data.PreEvents {
		for _, r := range w.applicable(c, mapID, ev) {
			if !w.ruleMatches(c, mapID, r) {
				continue
			}
			for _, a := range r.actions {
				if a.Data[0] != 2 {
					continue
				}
				kind, s1, s2 := le16(a.Data[:], 3), a.Data[8], a.Data[9]
				if kind == 5 || (s1 != 0xff && s2 != 0xff && kind != 2 && kind != 3) {
					out = append(out, w.actionBlock(v, mapID, a)...)
				}
			}
		}
	}
	// Administrator chest cooldowns override authored one-time prop frames.
	for _, ev := range m.data.Events {
		if expiry, ok := c.ChestRespawns[ChestKey(mapID, ev.ClickID)]; ok {
			state := int32(0)
			if time.Now().Before(expiry) {
				state = 1
			}
			old, known := v.Props[ev.ClickID]
			if force || !known || old != state {
				out = append(out, protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(ev.ClickID).U8(byte(state)))
			}
			v.Props[ev.ClickID] = state
		}
	}
	return append(out, w.Markers(c, v, force)...)
}

// actionBlock is PreEventInterpreter.ExecuteActionBlock for actor actions.
func (w *World) actionBlock(v *View, mapID uint16, a assets.Operation) [][]byte {
	d := a.Data[:]
	click, kind, s1, s2 := le16(d, 1), le16(d, 3), d[8], d[9]
	if s, ok := v.Props[click]; ok && kind == 5 && Mechanism(mapID) {
		return [][]byte{protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(click).U8(byte(s))}
	}
	// Guideposts and the honeycomb on map 12000 are permanent scenery.
	if mapID == game.MapID12000 && (click == 17 || click == 10 || click == 31) {
		return nil
	}
	switch {
	case kind == 7:
		if p := ActorPose(click, le16(d, 5), s1); p != nil {
			return [][]byte{p}
		}
		return nil
	case kind == 2 || (s1 == 0xff && s2 == 0xff):
		return [][]byte{w.HideActor(v, mapID, click)}
	case kind == 3:
		return [][]byte{w.ShowActor(v, mapID, click)}
	case v.Hidden[click]:
		// A frame refresh must not revive an actor this quest stage conceals.
		return nil
	}
	x, y := w.coordinates(mapID, click)
	return [][]byte{protocol.Builder{protocol.CommandScene, protocol.SceneActorPosition}.U16(click).U8(s1).U8(s2).U16(x).U16(y).U8(1).U32(0).U8(0)}
}

// ActorPose is SendActorPose: AC22:9 actor, animation, facing; nil when out of range.
func ActorPose(click, animation uint16, facing byte) []byte {
	if animation > 0xff {
		return nil
	}
	return protocol.Builder{protocol.CommandScene, protocol.SceneWireCode9}.U16(click).U8(byte(animation)).U8(facing)
}

// Markers is SyncQuestMinimapMarkers: authored PreEvent icons, else a question mark
// (icon 7) for an actor whose first eligible event advances an unfinished quest.
func (w *World) Markers(c *game.Character, v *View, force bool) [][]byte {
	mapID := c.Map
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	native := w.nativeMarkers(c, mapID)
	var out [][]byte
	for _, n := range m.data.NPCs {
		icon, authored := native[uint32(n.ClickID)]
		delete(native, uint32(n.ClickID))
		disabled := len(n.Events) > 0
		for _, id := range n.Events {
			if !w.Disabled(mapID, uint16(id)) {
				disabled = false
			}
		}
		if v.Hidden[n.ClickID] || disabled {
			icon = 0
		} else {
			for _, id := range n.Events {
				ev, ok := w.Event(mapID, uint16(id))
				if !ok {
					continue
				}
				i := w.FindBranch(c, v, mapID, ev, 0, 0, 0, -1)
				if i < 0 {
					continue
				}
				if w.Disabled(mapID, ev.ClickID) {
					icon = 0
				} else if !authored && !w.RoamingBattle(mapID, n.ClickID) && w.unfinished(c, ev, i) {
					icon = 7
				}
				break
			}
		}
		if p := Marker(v, 1, n.ClickID, icon, force); p != nil {
			out = append(out, p)
		}
	}
	keys := make([]uint32, 0, len(native))
	for k := range native {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, k := range keys {
		icon := native[k]
		if k < 1<<16 && v.Hidden[uint16(k)] {
			icon = 0
		}
		if p := Marker(v, byte(k>>16)+1, uint16(k), icon, force); p != nil {
			out = append(out, p)
		}
	}
	return out
}

// nativeMarkers is GetNativeMinimapMarkers: icons declared by PreEvent action 13, cleared
// to zero unless a matching rule sets them.
func (w *World) nativeMarkers(c *game.Character, mapID uint16) map[uint32]byte {
	out := map[uint32]byte{}
	m := w.maps[mapID]
	for _, ev := range m.data.PreEvents {
		for _, br := range ev.Branches {
			for _, a := range br.Operations {
				if kind := le16(a.Data[:], 1); a.Data[0] == 13 && kind >= 1 && kind <= 3 {
					out[uint32(kind-1)<<16|uint32(le16(a.Data[:], 3))] = 0
				}
			}
		}
	}
	for _, ev := range m.data.PreEvents {
		for _, r := range w.applicable(c, mapID, ev) {
			if !w.ruleMatches(c, mapID, r) {
				continue
			}
			for _, a := range r.actions {
				kind, icon := le16(a.Data[:], 1), le16(a.Data[:], 5)
				if a.Data[0] == 13 && kind >= 1 && kind <= 3 && icon >= 1 && icon <= 256 {
					out[uint32(kind-1)<<16|uint32(le16(a.Data[:], 3))] = byte(icon - 1)
				}
			}
		}
	}
	return out
}

// unfinished is IsUnfinishedQuestBranch.
func (w *World) unfinished(c *game.Character, ev *assets.Event, index int) bool {
	mentionsActive, hasActive, hasCompletion := false, false, false
	cond := DecodeCond(ev.Branches[index].Condition)
	for i := index; i < len(ev.Branches) && i <= index+cond.ExtraConditions(); i++ {
		k := DecodeCond(ev.Branches[i].Condition)
		flag, ok := w.catalog.Marks[k.W1]
		if k.Kind != 5 || !ok {
			continue
		}
		q, ok := c.Quests[uint32(k.W1)]
		present := ok && q.State == game.InProgress && q.Step > 0
		if flag == 0 {
			mentionsActive = true
			hasActive = hasActive || present
		} else {
			hasCompletion = hasCompletion || present
		}
	}
	changesActive := false
	for _, o := range ev.Branches[index].Operations {
		if op := DecodeOp(o); op.Code == ActionQuestMark {
			if flag, ok := w.catalog.Marks[op.D1]; ok && flag == 0 {
				changesActive = true
			}
		}
	}
	// A lost-item replacement after completion is not an unfinished quest.
	if hasCompletion && !hasActive && !changesActive {
		return false
	}
	return mentionsActive || changesActive
}
