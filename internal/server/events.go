package server

import (
	"context"
	"fmt"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// eventSession is one running EVE branch (C# NativeEventActive with its token). The
// session's pointer is the token: callbacks of a replaced session do nothing.
// Reference: EveEventRuntime.StartSession and EveEventInterpreter.ExecuteOpcode.
type eventSession struct {
	chestReward  *assets.ChestReward
	mapID, click uint16
	ev           *assets.Event
	branch       int
	index        int
	transition   bool
	onComplete   func() error
	onChoice     func(byte) error
	onMinigame   func(byte) error
}

const clickResume = 1500 * time.Millisecond

// eventFrame is BuildEventFrame, the native AC20:1 layout: type, subject, actor, mode,
// value, text or question, branch.
func eventFrame(kind, subject byte, actor uint16, mode byte, value uint32, text uint16, step, branch byte) []byte {
	return protocol.Builder{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, step, kind, subject}.U16(actor).U8(mode).U32(value).U16(text).U8(branch)
}

// dialogueFrame is the legacy single-line dialogue packet with a 24-bit talk ID.
func dialogueFrame(step, portrait, speaker byte, talk uint32) []byte {
	return []byte{protocol.CommandEvent, protocol.EventFrame, 0, 0, 0, step, 1, portrait, speaker, 0, 1, 0, 0, 0, 0, byte(talk), byte(talk >> 8), byte(talk >> 16)}
}

func recordPointStatus(char game.Character) []byte {
	if char.RecordPoint != nil && char.RecordPoint.Map != 0 {
		return []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode21, 1}
	}
	return []byte{protocol.CommandCharacterState, protocol.CharacterStateWireCode21, 2}
}

func (s *Server) sendAll(c *Session, packets [][]byte) error {
	for _, p := range packets {
		if e := c.send(p); e != nil {
			return e
		}
	}
	return nil
}

// commit persists a changed copy of the character, then adopts it in the session.
func (s *Server) commit(ctx context.Context, c *Session, next game.Character) error {
	old := *c.character
	if err := s.commitState(ctx, c, next); err != nil {
		return err
	}
	if old.ActiveVehicle != 0 && (old.ActiveVehicle != c.character.ActiveVehicle || old.VehicleSlot != c.character.VehicleSlot) {
		return s.sendVehiclePackets(c, [][]byte{vehicleDismountPacket(&old)})
	}
	return nil
}

// commitState saves and adopts state without publishing packets. The raft wreck
// path supplies its own break/dismount order. Callers hold worldMu.
func (s *Server) commitState(ctx context.Context, c *Session, next game.Character) error {
	if err := game.PreserveItemLocks(*c.character, &next); err != nil {
		return err
	}
	next.NormalizeVehicle(s.Assets.Items)
	if e := s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(stored *game.Character) error {
		*stored = next
		return nil
	}); e != nil {
		return e
	}
	changedOfferState := c.character.Bag != next.Bag || c.character.Gold != next.Gold
	// This transaction saved the complete supplied snapshot, including its
	// position. Unlike a database result that retains the old position, an
	// explicit full-state commit must adopt its destination exactly.
	*c.character = next
	baseline := next.Clone()
	c.autosaveBaseline = &baseline
	if changedOfferState {
		s.closeStall(c)
		s.cancelTrade(c)
	}
	return nil
}

// endEvent is Player.ClearInteraction.
func (s *Server) endEvent(c *Session) { c.event = nil; c.arcade = nil }

// cancelInteraction is Player.CancelInteraction: release movement and the dialogue lock.
func (s *Server) cancelInteraction(c *Session) error {
	s.endEvent(c)
	c.resumeAt = time.Now().Add(clickResume)
	return s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}})
}

func (s *Server) finishEvent(c *Session, es *eventSession) error {
	if c.event != es {
		return nil
	}
	if e := s.cancelInteraction(c); e != nil {
		return e
	}
	if err := s.sendAll(c, s.World.Sync(c.character, c.view, false)); err != nil {
		return err
	}
	return s.syncMapProps(context.Background(), c, time.Now())
}

func (s *Server) rejectDisabled(c *Session) error {
	if e := s.cancelInteraction(c); e != nil {
		return e
	}
	return c.send(headBanner("This quest is disabled: required game data is unavailable."))
}

// eventCommand handles AC20:1 clicks, AC20:6 acknowledgments and AC20:9 choices.
// The caller holds worldMu.
func (s *Server) eventCommand(ctx context.Context, c *Session, p []byte) error {
	switch p[1] {
	case protocol.EventActorClick:
		return s.npcClick(ctx, c, p[2:])
	case protocol.EventAcknowledge:
		if c.resultAck {
			c.resultAck = false
			return nil // The minigame result's own acknowledgment.
		}
		if c.storm {
			return s.endStorm(ctx, c)
		}
		if c.beach != nil {
			if c.beach.step >= 2 {
				return s.advanceBeach(ctx, c, false)
			}
			return nil // Acknowledgments before the wake-up animation are absorbed.
		}
		es := c.event
		if es == nil {
			return s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}})
		}
		if es.onChoice != nil || es.onMinigame != nil {
			return nil // The question stays open until AC20:9.
		}
		if next := es.onComplete; next != nil {
			es.onComplete = nil
			return next()
		}
		return nil
	case protocol.EventRegion:
		return s.regionRequest(ctx, c, p[2:])
	case protocol.EventChoice:
		if len(p) != 3 {
			return nil
		}
		if p[2] == 40 {
			return s.cancelInteraction(c)
		}
		if es := c.event; es != nil && es.onChoice != nil {
			choose := es.onChoice
			es.onChoice = nil
			return choose(p[2])
		}
		return nil
	}
	return ErrUnsupported
}

