package world

import (
	"slices"
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

// rules uses the authored condition count. Unrelated condition-only records must
// not attach themselves to the preceding rule (reference revisions fd63dac/0966c3d).
func rules(ev assets.Event) []rule {
	var out []rule
	for i, br := range ev.Branches {
		if len(br.Operations) == 0 {
			continue
		}
		count := 1 + DecodeCond(br.Condition).ExtraConditions()
		if i+count > len(ev.Branches) {
			continue // Truncated conditions cannot authorize an action.
		}
		r := rule{actions: br.Operations}
		valid := true
		for j := i; j < i+count; j++ {
			if j > i && len(ev.Branches[j].Operations) != 0 {
				valid = false
			}
			r.conditions = append(r.conditions, ev.Branches[j].Condition)
		}
		if valid {
			out = append(out, r)
		}
	}
	return out
}

func (w *World) applicable(c *game.Character, mapID uint16, ev assets.Event, views ...*View) []rule {
	all := rules(ev)
	// Sealed Bead's type-6 fallback must not override its quest-specific rules.
	policy, configured := w.StoryFirst(assets.StoryPreFallback, c, nil, StoryContext{Map: mapID, Event: ev.ClickID})
	if !configured {
		return all
	}
	specific := false
	for _, r := range all {
		if w.ruleMatches(c, mapID, r, views...) && r.conditions[0][0] != byte(policy.Value) {
			specific = true
		}
	}
	if !specific {
		return all
	}
	out := all[:0:0]
	for _, r := range all {
		if !(len(r.conditions) == 1 && r.conditions[0][0] == byte(policy.Value)) {
			out = append(out, r)
		}
	}
	return out
}

func (w *World) ruleMatches(c *game.Character, mapID uint16, r rule, views ...*View) bool {
	for _, cond := range r.conditions {
		if !w.preEventCondition(c, mapID, cond, views...) {
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
func (w *World) preEventCondition(c *game.Character, mapID uint16, d [21]byte, views ...*View) bool {
	var view *View
	if len(views) > 0 {
		view = views[0]
	}
	if r, ok := w.StoryFirst(assets.StoryPreCondition, c, view, StoryContext{Map: mapID, Raw: &d}); ok {
		return r.Value != 0
	}
	return w.condition(c, view, mapID, nil, d)
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
		q, exists := c.Quests[uint32(w1)]
		active := exists && (q.State == game.InProgress || q.State == game.Completed)
		if w2 == 2 {
			if op == 0 {
				return !exists || q.State != game.InProgress
			}
			return compare(int64(questState(c, w1)), value, op)
		}
		if w2 != 1 {
			return false
		}
		mark := int64(0)
		if active {
			mark = int64(q.Step)
			if q.State == game.Completed && mark == 0 {
				mark = 1
			}
		}
		if !active && (op == CompareLessOrEqual || (op == CompareLess && value > 1) || op == CompareGreater || op == CompareGreaterOrEqual) {
			return false
		}
		return compare(mark, value, op)
	case ConditionSubject:
		switch w1 {
		case SubjectBodyOrGender:
			switch w2 {
			case game.BodySmallMale, game.BodySmallFemale, game.BodyBigMale, game.BodyBigFemale:
				return c.Body == w2
			}
			male := c.Body == game.BodySmallMale || c.Body == game.BodyBigMale
			return (w2 == GenderMale && male) || (w2 == GenderFemale && !male)
		case SubjectElement, SubjectRebornJob:
			actual := uint16(c.Element)
			if w1 == SubjectRebornJob {
				actual = uint16(c.Job)
			}
			if op == CompareNotEqual {
				return actual != w2
			}
			return actual == w2
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
				event := uint16(0)
				if ev != nil {
					event = ev.ClickID
				}
				r, ok := w.StoryFirst(assets.StoryCondition, c, v, StoryContext{Map: mapID, Event: event, Raw: &d})
				return ok && r.Value != 0
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
		known := slices.Contains(w.catalog.EventGathering.Timers, w1)
		if w.catalog.EventGathering.Timers == nil {
			known = game.IsWaterGatheringTimer(w1)
		}
		if !known || w2 < 1 || w2 > 2 || w3 != 0 || w4 != 0 || w5 != 0 || w6&255 != 0 {
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
		return w1 == 1 && w2 == 0 && w3 == 0 && compare(team, value, op)
	case ConditionMissionVariable:
		actual := int64(0)
		if v != nil {
			actual = int64(v.MissionVariables[w1])
		}
		return (w2 == MissionVariableAdd && compare(actual, value, op)) || (w2 == MissionVariableAbsent && actual == 0)
	case ConditionCompanionRebirth:
		return w1 == uint16(c.CompanionRebirthState())
	case ConditionCompanionEquipment:
		event := uint16(0)
		if ev != nil {
			event = ev.ClickID
		}
		policy, allowed := w.StoryFirst(assets.StoryEquipmentGate, c, v, StoryContext{Map: mapID, Event: event, Raw: &d})
		if !allowed {
			return false
		}
		index, present := c.Pet(uint32(w1))
		slot := int(policy.Value)
		return present && slot >= 0 && slot < len(c.Pets[index].Equipment) && compare(int64(c.Pets[index].Equipment[slot].ID), int64(w3), op)
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
	var raw [21]byte
	raw[3] = byte(mode)
	raw[4] = byte(mode >> 8)
	if r, ok := w.StoryFirst(assets.StoryPropCondition, c, v, StoryContext{Map: mapID, Event: ev.ClickID, Actor: actor, Raw: &raw}); ok {
		return compare(r.Value, value, op)
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
		if state, declared := w.PropState(c, mapID, actor, v); declared {
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
func (w *World) PropState(c *game.Character, mapID, actor uint16, views ...*View) (state int32, declared bool) {
	m, ok := w.maps[mapID]
	if !ok {
		return 0, false
	}
	for _, ev := range m.data.PreEvents {
		for _, r := range w.applicable(c, mapID, ev, views...) {
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
			if w.ruleMatches(c, mapID, r, views...) {
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

// storyVisibility applies the SQL-authored early visibility overrides.
func (w *World) storyVisibility(c *game.Character, mapID, click uint16) (bool, bool) {
	r, ok := w.StoryFirst(assets.StoryVisibility, c, nil, StoryContext{Map: mapID, Actor: click})
	return r.Value != 0, ok
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
	if v, ok := w.storyVisibility(c, mapID, click); ok {
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
		for _, r := range w.applicable(c, mapID, ev, view) {
			if !w.ruleMatches(c, mapID, r, view) {
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
	if r, ok := w.StoryFirst(assets.StoryFinalVisibility, c, view, StoryContext{Map: mapID, Actor: click}); ok {
		return r.Value != 0
	}
	return visible
}

// Template is the Npc.dat data a pet or companion derives from.
func (w *World) Template(id uint32) (game.PetTemplate, bool) {
	if id > 0xffff {
		return game.PetTemplate{}, false
	}
	n, ok := w.catalog.NPCs[uint16(id)]
	return game.PetTemplate{Type: n.Type, Element: n.Element, Stats: game.Attributes{Strength: n.Stats[0], Constitution: n.Stats[1], Intelligence: n.Stats[2], Wisdom: n.Stats[3], Agility: n.Stats[4]}, Skills: n.Skills}, ok
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
