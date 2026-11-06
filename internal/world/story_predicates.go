package world

import (
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

type storyGateSource byte

const (
	storySourceInvalid        storyGateSource = 0
	storySourceMarkState      storyGateSource = 1
	storySourceMarkStep       storyGateSource = 2
	storySourceActiveMarkStep storyGateSource = 3
	storySourceMarkExists     storyGateSource = 4
	storySourceParty          storyGateSource = 5
	storySourcePartyExact     storyGateSource = 6
	storySourcePartyBroadcast storyGateSource = 7
	storySourceActiveVehicle  storyGateSource = 8
	storySourceActivePetExact storyGateSource = 9
	storySourceNPCTemplate    storyGateSource = 10
	storySourceEquipment      storyGateSource = 11
	storySourceInventory      storyGateSource = 12
)

type storyGateComparison byte

const (
	storyCompareInvalid      storyGateComparison = 0
	storyCompareEqual        storyGateComparison = 1
	storyCompareNotEqual     storyGateComparison = 2
	storyCompareLess         storyGateComparison = 3
	storyCompareLessEqual    storyGateComparison = 4
	storyCompareGreater      storyGateComparison = 5
	storyCompareGreaterEqual storyGateComparison = 6
)

type compiledStoryGate struct {
	source storyGateSource
	op     storyGateComparison
	id     uint32
	slot   byte
	value  int64
}
type storyGateProgram struct {
	groups  [][]compiledStoryGate
	invalid bool
}
type storyGatesKey struct {
	first *assets.StoryGate
	count int
}

// Story mark states use the EVE lifecycle values, independently of the
// gameplay database's QuestState enum. Presence predicates use a numeric flag.
const (
	storyMarkStateInProgress = 1
	storyMarkStateNotStarted = 2
	storyMarkStateCompleted  = 3
	storyGatePresent         = 1
)

func compileStoryGate(g assets.StoryGate) compiledStoryGate {
	out := compiledStoryGate{id: g.ID, slot: g.Slot, value: g.Value}
	switch g.Source {
	case "mark_state":
		out.source = storySourceMarkState
	case "mark_step":
		out.source = storySourceMarkStep
	case "active_mark_step":
		out.source = storySourceActiveMarkStep
	case "mark_exists":
		out.source = storySourceMarkExists
	case "party":
		out.source = storySourceParty
	case "party_exact":
		out.source = storySourcePartyExact
	case "party_broadcast":
		out.source = storySourcePartyBroadcast
	case "active_vehicle":
		out.source = storySourceActiveVehicle
	case "active_pet_exact":
		out.source = storySourceActivePetExact
	case "npc_template":
		out.source = storySourceNPCTemplate
	case "equipment":
		out.source = storySourceEquipment
	case "inventory":
		out.source = storySourceInventory
	}
	switch g.Op {
	case "eq":
		out.op = storyCompareEqual
	case "ne":
		out.op = storyCompareNotEqual
	case "lt":
		out.op = storyCompareLess
	case "le":
		out.op = storyCompareLessEqual
	case "gt":
		out.op = storyCompareGreater
	case "ge":
		out.op = storyCompareGreaterEqual
	}
	return out
}
func compileStoryGates(gates []assets.StoryGate) storyGateProgram {
	var out storyGateProgram
	groups := map[uint16]int{}
	for _, g := range gates {
		compiled := compileStoryGate(g)
		if compiled.source == storySourceInvalid {
			out.invalid = true
		}
		at, exists := groups[g.Group]
		if !exists {
			at = len(out.groups)
			groups[g.Group] = at
			out.groups = append(out.groups, nil)
		}
		out.groups[at] = append(out.groups[at], compiled)
	}
	return out
}
func (w *World) storyGateMatches(g compiledStoryGate, c *game.Character, x StoryContext) bool {
	var actual int64
	switch g.source {
	case storySourceMarkState:
		actual = storyMarkStateNotStarted
		if q, ok := c.Quests[g.id]; ok {
			if q.State == game.InProgress {
				actual = storyMarkStateInProgress
			}
			if q.State == game.Completed {
				actual = storyMarkStateCompleted
			}
		}
	case storySourceMarkStep:
		actual = int64(c.Quests[g.id].Step)
	case storySourceActiveMarkStep:
		if q, ok := c.Quests[g.id]; ok && q.State == game.InProgress {
			actual = int64(q.Step)
		}
	case storySourceMarkExists:
		if _, ok := c.Quests[g.id]; ok {
			actual = storyGatePresent
		}
	case storySourceParty:
		if _, ok := c.Pet(g.id); ok {
			actual = storyGatePresent
		}
	case storySourcePartyExact:
		for _, p := range c.Pets {
			if p.ID == g.id {
				actual = storyGatePresent
				break
			}
		}
	case storySourcePartyBroadcast:
		if w.InParty(c, g.id) {
			actual = storyGatePresent
		}
	case storySourceActiveVehicle:
		actual = int64(c.ActiveVehicle)
	case storySourceActivePetExact:
		actual = int64(c.ActivePet)
	case storySourceNPCTemplate:
		if n, ok := w.NPC(x.Map, x.Actor); ok {
			actual = int64(n.Template)
		}
	case storySourceEquipment:
		if i, ok := c.Pet(g.id); ok && int(g.slot) < len(c.Pets[i].Equipment) {
			actual = int64(c.Pets[i].Equipment[g.slot].ID)
		}
	case storySourceInventory:
		for _, it := range c.Bag {
			if uint32(it.ID) == g.id {
				actual += int64(it.Count)
			}
		}
	default:
		return false
	}
	switch g.op {
	case storyCompareEqual:
		return actual == g.value
	case storyCompareNotEqual:
		return actual != g.value
	case storyCompareLess:
		return actual < g.value
	case storyCompareLessEqual:
		return actual <= g.value
	case storyCompareGreater:
		return actual > g.value
	case storyCompareGreaterEqual:
		return actual >= g.value
	}
	return false
}
func (w *World) storyProgramMatches(p *storyGateProgram, c *game.Character, x StoryContext) bool {
	if p.invalid {
		return false
	}
	if len(p.groups) == 0 {
		return true
	}
	if c == nil {
		return false
	}
	for _, group := range p.groups {
		matched := true
		for _, g := range group {
			if !w.storyGateMatches(g, c, x) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// StoryGates reuses snapshot-compiled predicates, including when called inside
// a resource transaction. Character values are always read fresh. Ad hoc Go
// predicates retain a nonallocating fallback and are never cached by result.
func (w *World) StoryGates(gates []assets.StoryGate, c *game.Character, v *View, x StoryContext) bool {
	if len(gates) == 0 {
		return true
	}
	if c == nil {
		return false
	}
	if program := w.story.gates[storyGatesKey{&gates[0], len(gates)}]; program != nil {
		return w.storyProgramMatches(program, c, x)
	}
	for _, g := range gates {
		if compileStoryGate(g).source == storySourceInvalid {
			return false
		}
	}
	for i, g := range gates {
		seen := false
		for _, prev := range gates[:i] {
			if prev.Group == g.Group {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		matched := true
		for _, candidate := range gates {
			if candidate.Group == g.Group && !w.storyGateMatches(compileStoryGate(candidate), c, x) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