// npcClick is AC20.Recv1 with GameMap.ProcessInteraction and the EVE part of
// QuestNpc.Interact, including services for actors without a native event.
func (s *Server) npcClick(ctx context.Context, c *Session, data []byte) error {
	if c.event != nil {
		return nil
	}
	var click uint16
	switch {
	case len(data) >= 4:
		click = uint16(data[2]) | uint16(data[3])<<8
	case len(data) >= 2:
		click = uint16(data[0]) | uint16(data[1])<<8
	case len(data) == 1:
		click = uint16(data[0])
	}
	char := c.character
	release := func() error { return c.send([]byte{protocol.CommandEvent, protocol.EventResume}) }
	// Closing a native choice can queue a second click on the actor below it.
	if click == c.lastClick && char.Map == c.lastClickMap && time.Now().Before(c.resumeAt) {
		return release()
	}
	c.lastClick, c.lastClickMap = click, char.Map
	c.restMap, c.saleMode = 0, -1
	if c.view.Hidden[click] || !s.World.VisibleIn(char, c.view, char.Map, click) {
		if e := c.send(s.World.HideActor(c.view, char.Map, click)); e != nil {
			return e
		}
		return release()
	}
	m, ok := s.World.Map(char.Map)
	if !ok {
		return release()
	}
	npc, found := s.World.NPC(char.Map, click)
	if found {
		dx, dy := int64(char.X)-int64(npc.X), int64(char.Y)-int64(npc.Y)
		found = dx*dx+dy*dy <= npcInteractionRangePixels*npcInteractionRangePixels
	}
	if found {
		if handled, e := s.wildClick(c, npc); handled || e != nil {
			return e
		}
		handled, e := s.runNPCEvent(ctx, c, click)
		if e != nil || handled {
			return e
		}
		if handled, e := s.fallbackService(ctx, c, npc); handled || e != nil {
			return e
		}
		if handled, err := s.harvestProp(ctx, c, npc); handled || err != nil {
			return err
		}
		return s.npcGreeting(c, npc)
	}
	// A door object outside interaction range still opens its linked portal.
	if portal, ok := m.DoorPortal(click); ok {
		return s.enterPortal(ctx, c, portal)
	}
	return release()
}

// runNPCEvent is EveEventInterpreter.TryExecute's native-event path.
func (s *Server) runNPCEvent(ctx context.Context, c *Session, click uint16) (bool, error) {
	mapID := c.character.Map
	if handled, err := s.storyClick(ctx, c, click); handled || err != nil {
		return handled, err
	}
	events := s.robinsonRecoveryEvents(c.character, click, s.World.NPCEvents(mapID, click))
	if len(events) == 0 {
		return false, nil
	}
	disabled := true
	for _, ev := range events {
		disabled = disabled && s.World.Disabled(mapID, ev.ClickID)
	}
	if disabled {
		return true, s.rejectDisabled(c)
	}
	if n, ok := s.World.NPC(mapID, click); ok && (n.Template == monkeyTemplate || mapID == monkeyMap && click == monkeyActor) {
		return true, s.startMonkey(ctx, c, click)
	}
	for _, ev := range events {
		if i := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); i >= 0 {
			return true, s.startEvent(ctx, c, click, ev, i, true)
		}
	}
	return true, c.send([]byte{protocol.CommandEvent, protocol.EventResume})
}

// startEvent is StartSession: refuse unsupported actions before any change, lock the
// player, then run actions until one waits for the client.
func (s *Server) startEvent(ctx context.Context, c *Session, click uint16, ev *assets.Event, branch int, transition bool) error {
	mapID := c.character.Map
	if s.World.Disabled(mapID, ev.ClickID) {
		return s.rejectDisabled(c)
	}
	if handled, err := s.tryWaterGathering(ctx, c, click, ev, branch, time.Now()); handled {
		return err
	}
	ev = world.PrepareStory(mapID, world.PrepareBreillat(ev, branch), branch)
	s.endEvent(c)
	es := &eventSession{mapID: mapID, click: click, ev: ev, branch: branch, transition: transition}
	c.event = es
	if e := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); e != nil {
		return e
	}
	for _, o := range ev.Branches[branch].Operations {
		if op := world.DecodeOp(o); op.Unsupported() || (op.Code == world.ActionTransform && (!world.IsBreillat(ev) || ev.Branches[branch].Index != world.BreillatAcceptanceBranch)) || (op.Code == world.ActionLearnSkill && !s.hasSkill(op.D1)) {
			s.Log.Info("unsupported quest action", "map", mapID, "event", ev.ClickID, "branch", ev.Branches[branch].Index, "step", op.Index, "opcode", op.Code)
			if e := c.send(headBanner("This quest action is not supported yet. Please report this quest.")); e != nil {
				return e
			}
			return s.finishEvent(c, es)
		}
	}
	return s.advance(ctx, c, es)
}

func singleChoiceAck(mapID, event uint16) bool {
	switch mapID {
	case game.MapID11005:
		return event == 25 || event == 26
	case game.MapID11040:
		return event == 2 || event == 8 || event == 9
	case game.MapID12211:
		return event == 1
	case game.MapID11149:
		return event == 1 || event == 6
	case game.MapID11075:
		return event == 1 || event == 2
	}
	return false
}

