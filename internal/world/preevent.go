package world

import (
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Reference: PreEventInterpreter.ShouldNpcBeVisible and EveEventRuntime.MatchesCondition.
// Owned pets and active item vehicles evaluate against persisted character state.
// Shared prop runtime state remains pending; optional SQL quest actor lists are supported.

type rule struct {
	conditions [][21]byte
	actions    []assets.Operation
}

// rules groups PreEvent branches: a branch with actions starts a rule, and following
// branches without actions add AND conditions to it.
func rules(ev assets.Event) []rule {
	var out []rule
	for _, br := range ev.Branches {
		if len(br.Operations) > 0 || len(out) == 0 {
			out = append(out, rule{conditions: [][21]byte{br.Condition}, actions: br.Operations})
		} else {
			last := &out[len(out)-1]
			last.conditions = append(last.conditions, br.Condition)
		}
	}
	return out
}

func (w *World) applicable(c *game.Character, mapID uint16, ev assets.Event) []rule {
	all := rules(ev)
	// Sealed Bead's type-6 fallback must not override its quest-specific rules.
	if mapID != game.MapID12508 || ev.ClickID != 2 {
		return all
	}
	specific := false
	for _, r := range all {
		if w.ruleMatches(c, mapID, r) && r.conditions[0][0] != 6 {
			specific = true
		}
	}
	if !specific {
		return all
	}
	out := all[:0:0]
	for _, r := range all {
		if !(len(r.conditions) == 1 && r.conditions[0][0] == 6) {
			out = append(out, r)
		}
	}
	return out
}

func (w *World) ruleMatches(c *game.Character, mapID uint16, r rule) bool {
	for _, cond := range r.conditions {
		if !w.preEventCondition(c, mapID, cond) {
			return false
		}
	}
	return true
}

func compare(actual, expected int64, op byte) bool {
	switch op {
	case CompareLess:
		return actual < expected
	case CompareGreater:
		return actual > expected
	case CompareLessOrEqual:
		return actual <= expected
	case CompareGreaterOrEqual:
		return actual >= expected
	case CompareEqual:
		return actual == expected
	case CompareNotEqual:
		return actual != expected
	}
	return false
}

func questActive(c *game.Character, id uint16) (game.Quest, bool) {
	q, ok := c.Quests[uint32(id)]
	return q, ok && q.State == game.InProgress
}

// preEventCondition is MatchesPreEventCondition: no event context.
func (w *World) preEventCondition(c *game.Character, mapID uint16, d [21]byte) bool {
	kind, w1, w2, w3, w4, w5, w6 := d[0], le16(d[:], 1), le16(d[:], 3), le16(d[:], 5), le16(d[:], 7), le16(d[:], 9), le16(d[:], 11)
	// Holy's wine delivery conceals the record keeper only during an active delivery.
	if mapID == game.MapID11013 && kind == 5 && w1 == 13056 && w2 == 1 && w3 == 1 && w4 == 0x0301 && w5 == 0 && w6&255 == 0 {
		if q, ok := questActive(c, 13056); !ok || q.Step == 0 {
			return false
		}
	}
	return w.condition(c, nil, mapID, nil, d)
}

// condition is EveEventRuntime.MatchesCondition. ev is nil for PreEvents.
func (w *World) condition(c *game.Character, v *View, mapID uint16, ev *assets.Event, d [21]byte) bool {
	return w.conditionAt(c, v, mapID, ev, d, time.Now())
}

func (w *World) conditionAt(c *game.Character, v *View, mapID uint16, ev *assets.Event, d [21]byte, now time.Time) bool {
	kind, w1, w2, w3, w4, w5, w6 := d[0], le16(d[:], 1), le16(d[:], 3), le16(d[:], 5), le16(d[:], 7), le16(d[:], 9), le16(d[:], 11)
	value := int64(int32(uint32(w4>>8) | uint32(w5)<<8 | uint32(w6&255)<<24))
	op := byte(w4)
	switch kind {
	case ConditionAlways, ConditionAlwaysAlternate:
		return true
	case ConditionEmptyOperands:
		return w1 == 0 && w2 == 0 && w3 == 0 && w4 == 0
	case ConditionQuest:
		q, active := questActive(c, w1)
		if w2 == 2 {
			return !active
		}
		if w2 != 1 {
			return false
		}
		mark := int64(0)
		if active {
			mark = int64(q.Step)
		}
		return compare(mark, value, op)
	case ConditionSubject:
		switch w1 {
		case 1:
			switch w2 {
			case 1:
				// Travel checks accept the capsule family, except for an event that
				// hands in this vehicle: removing the base item must find that exact item.
				family := w3 >= 48001 && w3 <= 48042 && !handsInVehicle(ev, w3)
				count := int64(0)
				for _, it := range c.Bag {
					if it.ID == w3 || (family && game.SameVehicle(uint32(it.ID), w3)) {
						count += int64(it.Count)
					}
				}
				return compare(count, value, op)
			case 2:
				return compare(int64(c.Gold), value, op)
			case 3:
				return compare(int64(c.Level), value, op)
			case 4:
				riding := int64(0)
				if game.SameVehicle(uint32(c.ActiveVehicle), w3) {
					riding = 1
				}
				return w3 >= 48001 && w3 <= 48042 && compare(riding, value, op)
			}
		case 2:
			_, present := c.Pet(uint32(w3))
			switch w2 {
			case 1:
				return present
			case 2:
				return !present
			case 3:
				// The only 2/2/3 record is Clive's prison branch with the active-Clive alternative.
				return ev != nil && mapID == game.MapID11050 && ev.ClickID == 1 && w3 == 14175 && present
			case 5:
				return compare(int64(max(0, game.MaxPets-len(c.Pets))), value, op)
			case 6:
				// The packed value names the following companion; it must be in the party.
				if w3 != 0 || value <= 0 || value > 0xffff || (op != 5 && op != 6) {
					return false
				}
				following := false
				if c.ActivePet != 0 {
					for _, p := range c.Pets {
						if game.SamePet(p.ID, c.ActivePet) && game.SamePet(p.ID, uint32(value)) {
							following = true
						}
					}
				}
				return following == (op == 5)
			case 8, 9:
				owned := present
				for _, pet := range c.HotelPets {
					owned = owned || game.SamePet(pet.ID, uint32(w3))
				}
				return owned == (w2 == 8)
			case 7, 10:
				i, ok := c.Pet(uint32(w3))
				if !ok {
					return false
				}
				if w2 == 7 {
					return compare(int64(c.Pets[i].Amity), value, op)
				}
				return compare(int64(c.Pets[i].Level), value, op)
			}
		}
		return false
	case ConditionSkillGrade:
		if w2 != 1 || w3 != 0 {
			return false
		}
		if _, ok := w.catalog.Skills[w1]; !ok {
			return false
		}
		grade := int64(0)
		for _, s := range c.Skills {
			if s.ID == w1 {
				grade = int64(s.Grade)
				break
			}
		}
		return compare(grade, value, op)
	case ConditionWaterGathering:
		// Only the four verified water timers have native condition semantics.
		if !game.IsWaterGatheringTimer(w1) || w2 < 1 || w2 > 2 || w3 != 0 || w4 != 0 || w5 != 0 || w6&255 != 0 {
			return false
		}
		return c.EventTimerActive(w1, now) == (w2 == 1)
	case ConditionProp:
		return ev != nil && w.propCondition(c, v, mapID, ev, w1, w2, value, op)
	case ConditionTeamSize:
		// EVE 13/1 is the team size; only the prison escape's full-team reminder uses it.
		team := int64(1)
		if v != nil {
			team += int64(v.Team.Load())
		}
		return ev != nil && mapID == game.MapID11052 && ev.ClickID == 3 && w1 == 1 && w2 == 0 && w3 == 0 && compare(team, value, op)
	case ConditionCompanionEquipment:
		// EveEventRuntime enables only Elin's three authored weapon gates.
		if mapID != storyElinWeaponMap || w1 != storyElinTemplate ||
			!((w2 == storyElinNativeWeaponOperand4 && w3 == storyElinOriginalWeapon) ||
				(w2 == storyElinNativeWeaponOperand1 && w3 == storyElinReplacementWeapon)) {
			return false
		}
		index, present := c.Pet(storyElinTemplate)
		return present && compare(int64(c.Pets[index].Equipment[storyWeaponEquipmentIndex].ID), int64(w3), op)
	case ConditionFreeBagSlots:
		free := int64(0)
		for _, it := range c.Bag {
			if it.ID == 0 {
				free++
			}
		}
		return w1 == 1 && compare(free, value, op)
	}
	// Unknown condition types never authorize rewards.
	return false
}

func handsInVehicle(ev *assets.Event, vehicle uint16) bool {
	if ev == nil {
		return false
	}
	for _, br := range ev.Branches {
		for _, o := range br.Operations {
			op := DecodeOp(o)
			if op.Code == ActionPlayer && op.D1 == 1 && op.D2 == 1 && game.SameVehicle(uint32(op.D3), vehicle) && int32(op.Value()) < 0 {
				return true
			}
		}
	}
	return false
}

// propCondition is MatchesCondition type 3: a durable door mark on three maps, a prop
// frame authored by PreEvents, or else whether this event's chest has been emptied.
func (w *World) propCondition(c *game.Character, v *View, mapID uint16, ev *assets.Event, actor, mode uint16, value int64, op byte) bool {
	flag := func(open bool) bool {
		if open {
			return compare(1, value, op)
		}
		return compare(0, value, op)
	}
	switch {
	case mapID == game.MapID11036 && actor == 7 && mode == 3:
		door, ok := questActive(c, 10024)
		done, ok2 := questActive(c, 10025)
		return flag((ok && door.Step >= 1) || (ok2 && done.Step == 1))
	case mapID == game.MapID11037 && actor == 7 && mode == 3:
		fate, ok := questActive(c, 13086)
		done, ok2 := questActive(c, 13087)
		return flag(ok && fate.Step > 0 && !(ok2 && done.Step > 0))
	case mapID == game.MapID11015 && actor == 4 && mode == 3:
		heretic, ok := questActive(c, 13024)
		done, ok2 := questActive(c, 13025)
		return flag((ok && heretic.Step >= 7) || (ok2 && done.Step > 0))
	}
	if expiry, configured := c.ChestRespawns[ChestKey(mapID, ev.ClickID)]; configured {
		return flag(time.Now().Before(expiry))
	}
	if mode == 3 && v != nil && Mechanism(mapID) {
		if state, ok := v.Props[actor]; ok {
			return compare(int64(state), value, op)
		}
	}
	if mode == 3 {
		if state, declared := w.PropState(c, mapID, actor); declared {
			return compare(int64(state), value, op)
		}
	}
	q, ok := c.Quests[ChestKey(mapID, ev.ClickID)]
	return flag(ok && q.State == game.Completed)
}

// ChestKey is the per-character mark recording an emptied chest.
func ChestKey(mapID, event uint16) uint32 { return uint32(mapID)*1000 + uint32(event) }

// PropState is PreEventInterpreter.TryGetNativePropState: the frame PreEvents author for
// an actor (action 2/5 with word 1), where later matching rules win. declared reports
// whether any rule mentions the actor; undeclared props start closed (0).
func (w *World) PropState(c *game.Character, mapID, actor uint16) (state int32, declared bool) {
	m, ok := w.maps[mapID]
	if !ok {
		return 0, false
	}
	for _, ev := range m.data.PreEvents {
		for _, r := range w.applicable(c, mapID, ev) {
			var frames []int32
			for _, a := range r.actions {
				if a.Data[0] == 2 && le16(a.Data[:], 1) == actor && le16(a.Data[:], 3) == 5 && le16(a.Data[:], 5) == 1 {
					frames = append(frames, int32(uint32(a.Data[8])|uint32(a.Data[9])<<8|uint32(a.Data[10])<<16|uint32(a.Data[11])<<24))
				}
			}
			if len(frames) == 0 {
				continue
			}
			declared = true
			if w.ruleMatches(c, mapID, r) {
				state = frames[len(frames)-1]
			}
		}
	}
	return state, declared
}

// questState maps marks to EVE lifecycle values: 1 in progress, 2 not started, 3 completed.
func questState(c *game.Character, id uint16) uint16 {
	q, ok := c.Quests[uint32(id)]
	if !ok {
		return 2
	}
	switch q.State {
	case game.InProgress:
		return 1
	case game.Completed:
		return 3
	}
	return 2
}

func questStep(c *game.Character, id uint16) int {
	if q, ok := c.Quests[uint32(id)]; ok {
		return q.Step
	}
	return 0
}

// storyVisibility ports the hardcoded ShouldNpcBeVisible overrides that depend only on
// quest marks. Companion-based overrides evaluate with an empty party.
func storyVisibility(c *game.Character, mapID, click uint16) (visible, decided bool) {
	switch {
	case mapID == game.MapID12000 && (click == 17 || click == 10 || click == 31):
		return true, true
	case mapID == game.MapID11040 && click == 4:
		if q, ok := questActive(c, 13102); ok && q.Step == 4 {
			if done, ok := questActive(c, 13103); !ok || done.Step == 0 {
				return true, true
			}
		}
	case mapID == game.MapID11016 && click == 1:
		// S. Monkey leaves once recruited (as a pet or by mark 12002).
		_, monkey := c.Pet(17162)
		_, alias := c.Pet(10727)
		return !(monkey || alias || c.ActivePet == 17162 || c.ActivePet == 10727 || questState(c, 12002) == 3), true
	case mapID == game.MapID12000:
		switch click {
		case 32:
			return false, true
		case 29:
			return true, true
		case 28:
			st, step := questState(c, 13046), questStep(c, 13046)
			return !(st == 2 || (st == 1 && step < 2 && questState(c, 13047) != 1)), true
		case 20:
			st, step := questState(c, 13046), questStep(c, 13046)
			return st == 1 && step < 2 && questState(c, 13047) != 1, true
		case 18, 19:
			return questState(c, 13023) != 2, true
		case 14:
			st, step := questState(c, 12020), questStep(c, 12020)
			return (st == 1 && step >= 2) || st == 3 || questState(c, 12021) == 1, true
		case 15:
			return questState(c, 12020) == 1 && questStep(c, 12020) == 1, true
		case 16:
			return !(questState(c, 13020) == 2 || questState(c, 13021) == 1), true
		}
	}
	return false, false
}

// Visible reports whether a map actor starts visible for a character.
func (w *World) Visible(c *game.Character, mapID, click uint16) bool {
	return w.visible(c, nil, mapID, click)
}

// VisibleIn is Visible with the client's scripted actor state.
func (w *World) VisibleIn(c *game.Character, v *View, mapID, click uint16) bool {
	return w.visible(c, v, mapID, click)
}

// visible is ShouldNpcBeVisible; on mechanism maps a scripted show/hide wins.
func (w *World) visible(c *game.Character, view *View, mapID, click uint16) bool {
	if view != nil {
		if shown, ok := view.AdminActors[click]; ok {
			return shown
		}
	}
	if view != nil && Mechanism(mapID) {
		if shown, ok := view.Actors[click]; ok {
			return shown
		}
	}
	// A defeated wild monster stays hidden until it respawns.
	if w.Defeated(mapID, click) {
		if m, ok := w.maps[mapID]; ok {
			if n, ok := m.npc(click); ok && w.Wild(mapID, n) {
				return false
			}
		}
	}
	if v, ok := storyVisibility(c, mapID, click); ok {
		return v
	}
	m, ok := w.maps[mapID]
	if !ok {
		return true
	}
	// A recruited story companion leaves the map.
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			if n.Template > 0 && w.InParty(c, n.Template) {
				return false
			}
			break
		}
	}
	if visible, owned := w.questVisibility(c, mapID, click); owned {
		return visible
	}
	visible := true
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			visible = n.Flags&1 != 0
			break
		}
	}
	// Rules are cumulative in file order; later matches override earlier ones.
	for _, ev := range m.data.PreEvents {
		for _, r := range w.applicable(c, mapID, ev) {
			if !w.ruleMatches(c, mapID, r) {
				continue
			}
			for _, a := range r.actions {
				if a.Data[0] != 2 || le16(a.Data[:], 1) != click {
					continue
				}
				switch le16(a.Data[:], 3) {
				case 2:
					visible = false
				case 3:
					visible = true
				}
			}
		}
	}
	if mapID == game.MapID60002 && (click == 43 || click == 44) {
		if q, ok := questActive(c, 50042); ok && (q.Step == 1 || (q.Step > 1 && questState(c, 50047) == 1)) {
			return true
		}
	}
	if mapID == game.MapID12002 && (click == 2 || click == 7) && xaolanInFate(c) {
		return false
	}
	if mapID == game.MapID12002 && (click == 2 || click == 3) {
		if q, ok := questActive(c, 13033); ok && q.Step == 1 && questState(c, 13004) != 1 && questState(c, 13005) != 1 {
			return click != 2 || !w.InParty(c, 14156)
		}
	}
	return visible
}

func xaolanInFate(c *game.Character) bool {
	if q, ok := questActive(c, 13087); ok && q.Step > 0 {
		return true
	}
	q, ok := questActive(c, 13086)
	return ok && q.Step >= 2
}

// Template is the Npc.dat data a pet or companion derives from.
func (w *World) Template(id uint32) (game.PetTemplate, bool) {
	if id > 0xffff {
		return game.PetTemplate{}, false
	}
	n, ok := w.catalog.NPCs[uint16(id)]
	return game.PetTemplate{Type: n.Type, Stats: game.Attributes{Strength: n.Stats[0], Constitution: n.Stats[1], Intelligence: n.Stats[2], Wisdom: n.Stats[3], Agility: n.Stats[4]}, Skills: n.Skills}, ok
}

// InParty is HasStoryCompanionInParty: a story companion template now in the party.
func (w *World) InParty(c *game.Character, template uint32) bool {
	t, known := w.Template(game.BroadcastID(template))
	if !game.StoryCompanion(template, t, known) {
		return false
	}
	_, ok := c.Pet(template)
	return ok
}
