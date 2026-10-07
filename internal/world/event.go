package world

import (
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// Op is a decoded EVE action (EventSubSubEntry): Code is DialogPtr.
type Op struct {
	Index, Code    byte
	D1, D2, D3, D4 uint16
	DW1, DW2, DW3  uint32
	raw            assets.Operation
}

func DecodeOp(o assets.Operation) Op {
	d := o.Data[:]
	dw := func(i int) uint32 { return uint32(d[i]) | uint32(d[i+1])<<8 | uint32(d[i+2])<<16 | uint32(d[i+3])<<24 }
	return Op{Index: o.Index, Code: d[0], D1: le16(d, 1), D2: le16(d, 3), D3: le16(d, 5), D4: le16(d, 7), DW1: dw(9), DW2: dw(13), DW3: dw(17), raw: o}
}

// Value is DecodeActionValue: an amount packed across dialog4's high byte and dword1.
func (o Op) Value() uint32 { return uint32(o.D4>>8) | o.DW1<<8 }

// Cond is a decoded branch condition (EventSubEntry header).
type Cond struct {
	Kind               byte
	W1, W2, W3, W4, W5 uint16
	W6                 uint16
}

func DecodeCond(d [21]byte) Cond {
	return Cond{d[0], le16(d[:], 1), le16(d[:], 3), le16(d[:], 5), le16(d[:], 7), le16(d[:], 9), le16(d[:], 11)}
}

// Callback reports branches entered only from battle, choice or minigame outcomes.
func (c Cond) Callback() bool {
	return c.Kind == ConditionBattleResult || c.Kind == ConditionChoiceResult || c.Kind == ConditionMinigameResult
}

// ExtraConditions is the number of condition-only branches following this one.
func (c Cond) ExtraConditions() int { return max(1, int(c.W6>>8)) - 1 }

// Event returns a map event by ID.
func (w *World) Event(mapID, id uint16) (*assets.Event, bool) {
	m, ok := w.maps[mapID]
	if !ok {
		return nil, false
	}
	for i := range m.data.Events {
		if m.data.Events[i].ClickID == id {
			return &m.data.Events[i], true
		}
	}
	return nil, false
}

func hasActions(ev *assets.Event) bool {
	for _, br := range ev.Branches {
		if len(br.Operations) > 0 {
			return true
		}
	}
	return false
}

// NPCEvents lists the events an actor click may run: its linked events in EVE order, or
// the event with the same ID when it links none. Reference: EveEventInterpreter.TryExecute.
func (w *World) NPCEvents(mapID, click uint16) []*assets.Event {
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	var out []*assets.Event
	add := func(id uint16) {
		ev, ok := w.Event(mapID, id)
		if !ok || len(ev.Branches) == 0 {
			return
		}
		for _, have := range out {
			if have == ev {
				return
			}
		}
		out = append(out, ev)
	}
	for _, n := range m.data.NPCs {
		if n.ClickID == click {
			for _, id := range n.Events {
				add(uint16(id))
			}
			break
		}
	}
	if len(out) == 0 {
		add(click)
	}
	return out
}

// RoamingBattle is IsRoamingBattleNpc: a field actor (flag 5) linked to an encounter.
func (w *World) RoamingBattle(mapID, click uint16) bool {
	m, ok := w.maps[mapID]
	if !ok {
		return false
	}
	for _, n := range m.data.NPCs {
		if n.ClickID != click {
			continue
		}
		if n.Flags != 5 {
			return false
		}
		for _, id := range n.Events {
			if ev, ok := w.Event(mapID, uint16(id)); ok && ev.Kind == 10 {
				for _, br := range ev.Branches {
					for _, o := range br.Operations {
						if op := DecodeOp(o); op.Code == ActionBattle && op.D1 == 1 {
							return true
						}
					}
				}
			}
		}
		return false
	}
	return false
}

// FindBranch is EveEventRuntime.FindBranch. trigger 0 selects a normal entry branch by
// its conditions; 4, 7 and 8 select battle, choice and minigame callbacks by source and
// result. Selection resumes after exclude for choice continuations. It returns -1 when
// no branch applies. Scoped story hand-ins and forward-only continuations follow
// the reference controller ordering.
func (w *World) FindBranch(c *game.Character, v *View, mapID uint16, ev *assets.Event, trigger byte, question, answer uint16, exclude int) int {
	return w.FindBranchAt(c, v, mapID, ev, trigger, question, answer, exclude, time.Now())
}

// FindBranchAt evaluates every timer in the branch against one instant.
func (w *World) FindBranchAt(c *game.Character, v *View, mapID uint16, ev *assets.Event, trigger byte, question, answer uint16, exclude int, now time.Time) int {
	first := 0
	if exclude >= 0 && (DecodeCond(ev.Branches[exclude].Condition).Kind == ConditionChoiceResult || storyForwardOnly(mapID, ev.ClickID)) {
		first = exclude + 1
	}
	if mapID == storyXaolanHomeMap && (ev.ClickID == storyXaolanHomeFirst || ev.ClickID == storyXaolanHomeSecond) && xaolanFate(c) {
		return -1
	}
	order := make([]int, 0, len(ev.Branches))
	// Native voucher hand-ins precede the broad greeting and transformation offer.
	if trigger == TriggerEntry {
		for i := first; i < len(ev.Branches); i++ {
			if _, ok := BreillatExchange(ev, i); ok {
				order = append(order, i)
			}
		}
	}
	// The broad greeting condition precedes the unlock condition in native data.
	if trigger == TriggerEntry && IsBreillat(ev) && first <= BreillatOfferBranch-1 {
		order = append(order, BreillatOfferBranch-1)
	}
	preferred := storyPreferredBranch(mapID, ev.ClickID)
	if trigger == TriggerEntry && preferred != 0 {
		for i := first; i < len(ev.Branches); i++ {
			if ev.Branches[i].Index == preferred {
				order = append(order, i)
			}
		}
	}
	for i := first; i < len(ev.Branches); i++ {
		order = append(order, i)
	}
	for _, i := range order {
		br := ev.Branches[i]
		if i == exclude || len(br.Operations) == 0 {
			continue
		}
		cond := DecodeCond(br.Condition)
		if trigger == TriggerEntry && cond.Callback() || trigger != TriggerEntry && cond.Kind != trigger {
			continue
		}
		if trigger != TriggerEntry {
			if cond.W1 != question {
				continue
			}
			if trigger == ConditionMinigameResult {
				if !compare(int64(answer), int64(cond.W4>>8), byte(cond.W4)) {
					continue
				}
			} else if cond.W2 != answer {
				continue
			}
		}
		matches := trigger != TriggerEntry || w.conditionAt(c, v, mapID, ev, br.Condition, now)
		extra := cond.ExtraConditions()
		if i+extra >= len(ev.Branches) {
			continue
		}
		for j := i + 1; j <= i+extra; j++ {
			if len(ev.Branches[j].Operations) > 0 || !w.conditionAt(c, v, mapID, ev, ev.Branches[j].Condition, now) {
				matches = false
			}
		}
		if matches {
			return i
		}
	}
	return -1
}

// Speech reports whether an action is a dialogue line the client acknowledges.
func (o Op) PlayerSpeech() bool {
	return o.Code == ActionPlayer && o.D1 == PlayerActionSpeech && o.D2 >= MinDialogueID
}
func (o Op) NPCSpeech() bool {
	return o.Code == ActionActor && (o.D2 == ActorActionSpeech || o.D2 == ActorActionSpeechAlternate) && o.D3 >= MinDialogueID
}

// Choice reports a player (1/4) or actor (2/6) question.
func (o Op) Choice() bool {
	return (o.Code == ActionPlayer && o.D1 == PlayerActionChoice) || (o.Code == ActionActor && o.D2 == ActorActionChoice)
}

// NPCPath is an actor walking an authored path (2/9 with a mode of 1..3).
func (o Op) NPCPath() bool {
	return o.Code == ActionActor && o.D2 == ActorActionPath && o.D1 != 0 && o.D3 >= 1 && o.D3 <= 3
}

// ClientStep is IsClientStep: actions that wait for the client before continuing.
func (o Op) ClientStep() bool {
	return (o.Code == ActionPlayer && (o.D1 == PlayerActionChoice || o.D1 == PlayerActionAnimation || o.D1 == PlayerActionEffect || (o.D1 == PlayerActionSpeech && o.D2 >= MinDialogueID))) ||
		(o.Code == ActionActor && (o.D2 == ActorActionSpeech || o.D2 == ActorActionSpeechAlternate || o.D2 == ActorActionChoice || o.D2 == ActorActionPath)) ||
		o.Code == ActionBattle || o.Code == ActionBattleAlternate || o.Code == ActionServiceOrTeleport || o.Code == ActionMovie || o.Code == ActionMinigame || o.Code == ActionEffect
}

// FullRecovery is the 1/1/3 (HP) or 1/1/4 (SP) restore with a zero operand.
func (o Op) FullRecovery() bool {
	return o.Code == ActionPlayer && o.D1 == PlayerActionReward && (o.D2 == 3 || o.D2 == 4) && o.D3 == 1 && o.Value() == 0
}

// Unsupported reports actions this server cannot perform, so a branch containing one is
// refused before any change. It is UnsupportedRewardAction plus every action whose
// subsystem is not ported: unknown companion actions (3), invalid minigame types (9),
// standalone gathering timers (14), transformations other than Breillat (12) and water rewards.
// Verified complete water branches are intercepted atomically before this ordinary
// action executor; recognizing them does not enable individual timer/reward ops.
func (o Op) Unsupported() bool {
	switch o.Code {
	case ActionCompanion:
		return o.D1 != 1 && o.D1 != 2 && o.D1 != 5
	case ActionMinigame:
		return o.D1 == 0 || o.D1 > 255
	case ActionTransform:
		return !o.BreillatTransform()
	case ActionWireCode10, ActionGather, ActionWireCode16, ActionWireCode17:
		return true
	case ActionLearnSkill:
		return o.D1 == 0 || o.D2 != 1 || o.D3 != 0 || o.Value() != 1
	case ActionMinimapMarker:
		return o.D1 < 1 || o.D1 > 3 || o.D3 < 1 || o.D3 > 256
	case ActionEffect:
		return o.D1 != 1 || o.D2 != 1 || o.D3 != 0 || o.Value() == 0 || o.Value() > 9999
	case ActionPlayer:
		return o.D1 == 1 && !o.FullRecovery() && o.D2 != 1 && o.D2 != 2 && o.D2 != 5 && o.D2 != 7
	}
	return false
}

// Formation lists the enemy templates of a map battle entry (EVE category 8): each
// 3-byte member starts with a template ID; IDs below 10000 are not monsters.
func (w *World) Formation(mapID, entry uint16) []uint32 {
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	for _, b := range m.data.Battles {
		if b.ClickID != entry {
			continue
		}
		var out []uint32
		for i := 0; i+3 <= len(b.Team1); i += 3 {
			if id := uint32(b.Team1[i]) | uint32(b.Team1[i+1])<<8; id >= MinMonsterTemplateID {
				out = append(out, id)
			}
		}
		return out
	}
	return nil
}

// Disabled reports events the data set marks unavailable.
func (w *World) Disabled(mapID, event uint16) bool {
	_, off := w.catalog.DisabledEvents[assets.EventKey(mapID, event)]
	return off
}

// Area is an EVE entry record (category 1): a cell rectangle (20 px cells) linked to
// events. Kind 1 is a region the player walks into; kind 2 is a door.
type Area struct {
	ClickID        uint16
	Kind           byte
	X1, Y1, X2, Y2 int32
	Events         []byte
}

func areaOf(e assets.AreaEntry) Area {
	a := Area{ClickID: e.ClickID, X1: int32(e.X), Y1: int32(e.Y), Events: e.Events}
	if len(e.Tail) >= 10 {
		t := e.Tail
		a.X2 = int32(uint32(t[1]) | uint32(t[2])<<8 | uint32(t[3])<<16 | uint32(t[4])<<24)
		a.Y2 = int32(uint32(t[5]) | uint32(t[6])<<8 | uint32(t[7])<<16 | uint32(t[8])<<24)
		a.Kind = t[9]
	}
	return a
}

// Inside is InsideEntry: the position's cell, optionally widened by margin cells.
// EVE cells count from 1, and the area spans |X2 - X1| + 1 cells from X1, as the
// client's scene loader builds it (FUN_003090f4: (X1 - 1) * 20 wide
// (|X2 - X1| + 1) * 20), so the client's 20/8 and the server agree on the area.
func (a Area) Inside(x, y uint16, margin int32) bool {
	cx, cy := int32(x)/RegionCellPixels+1, int32(y)/RegionCellPixels+1
	x2, y2 := a.X1+absCells(a.X2-a.X1), a.Y1+absCells(a.Y2-a.Y1)
	return cx >= a.X1-margin && cx <= x2+margin && cy >= a.Y1-margin && cy <= y2+margin
}

func absCells(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// Entry is the region or door with this ID (TryExecuteEntry's lookup).
func (w *World) Entry(mapID, id uint16) (Area, bool) {
	if m, ok := w.maps[mapID]; ok {
		for _, e := range m.data.Entries {
			if a := areaOf(e); a.ClickID == id && (a.Kind == RegionEntry || a.Kind == RegionDoor) {
				return a, true
			}
		}
	}
	return Area{}, false
}

// Regions lists a map's walk-in regions in EVE order.
func (w *World) Regions(mapID uint16) []Area {
	var out []Area
	if m, ok := w.maps[mapID]; ok {
		for _, e := range m.data.Entries {
			if a := areaOf(e); a.Kind == RegionEntry {
				out = append(out, a)
			}
		}
	}
	return out
}

// PreEvents lists the map's PreEvents; story arrivals run some of them as events.
func (w *World) PreEvents(mapID uint16) []*assets.Event {
	m, ok := w.maps[mapID]
	if !ok {
		return nil
	}
	out := make([]*assets.Event, 0, len(m.data.PreEvents))
	for i := range m.data.PreEvents {
		out = append(out, &m.data.PreEvents[i])
	}
	return out
}