// movieWithoutHold lists movies that must not hold rendering for a map load (mode 1).
func movieWithoutHold(mapID, event uint16) bool {
	switch mapID {
	case game.MapID11039, game.MapID11052:
		return true
	case game.MapID60014:
		return event == 54
	case game.MapID11149:
		return event == 1 || event == 2 || event == 5
	case game.MapID11185:
		return event == 4
	case game.MapID11075:
		return event == 1 || event == 2
	}
	return false
}

// cliveAction is IsCliveActorAction: verified actor animation, expression, path-wait and
// constellation actions on specific story maps.
func cliveAction(mapID uint16, op world.Op) bool {
	switch {
	case op.Code != world.ActionActor:
		return false
	case mapID == game.MapID11185 && op.D2 == 11 && op.D1 == 1 && op.D3 == 1 && op.Value() == 8:
		return true
	case mapID == game.MapID11149 && ((op.D2 == 11 && op.D1 == 3 && op.D3 == 1 && op.Value() == 6) || (op.D2 == 4 && op.D1 == 4 && op.D3 == 8)):
		return true
	case mapID == game.MapID11148 && op.D2 == 8 && (op.D1 == 1 || op.D1 == 2) && op.D3 == 7:
		return true
	}
	switch mapID {
	case game.MapID11039, game.MapID11049, game.MapID11050, game.MapID11052, game.MapID12211:
		return op.D2 == 4 || op.D2 == 8 || op.D2 == 10 || (mapID == game.MapID12211 && op.D2 == 11 && op.D1 == 1 && op.D3 == 1 && op.Value() == 20)
	}
	return false
}

// canDeliver is CanDeliverPendingRewards: every reward before the next client step must
// fit before the first of them is given.
func (s *Server) canDeliver(c *Session, es *eventSession) bool {
	ops := es.ev.Branches[es.branch].Operations
	gold := int64(c.character.Gold)
	var changes []game.ItemChange
	chestPlanned := false
	party := make([]uint32, 0, len(c.character.Pets))
	for _, p := range c.character.Pets {
		party = append(party, p.ID)
	}
	reserve := make([]uint32, 0, len(c.character.ReservePets))
	for _, p := range c.character.ReservePets {
		reserve = append(reserve, p.ID)
	}
	inList := func(list []uint32, id uint32) int {
		for i, p := range list {
			if game.SamePet(p, id) {
				return i
			}
		}
		return -1
	}
	for _, o := range ops[es.index:] {
		op := world.DecodeOp(o)
		if op.ClientStep() {
			break
		}
		if op.Unsupported() {
			return false
		}
		if op.Code == world.ActionLearnSkill && !s.hasSkill(op.D1) {
			return false
		}
		if op.Code == world.ActionCompanion {
			// Companion changes must fit the party and reserve as planned so far.
			id := uint32(op.D2)
			present := inList(party, id) >= 0
			t, known := s.World.Template(game.BroadcastID(id))
			switch op.D1 {
			case 1:
				if !present {
					for _, pet := range c.character.HotelPets {
						if game.SamePet(pet.ID, id) {
							return false
						}
					}
					if (game.SamePet(id, 14156) && xaolanInFate(c.character)) || len(party) >= game.MaxPets || !known {
						return false
					}
					party = append(party, id)
					if i := inList(reserve, id); i >= 0 {
						reserve = append(reserve[:i], reserve[i+1:]...)
					}
				}
			case 2:
				if present && game.StoryCompanion(id, t, known) {
					if len(reserve) >= 255 || inList(reserve, id) >= 0 {
						return false
					}
					reserve = append(reserve, id)
				}
				if i := inList(party, id); i >= 0 {
					party = append(party[:i], party[i+1:]...)
				}
			case 5:
				if !present {
					return false
				}
			}
			continue
		}
		if op.Code != world.ActionPlayer || op.D1 != 1 {
			continue
		}
		amount := int32(op.Value())
		switch {
		case op.D2 == 1 || op.D2 == 7:
			if amount > 0 && world.DecodeCond(es.ev.Branches[es.branch].Condition).Kind == 3 {
				if pool := s.chestPool(es.mapID, es.click); pool != nil {
					expiry := c.character.ChestRespawns[world.ChestKey(es.mapID, es.ev.ClickID)]
					if chestPlanned || (!expiry.IsZero() && time.Now().Before(expiry)) {
						continue
					}
					chestPlanned = true
					if es.chestReward == nil {
						reward := rollChestReward(*pool)
						es.chestReward = &reward
					}
					op.D3, amount = es.chestReward.Item, int32(es.chestReward.Count)
				}
			}
			if op.D3 == 0 {
				return false
			}
			if amount != 0 {
				if xaolanRobberyReward(es.mapID, es.ev.Branches[es.branch], op) && c.character.Quests[xaolanRobberyCompleted].State == game.Completed {
					continue
				}
				changes = append(changes, game.ItemChange{ID: op.D3, Count: int(amount)})
				if xaolanRobberyReward(es.mapID, es.ev.Branches[es.branch], op) {
					changes = append(changes, game.ItemChange{ID: xaolanRobberyStar, Count: xaolanRobberyStarCount})
				}
			}
		case op.D2 == 2 && op.D3 == 0:
			gold += int64(amount)
			if gold < 0 || gold > game.MaxGold {
				return false
			}
		case op.D2 == 5 && op.D3 == 1:
			if amount < 0 {
				return false
			}
		case op.FullRecovery():
		default:
			return false
		}
	}
	if len(changes) == 0 {
		return true
	}
	bag := c.character.Bag
	_, e := bag.ApplyQuestItems(changes, s.stackLimit, s.Assets.Items)
	return e == nil
}

func (s *Server) advance(ctx context.Context, c *Session, es *eventSession) error {
	if c.event != es {
		return nil
	}
	if !s.canDeliver(c, es) {
		if e := c.send(headBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")); e != nil {
			return e
		}
		return s.finishEvent(c, es)
	}
	br := es.ev.Branches[es.branch]
	resume := func() error { return s.advance(ctx, c, es) }
	for es.index < len(br.Operations) {
		op := world.DecodeOp(br.Operations[es.index])
		es.index++
		if op.Code == world.ActionMinigame {
			return s.startMinigame(ctx, c, es, op)
		}
		if op.Choice() {
			player := op.Code == world.ActionPlayer
			question, subject, actor := op.D3, byte(3), op.D1
			if player {
				question, subject, actor = op.D2, 7, 0
			}
			es.onChoice = func(choice byte) error {
				if c.event != es {
					return nil
				}
				if choice == 40 {
					return s.finishEvent(c, es)
				}
				next := s.World.FindBranch(c.character, c.view, es.mapID, es.ev, world.ConditionChoiceResult, question, uint16(choice), -1)
				if next < 0 {
					return s.finishEvent(c, es)
				}
				return s.startEvent(ctx, c, es.click, es.ev, next, true)
			}
			// Mode 0 also closes with AC20:6; mode 1 reports only the choice.
			mode := byte(0)
			if singleChoiceAck(es.mapID, es.ev.ClickID) {
				mode = 1
			}
			return c.send(eventFrame(6, subject, actor, mode, 0, question, op.Index, br.Index))
		}
		if op.NPCPath() {
			es.onComplete = resume
			return s.sendPathGroup(c, es, op)
		}
		speech := op.PlayerSpeech() || op.NPCSpeech()
		path := op.Code == world.ActionActor && (op.D2 == 9 || (cliveAction(es.mapID, op) && (op.D2 == 10 || op.D2 == 11)))
		movie := op.Code == world.ActionMovie && (op.D1 == 1 || op.D1 == 2)
		music := op.Code == world.ActionEffect
		effect := op.Code == world.ActionPlayer && (op.D1 == 6 || op.D1 == 7)
		if speech || path || movie || music || effect {
			es.onComplete = resume
			switch {
			case op.PlayerSpeech():
				return c.send(eventFrame(1, 7, 0, 1, 0, op.D2, op.Index, br.Index))
			case op.NPCSpeech():
				return c.send(eventFrame(1, 3, op.D1, 1, 0, op.D3, op.Index, br.Index))
			case movie && es.mapID == 11005 && (es.ev.ClickID == 25 || es.ev.ClickID == 26) && op.Value() == 25010 && laterMovie(br.Operations[es.index:], 156):
				// Holding the first of these movies would stop the second from playing.
				return c.send(eventFrame(5, 0, 0, 1, 25010, 0, op.Index, br.Index))
			case movie && movieWithoutHold(es.mapID, es.ev.ClickID):
				return c.send(eventFrame(5, 0, 0, 1, op.Value(), 0, op.Index, br.Index))
			}
			ok, e := s.execute(ctx, c, es, op)
			if e != nil {
				return e
			}
			if !ok {
				return s.finishEvent(c, es)
			}
			return nil
		}
		ok, e := s.execute(ctx, c, es, op)
		if e != nil {
			return e
		}
		if !ok || op.Code == world.ActionServiceOrTeleport {
			return s.finishEvent(c, es)
		}
		if op.Code == world.ActionBattle || op.Code == world.ActionBattleAlternate {
			return nil // The battle outcome owns the next branch.
		}
		if c.event != es || es.onComplete != nil {
			return nil // Teleported, cancelled, or waiting for the client.
		}
	}
	if world.IsBreillat(es.ev) && (br.Index == world.BreillatInitialBranch || br.Index == world.BreillatGreetingBranch) {
		if next := s.World.FindBranch(c.character, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, -1); next >= 0 && es.ev.Branches[next].Index == world.BreillatOfferBranch {
			return s.startEvent(ctx, c, es.click, es.ev, next, false)
		}
	}
	if handled, err := s.storyContinuation(ctx, c, es); handled || err != nil {
		return err
	}
	// A pure mark change may enable the next branch of the same event.
	if es.transition {
		pure := true
		for _, o := range br.Operations {
			if op := world.DecodeOp(o); op.Code != world.ActionQuestMark && op.Code != world.ActionCompanion {
				pure = false
			}
		}
		if pure {
			if next := s.World.FindBranch(c.character, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, es.branch); next >= 0 {
				return s.startEvent(ctx, c, es.click, es.ev, next, false)
			}
		}
	}
	return s.finishEvent(c, es)
}

func laterMovie(ops []assets.Operation, id uint32) bool {
	for _, o := range ops {
		if op := world.DecodeOp(o); op.Code == world.ActionMovie && op.Value() == id {
			return true
		}
	}
	return false
}

// sendPathGroup is SendNpcPathGroup: consecutive path legs of one group start together,
// at most ten actors and never two legs for the same actor.
func (s *Server) sendPathGroup(c *Session, es *eventSession, op world.Op) error {
	ops := es.ev.Branches[es.branch].Operations
	branch := es.ev.Branches[es.branch].Index
	group := op.DW1 >> 24
	actors := map[uint16]bool{}
	var packets [][]byte
	for {
		actors[op.D1] = true
		packets = append(packets, eventFrame(4, 3, op.D1, byte(op.D3), op.Value(), 0, op.Index, branch))
		if group == 0 || len(actors) == 10 || es.index == len(ops) {
			break
		}
		next := world.DecodeOp(ops[es.index])
		if !next.NPCPath() || next.DW1>>24 != group || actors[next.D1] {
			break
		}
		op = next
		es.index++
	}
	return s.sendAll(c, packets)
}

// replacementHeld is HasReplacementQuestItem: a lost quest item is reissued only when
// the branch's own "count == 0" condition is no longer true.
func replacementHeld(c *game.Character, ev *assets.Event, branch int, item uint16) bool {
	for i := branch + 1; i < len(ev.Branches); i++ {
		if len(ev.Branches[i].Operations) > 0 {
			break
		}
		k := world.DecodeCond(ev.Branches[i].Condition)
		if k.Kind == 2 && k.W1 == 1 && k.W2 == 1 && k.W3 == item && k.W4 == 5 && k.W5 == 0 && k.W6&255 == 0 {
			for _, it := range c.Bag {
				if it.ID == item && !it.Empty() {
					return true
				}
			}
		}
	}
	return false
}

func (s *Server) itemName(id uint16) string {
	if d, ok := s.Assets.Items[id]; ok && d.Name != "" {
		return d.Name
	}
	return fmt.Sprintf("Item #%d", id)
}

func freeSlots(c *game.Character) int {
	n := 0
	for _, it := range c.Bag {
		if it.ID == 0 {
			n++
		}
	}
	return n
}

// fallbackDialogue sends an untyped dialogue line; its acknowledgment ends the event.
// C# only released the client there and left the event active, which blocked the next
// click until a map change.
func (s *Server) fallbackDialogue(c *Session, es *eventSession, step, portrait, speaker byte, talk uint32) (bool, error) {
	if (talk == 11087 || talk == 50062 || talk == 30025) && freeSlots(c.character) >= 1 {
		return true, nil // An inventory-full warning with space left.
	}
	es.onComplete = func() error { return s.finishEvent(c, es) }
	return true, s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 1}, dialogueFrame(step, portrait, speaker, talk)})
}

// execute is ExecuteOpcode for the supported actions. It returns false to end the event.
func (s *Server) execute(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	char := c.character
	br := es.ev.Branches[es.branch]
	switch op.Code {
	case world.ActionTransform:
		return s.convertBreillat(ctx, c, es, op)
	case world.ActionPlayer:
		switch {
		case op.D1 == world.PlayerActionAnimation:
			return true, c.send(eventFrame(14, byte(op.D2), op.D3, 0, 0, 0, op.Index, br.Index))
		case op.D1 == world.PlayerActionEffect:
			return true, c.send(eventFrame(9, 1, 0, 1, uint32(op.D2), op.D3, op.Index, br.Index))
		case op.D1 == world.PlayerActionRecordPoint:
			// A Record site saves the current position as the return point.
			next := char.Clone()
			next.RecordPoint = &game.Location{Map: char.Map, X: char.X, Y: char.Y}
			if e := s.commit(ctx, c, next); e != nil {
				return false, e
			}
			return true, c.send(recordPointStatus(next))
		case op.D1 == world.PlayerActionSceneTransition:
			// Only the deck's chapter transition starts the shipwreck rescue.
			// Ordinary deck/cabin doors use their authored warp IDs, including the
			// cabin exit whose ID is also 1. A map-wide override strands arcade visitors.
			if char.Map == game.MapID10017 && op.D2 == starterShipwreckTransition {
				c.beachPending = true
				return true, s.commandTeleport(ctx, c, beachDestination)
			}
			dst, ok := s.World.WarpEntry(char.Map, op.D2)
			if char.Map == carnie.Map {
				dst, ok = s.portalDestination(c, op.D2)
			}
			if ok {
				s.Log.Debug("scene transition", "session", c.info.ID, "map", char.Map, "warp", op.D2, "destination", dst.Map)
				return true, s.commandTeleport(ctx, c, dst)
			}
			return true, nil
		case op.D1 == world.PlayerActionReward && op.D2 == 2 && op.D3 == 0:
			balance := int64(char.Gold) + int64(int32(op.Value()))
			if balance < 0 || balance > game.MaxGold {
				return false, nil
			}
			next := char.Clone()
			next.Gold = uint32(balance)
			if e := s.commit(ctx, c, next); e != nil {
				return false, e
			}
			return true, c.send(protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
		case op.D1 == world.PlayerActionReward && op.D2 == 5 && op.D3 == 1:
			// C# assigns CurExp, whose setter adds EXP (unscaled) and levels up.
			exp := int32(op.Value())
			if exp < 0 {
				return false, nil
			}
			next := char.Clone()
			levels := next.AddExp(uint64(exp))
			if levels > 0 {
				next.Refill(s.Assets.Items)
			}
			if e := s.commit(ctx, c, next); e != nil {
				return false, e
			}
			packets := [][]byte{next.ExpPacket()}
			if levels > 0 {
				packets = append(packets, next.StatPackets(s.Assets.Items)...)
			}
			return true, s.sendAll(c, packets)
		case op.FullRecovery():
			next := char.Clone()
			full := next.Combat(s.Assets.Items)
			if op.D2 == 3 {
				next.MaxHP = uint32(max(full.MaxHP, 1))
				next.HP = next.MaxHP
			} else {
				next.MaxSP = uint32(max(full.MaxSP, 0))
				next.SP = next.MaxSP
			}
			if e := s.commit(ctx, c, next); e != nil {
				return false, e
			}
			return true, s.sendAll(c, next.StatPackets(s.Assets.Items))
		case op.D1 == world.PlayerActionReward && op.D2 != 1 && op.D2 != 7:
			return false, nil
		case op.D1 == world.PlayerActionReward && op.D3 > 0:
			return s.questItem(ctx, c, es, op)
		case op.D2 > 0:
			portrait := byte(3)
			if op.D1 > 0 {
				portrait = byte(op.D1)
			}
			speaker := byte(es.click)
			if portrait == 7 {
				speaker = 0
			}
			return s.fallbackDialogue(c, es, op.Index, portrait, speaker, uint32(op.D2)|uint32(op.D3)<<16)
		}
	case world.ActionActor:
		return s.actorAction(ctx, c, es, op)
	case world.ActionQuestMark:
		return s.questMark(ctx, c, op)
	case world.ActionCompanion:
		return s.companionAction(ctx, c, op)
	case world.ActionBattle, world.ActionBattleAlternate:
		return s.questBattle(c, es, op)
	case world.ActionServiceOrTeleport:
		if op.D1 == 0 {
			return false, nil
		}
		if op.D1 < 1000 {
			return true, s.openService(c, op.D1)
		}
		dst := world.Destination{Map: op.D1, X: op.D2, Y: op.D3}
		if dst.X == 0 {
			dst.X = 500
		}
		if dst.Y == 0 {
			dst.Y = 500
		}
		return true, s.commandTeleport(ctx, c, dst)
	case world.ActionMovie:
		if op.D4 == stormMovieOp {
			return true, s.playStorm(c)
		}
		if op.D1 == 1 || op.D1 == 2 {
			return true, c.send(eventFrame(5, 0, 0, 2, op.Value(), 0, op.Index, br.Index))
		}
		return true, c.send([]byte{protocol.CommandEvent, protocol.EventStepComplete})
	case world.ActionLearnSkill:
		if !s.hasSkill(op.D1) {
			return false, nil
		}
		next := c.character.Clone()
		packets := next.LearnSkill(op.D1)
		if len(packets) == 0 {
			return true, nil
		}
		if err := s.commit(ctx, c, next); err != nil {
			return false, err
		}
		return true, s.sendAll(c, packets)
	case world.ActionMinimapMarker:
		if p := world.Marker(c.view, byte(op.D1), op.D2, byte(op.D3-1), false); p != nil {
			return true, c.send(p)
		}
		return true, nil
	case world.ActionEffect:
		return true, c.send(eventFrame(15, byte(op.D2), 0, 0, op.Value(), 0, op.Index, br.Index))
	}
	return false, nil
}

// questItem gives (positive) or takes (negative) a quest item. A chest branch (kind 3)
// also opens its prop and records the chest as emptied for this character.
func (s *Server) questItem(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	char := c.character
	amount := int32(op.Value())
	if amount == 0 {
		return true, nil
	}
	if xaolanRobberyReward(es.mapID, es.ev.Branches[es.branch], op) {
		return s.claimXaolanRobbery(ctx, c, es, op)
	}
	chest := world.DecodeCond(es.ev.Branches[es.branch].Condition).Kind == 3 && amount > 0
	pool := s.chestPool(es.mapID, es.click)
	if amount > 0 && (!chest || pool == nil) && replacementHeld(char, es.ev, es.branch, op.D3) {
		return true, nil
	}
	if chest && pool != nil {
		if expiry, configured := char.ChestRespawns[world.ChestKey(es.mapID, es.ev.ClickID)]; configured && time.Now().Before(expiry) {
			return true, nil
		}
		if es.chestReward == nil {
			reward := rollChestReward(*pool)
			es.chestReward = &reward
		}
		op.D3 = es.chestReward.Item
		amount = int32(es.chestReward.Count)
	}
	next := char.Clone()
	result, e := next.Bag.ApplyQuestItems([]game.ItemChange{{ID: op.D3, Count: int(amount)}}, s.stackLimit, s.Assets.Items)
	if e != nil {
		if amount > 0 {
			return false, c.send(headBanner("Cannot receive reward. Check inventory space and item data."))
		}
		return false, nil
	}
	if chest {
		if pool != nil {
			if next.ChestRespawns == nil {
				next.ChestRespawns = map[uint32]time.Time{}
			}
			next.ChestRespawns[world.ChestKey(es.mapID, es.ev.ClickID)] = time.Now().UTC().Add(time.Duration(pool.RespawnSeconds) * time.Second)
		}
		next.Quests[world.ChestKey(es.mapID, es.ev.ClickID)] = game.Quest{ID: world.ChestKey(es.mapID, es.ev.ClickID), State: game.Completed, Step: 1, StartedAt: time.Now().UTC()}
	}
	if e = s.commit(ctx, c, next); e != nil {
		return false, e
	}
	var packets [][]byte
	for _, r := range result.Removed {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, r.Slot, r.Count})
	}
	if len(result.Added) > 0 {
		packets = append(packets, next.Bag.AdditionPacket(result.Added))
	}
	if amount < 0 {
		packets = append(packets, headBanner(fmt.Sprintf("Lost %s x%d", s.itemName(op.D3), -int64(amount))))
		return true, s.sendAll(c, packets)
	}
	packets = append(packets, headBanner(fmt.Sprintf("Obtain %s x%d", s.itemName(op.D3), amount)), []byte{protocol.CommandEvent, protocol.EventStepComplete})
	if chest {
		if c.view != nil && pool != nil {
			c.view.Props[es.ev.ClickID] = 1
		}
		packets = append(packets, protocol.Builder{protocol.CommandScene, protocol.SceneActorState}.U16(es.ev.ClickID).U8(1))
	}
	return true, s.sendAll(c, packets)
}

// questMark is opcode 5: advance a mark by an amount (mode 1) or complete it (mode 2).
func (s *Server) questMark(ctx context.Context, c *Session, op world.Op) (bool, error) {
	if op.D1 == 0 || (op.D2 != 1 && op.D2 != 2) {
		return false, nil
	}
	id := uint32(op.D1)
	prev, had := c.character.Quests[id]
	current := 0
	if had && prev.State == game.InProgress {
		current = prev.Step
	}
	step := current
	if op.D2 == 1 {
		step += int(int32(op.Value()))
	}
	if step < 0 || step > 255 {
		return false, nil
	}
	now := time.Now().UTC()
	q := game.Quest{ID: id, State: game.InProgress, Step: step, StartedAt: now}
	if had && !prev.StartedAt.IsZero() {
		q.StartedAt = prev.StartedAt
	}
	if op.D2 == 2 {
		q.State, q.CompletedAt = game.Completed, &now
	}
	next := c.character.Clone()
	next.Quests[id] = q
	if e := s.commit(ctx, c, next); e != nil {
		return false, e
	}
	packets := s.World.QuestUpdate(c.view, id, q)
	for _, reward := range storyStars {
		if id == reward.mark {
			packets = append(packets, storyConstellations(c.character))
			break
		}
	}
	return true, s.sendAll(c, packets)
}

// actorAction is opcode 2: animations, paths, prop frames, actor visibility and lines.
func (s *Server) actorAction(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	br := es.ev.Branches[es.branch]
	mapID := es.mapID
	if cliveAction(mapID, op) {
		switch op.D2 {
		case world.ActorActionAnimation, world.ActorActionAnimationAlternate:
			if op.D3 > 0xff {
				return false, nil
			}
			if op.D2 == world.ActorActionAnimation {
				return true, c.send(protocol.Builder{protocol.CommandScene, protocol.SceneWireCode8}.U16(op.D1).U8(byte(op.D3)).U8(byte(op.DW2)))
			}
			return true, c.send(protocol.Builder{protocol.CommandScene, protocol.SceneWireCode7}.U16(op.D1).U8(byte(op.D3)))
		case world.ActorActionWireCode10:
			return true, c.send(eventFrame(11, byte(op.D1), op.D2, byte(op.D3), op.DW2, 0, op.Index, br.Index))
		}
		star := byte(op.Value())
		return true, s.sendAll(c, [][]byte{{protocol.CommandPetControl, protocol.PetControlWireCode20, star, byte(op.D1)}, eventFrame(13, byte(op.D3), op.D1, byte(op.DW2), uint32(star), 0, op.Index, br.Index)})
	}
	switch op.D2 {
	case world.ActorActionPose:
		p := world.ActorPose(op.D1, op.D3, byte(op.Value()))
		if p == nil {
			return false, nil
		}
		return true, c.send(p)
	case world.ActorActionPath:
		return true, c.send(eventFrame(4, 3, op.D1, byte(op.D3), op.Value(), 0, op.Index, br.Index))
	case world.ActorActionPropState:
		target := es.click
		if op.D1 > 0 {
			target = op.D1
		}
		state := int32(1)
		if world.Mechanism(mapID) {
			state = int32(op.Value())
			if op.D3 != 1 || state < 0 || state > 1 {
				return false, nil
			}
		}
		if !questEvent(es.ev, es.branch) && c.tentOwner == 0 {
			if _, exists := s.World.NPC(mapID, target); exists {
				if err := s.Store.SetMapPropState(ctx, mapID, target, byte(state), time.Now(), scriptedPropResetInterval); err != nil {
					return false, err
				}
				s.publishProp(mapID, target, byte(state))
				return true, nil
			}
			// The source broadcasts an unknown target but has no actor to reset.
			s.broadcastWorld(c, propFrame(target, byte(state)))
		}
		c.view.Props[target] = state
		return true, c.send(propFrame(target, byte(state)))

	case world.ActorActionHide, world.ActorActionShow:
		target := es.click
		if op.D1 > 0 {
			target = op.D1
		}
		if world.Mechanism(mapID) {
			c.view.Actors[target] = op.D2 == world.ActorActionShow
		}
		if op.D2 == world.ActorActionShow {
			return true, c.send(s.World.ShowActor(c.view, mapID, target))
		}
		return true, c.send(s.World.HideActor(c.view, mapID, target))
	}
	if op.D2 == world.ActorActionSpeech && op.D3 == 0 {
		return false, nil
	}
	var talk uint32
	if op.D3 >= world.MinDialogueID && op.D3 <= world.MaxDialogueWordID {
		talk = uint32(op.D3) | uint32(op.D2)<<16
	} else if op.D2 >= world.MinDialogueID && op.D2 <= world.MaxDialogueWordID {
		talk = uint32(op.D2)
	}
	if talk < world.MinDialogueID {
		return true, nil // Not a dialogue line.
	}
	portrait, speaker := byte(3), byte(es.click)
	if op.D2 == world.ActorActionHide {
		portrait, speaker = 7, 0
	} else if op.D1 > 0 {
		speaker = byte(op.D1)
	}
	return s.fallbackDialogue(c, es, op.Index, portrait, speaker, talk)
}

// questEvent reports a per-character prop: the branch or any branch of its event names a mark.
func questEvent(ev *assets.Event, branch int) bool {
	if world.DecodeCond(ev.Branches[branch].Condition).W1 > 0 {
		return true
	}
	for _, br := range ev.Branches {
		if world.DecodeCond(br.Condition).W1 > 0 {
			return true
		}
	}
	return false
}

// runArea is ExecuteEntryEvents: the first linked event with an eligible branch runs
// with no speaker; otherwise the client is released.
func (s *Server) runArea(ctx context.Context, c *Session, area world.Area) error {
	mapID := c.character.Map
	linked, disabled := 0, 0
	for _, id := range area.Events {
		if _, ok := s.World.Event(mapID, uint16(id)); ok {
			linked++
			if s.World.Disabled(mapID, uint16(id)) {
				disabled++
			}
		}
	}
	if linked > 0 && disabled == linked {
		return s.rejectDisabled(c)
	}
	for _, id := range area.Events {
		ev, ok := s.World.Event(mapID, uint16(id))
		if !ok {
			continue
		}
		if i := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); i >= 0 {
			return s.startEvent(ctx, c, 0, ev, i, true)
		}
	}
	return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
}

// tryRegion is TryExecuteRegion: a walk-in region containing the player (and, after a
// move, not containing the previous position) runs its first eligible event.
func (s *Server) tryRegion(ctx context.Context, c *Session, prevX, prevY uint16, moved bool) (bool, error) {
	if c.event != nil {
		return true, nil
	}
	mapID := c.character.Map
	for _, area := range s.World.Regions(mapID) {
		if !area.Inside(c.character.X, c.character.Y, 0) || (moved && area.Inside(prevX, prevY, 0)) {
			continue
		}
		linked, disabled := 0, 0
		for _, id := range area.Events {
			if _, ok := s.World.Event(mapID, uint16(id)); ok {
				linked++
				if s.World.Disabled(mapID, uint16(id)) {
					disabled++
				}
			}
		}
		if linked > 0 && disabled == linked {
			return true, s.rejectDisabled(c)
		}
		for _, id := range area.Events {
			ev, ok := s.World.Event(mapID, uint16(id))
			if !ok {
				continue
			}
			if i := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); i >= 0 {
				return true, s.startEvent(ctx, c, 0, ev, i, true)
			}
		}
	}
	return false, nil
}

// tryEntry is TryExecuteEntry for the area ID an AC20:8 step reports. Doors accept a
// one-cell margin because the request can precede the last movement update.
func (s *Server) tryEntry(ctx context.Context, c *Session, id uint16) (bool, error) {
	area, ok := s.World.Entry(c.character.Map, id)
	if !ok {
		return false, nil
	}
	if c.event != nil {
		return true, nil
	}
	margin := int32(1)
	switch {
	case c.character.Map == game.MapID11075 && id == 2:
		margin = 2
	case area.Kind == 1:
		margin = 0
	}
	if !area.Inside(c.character.X, c.character.Y, margin) {
		return true, c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	return true, s.runArea(ctx, c, area)
}

// regionRequest is AC20:4 (ExecuteRegionRequest): the client names a region it entered.
func (s *Server) regionRequest(ctx context.Context, c *Session, data []byte) error {
	if len(data) != 2 {
		if c.event == nil {
			return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
		}
		return nil
	}
	if c.event != nil {
		return nil
	}
	id := uint16(data[0]) | uint16(data[1])<<8
	area, ok := s.World.Entry(c.character.Map, id)
	margin := int32(0)
	if c.character.Map == game.MapID11162 && id == 2 {
		margin = 2 // The well rope reports before its last movement update.
	}
	if !ok || area.Kind != 1 || !area.Inside(c.character.X, c.character.Y, margin) {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	return s.runArea(ctx, c, area)
}

// storyArrival is TryExecuteStoryArrival: resume interrupted spider-story state,
// then execute authored PreEvents on the other story maps.
func (s *Server) storyArrival(ctx context.Context, c *Session) (bool, error) {
	mapID := c.character.Map
	if c.event != nil {
		return true, nil
	}
	if fate, ok := c.character.Quests[13086]; mapID == game.MapID11077 && ok && fate.State == game.InProgress && fate.Step == 2 {
		farewell, found := s.World.Event(mapID, 12)
		if !found {
			return false, nil
		}
		// The death flag precedes completion in the authored ending. A restart
		// between those commits must finish cleanup without replaying the movie.
		if done, ok := c.character.Quests[13087]; ok && done.State == game.InProgress && done.Step > 0 {
			for _, br := range farewell.Branches {
				if br.Index != 1 {
					continue
				}
				for _, raw := range br.Operations {
					op := world.DecodeOp(raw)
					if op.Code == world.ActionQuestMark && op.D1 == 13086 && op.D2 == 2 {
						_, err := s.questMark(ctx, c, op)
						return false, err
					}
				}
			}
			return false, nil
		}
		i := s.World.FindBranch(c.character, c.view, mapID, farewell, world.TriggerEntry, 0, 0, -1)
		if i < 0 {
			return false, nil
		}
		return true, s.startEvent(ctx, c, 13, farewell, i, false)
	}
	if mapID != game.MapID12055 && mapID != game.MapID12000 && mapID != game.MapID11167 && mapID != game.MapID11039 {
		return false, nil
	}
	for _, pre := range s.World.PreEvents(mapID) {
		if mapID == game.MapID12000 {
			if pre.ClickID != 10 && pre.ClickID != 12 {
				continue
			}
		} else if pre.ClickID != 1 {
			continue
		}
		i := s.World.FindBranch(c.character, c.view, mapID, pre, world.TriggerEntry, 0, 0, -1)
		if i < 0 {
			continue
		}
		// The reunion cleanup is already durable after the first visit.
		if q, ok := c.character.Quests[13052]; mapID == game.MapID12000 && pre.ClickID == 10 && ok && q.State == game.Completed {
			continue
		}
		return true, s.startEvent(ctx, c, 0, pre, i, false)
	}
	return false, nil
}
