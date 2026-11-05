package server

import (
	"context"
	"fmt"
	"slices"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// eventSession is one running EVE branch (C# NativeEventActive with its token). The
// session's pointer is the token: callbacks of a replaced session do nothing.
// Reference: EveEventRuntime.StartSession and EveEventInterpreter.ExecuteOpcode.
type eventSession struct {
	transitions        int
	chestReward        *assets.ChestReward
	mapID, click       uint16
	ev                 *assets.Event
	branch             int
	index              int
	transition         bool
	cinematicPosture   bool
	onComplete         func() error
	onChoice           func(byte) error
	onMinigame         func(byte) error
	choiceAcknowledged bool // Mode-zero questions send AC20:6 after their answer.
	ignoreChoiceAck    bool
	shopCart           bool   // Native item-picker question owns a purchase.
	bankOwner          uint32 // Queued service belongs to this event and character.
}

const clickResume = 1500 * time.Millisecond

const maxAutomaticStoryTransitions = assets.MaxStoryAutomaticTransitions

// endEvent is Player.ClearInteraction.
func (s *Server) endEvent(c *Session) {
	if c.event != nil {
		c.event.bankOwner = 0
	}
	c.event = nil
	c.arcade = nil
}

// cancelInteraction is Player.CancelInteraction: release movement and the dialogue lock.
func (s *Server) cancelInteraction(c *Session) error {
	posture := c.event != nil && c.event.cinematicPosture
	s.endEvent(c)
	c.resumeAt = time.Now().Add(clickResume)
	if posture {
		s.sendOrClose(c, protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode30, 0}.U32(c.character.ID).U8(0), []byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
	}
	return s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 0}, {protocol.CommandEvent, protocol.EventResume}})
}

func (s *Server) finishEvent(c *Session, es *eventSession) error {
	if es == nil || c.event != es {
		return nil
	}
	if c.character == nil {
		s.endEvent(c)
		return nil
	}
	openBank := es.bankOwner != 0 && es.bankOwner == c.character.ID && es.mapID == c.character.Map
	es.bankOwner = 0
	if e := s.cancelInteraction(c); e != nil {
		return e
	}
	if err := s.sendAll(c, s.World.Sync(c.character, c.view, false)); err != nil {
		return err
	}
	if err := s.syncMapProps(context.Background(), c, time.Now()); err != nil {
		return err
	}
	if openBank {
		return s.openMoneyBank(c)
	}
	return nil
}

func (s *Server) rejectDisabled(c *Session) error {
	if e := s.cancelInteraction(c); e != nil {
		return e
	}
	return c.send(serverwire.HeadBanner("This quest is disabled: required game data is unavailable."))
}

// eventCommand handles AC20:1 clicks, AC20:2 contacts, acknowledgments and choices.
// The caller holds worldMu.
func (s *Server) eventCommand(ctx context.Context, c *Session, p []byte) error {
	switch p[1] {
	case protocol.EventStallOpen, protocol.EventStallClose, protocol.EventStallBrowse, protocol.EventStallBuy:
		return s.eventStallCommand(ctx, c, p)
	case protocol.EventActorClick, protocol.EventActorContact:
		return s.npcClick(ctx, c, p[2:])
	case protocol.EventAcknowledge:
		if c.resultAck {
			c.resultAck = false
			return nil // The minigame result's own acknowledgment.
		}
		if c.storm {
			if !c.stormAck {
				return nil
			}
			return s.endStorm(ctx, c)
		}
		if c.beach != nil {
			if c.beach.step >= 2 {
				return s.advanceBeach(ctx, c, false)
			}
			return nil // Acknowledgments before the wake-up animation are absorbed.
		}
		es := c.event
		if es != nil {
			if es.ignoreChoiceAck {
				es.ignoreChoiceAck = false
				return nil
			}
			if es.onChoice != nil || es.onMinigame != nil {
				return nil // The question stays open until AC20:9.
			}
			if next := es.onComplete; next != nil {
				es.onComplete = nil
				es.transitions = 0
				return next()
			}
			return s.finishEvent(c, es)
		}
		// finishEvent already released this interaction. Ignore its trailing
		// ACK at any latency: answering it with a resume creates an echo loop
		// and can hide a service window opened after the dialogue.
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
			es.transitions = 0
			err := choose(p[2])
			if err == nil && es.choiceAcknowledged && c.event != nil {
				c.event.ignoreChoiceAck = true
			}
			return err
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
	c.storageMap, c.hotelMap = 0, 0
	c.view.ShopItems = nil
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
	events := s.storyCandidates(c.character, click, s.World.NPCEvents(mapID, click))
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
	if _, ok := s.recruitmentPolicy(c, click); ok {
		return true, s.startRecruitment(ctx, c, click)
	}
	for _, ev := range events {
		i, err := s.findCollectionBranch(ctx, c, s.collectionNode(mapID, click, ev, nil), ev, time.Now())
		if err != nil {
			return true, err
		}
		if i >= 0 {
			return true, s.startEvent(ctx, c, click, ev, i, true)
		}
	}
	return true, c.send([]byte{protocol.CommandEvent, protocol.EventResume})
}

// startEvent is StartSession: refuse unsupported actions before any change, lock the
// player, then run actions until one waits for the client.
func (s *Server) startEvent(ctx context.Context, c *Session, click uint16, ev *assets.Event, branch int, transition bool, nodes ...store.ResourceKey) error {
	mapID := c.character.Map
	transitions := 1
	if c.event != nil {
		transitions = c.event.transitions + 1
	}
	if transitions > maxAutomaticStoryTransitions {
		s.Log.Error("automatic story transition limit", "map", mapID, "event", ev.ClickID)
		return s.finishEvent(c, c.event)
	}
	if s.World.Disabled(mapID, ev.ClickID) {
		return s.rejectDisabled(c)
	}
	if handled, err := s.tryWaterGathering(ctx, c, click, ev, branch, time.Now(), nodes...); handled {
		return err
	}
	ev = s.World.PrepareStory(c.character, c.view, mapID, ev, branch)
	s.Log.Debug("story event", "map", mapID, "event", ev.ClickID, "branch", ev.Branches[branch].Index, "actor", click)
	s.endEvent(c)
	es := &eventSession{transitions: transitions, mapID: mapID, click: click, ev: ev, branch: branch, transition: transition}
	c.event = es
	if e := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); e != nil {
		return e
	}
	for index, o := range ev.Branches[branch].Operations {
		if op := world.DecodeOp(o); (op.Unsupported() && !s.World.QuestCompanionAction(mapID, ev, branch, index)) || (op.Code == world.ActionTransform && !s.transformationAllowed(mapID, ev, branch, op)) || (op.Code == world.ActionLearnSkill && !s.hasSkill(op.D1)) {
			s.Log.Info("unsupported quest action", "map", mapID, "event", ev.ClickID, "branch", ev.Branches[branch].Index, "step", op.Index, "opcode", op.Code)
			if e := c.send(serverwire.HeadBanner("This quest action is not supported yet. Please report this quest.")); e != nil {
				return e
			}
			return s.finishEvent(c, es)
		}
	}
	return s.advance(ctx, c, es)
}

// eventPolicy resolves the SQL snapshot; presentation values are bounded at load.
func (s *Server) eventPolicy(kind string, c *Session, es *eventSession, op world.Op) (assets.StoryRule, bool) {
	raw := op.Raw()
	mapID := es.mapID
	if mapID == 0 {
		mapID = c.character.Map
	}
	r, ok := s.World.StoryFirst(kind, c.character, c.view, world.StoryContext{Map: mapID, Event: es.ev.ClickID, Actor: es.click, Branch: es.ev.Branches[es.branch].Index, Raw: &raw})
	if ok {
		s.Log.Debug("story policy", "rule", r.ID, "kind", kind, "map", mapID, "event", es.ev.ClickID, "branch", es.ev.Branches[es.branch].Index)
	}
	return r, ok
}
func (s *Server) actorCapability(c *Session, es *eventSession, op world.Op) bool {
	_, ok := s.eventPolicy(assets.StoryActorCapability, c, es, op)
	return ok
}

// canDeliver is CanDeliverPendingRewards: every reward before the next client step must
// fit before the first of them is given.
func (s *Server) canDeliver(c *Session, es *eventSession) bool {
	if plan, ok := s.World.QuestCompanion(es.mapID, es.ev, es.branch, es.index); ok {
		return s.canChangeQuestCompanion(c.character, plan)
	}
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
					if s.recruitBlocked(c.character, id) || len(party) >= game.MaxPets || !known {
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
					if es.chestReward.Gold > 0 {
						gold += int64(es.chestReward.Gold)
						if gold > game.MaxGold {
							return false
						}
						continue
					}
					op.D3, amount = es.chestReward.Item, int32(es.chestReward.Count)
				}
			}
			if op.D3 == 0 {
				return false
			}
			if amount != 0 {
				policy, checkpoint := s.storyReward(es.mapID, es.ev.Branches[es.branch], op)
				if checkpoint && c.character.Quests[policy.CompletedMark].State == game.Completed {
					continue
				}
				changes = append(changes, game.ItemChange{ID: op.D3, Count: int(amount)})
				if checkpoint {
					for _, item := range policy.ExtraItems {
						changes = append(changes, game.ItemChange{ID: item.ItemID, Count: item.Count})
					}
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
		if e := c.send(serverwire.HeadBanner("Cannot receive reward. Check materials, bag space, gold, party slots and Pet Hotel.")); e != nil {
			return e
		}
		return s.finishEvent(c, es)
	}
	br := es.ev.Branches[es.branch]
	// The native exchange starts with dialogue; its remaining item operations
	// must commit together after that dialogue is acknowledged.
	if changes, ok := s.World.StoryExchange(es.mapID, es.ev, es.branch, es.index); ok {
		if err := s.redeemStoryExchange(ctx, c, es, changes); err != nil {
			return err
		}
		return s.finishEvent(c, es)
	}
	resume := func() error { return s.advance(ctx, c, es) }
	for es.index < len(br.Operations) {
		if plan, ok := s.World.QuestCompanion(es.mapID, es.ev, es.branch, es.index); ok {
			if err := s.changeQuestCompanion(ctx, c, es, plan); err != nil {
				return err
			}
			break
		}
		if plan, ok := s.World.QuestCompletion(es.mapID, es.ev, es.branch, es.index); ok {
			if err := s.completeNativeQuest(ctx, c, es, plan); err != nil {
				return err
			}
			break
		}
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
					if handled, err := s.merchantChoice(c, es, uint16(choice)); handled {
						return err
					}
					return s.finishEvent(c, es)
				}
				return s.startEvent(ctx, c, es.click, es.ev, next, true)
			}
			// Mode 0 also closes with AC20:6; mode 1 reports only the choice.
			mode := byte(0)
			if r, ok := s.eventPolicy(assets.StoryChoiceMode, c, es, op); ok {
				mode = byte(r.Value)
			}
			s.authorizeShopQuestion(c, es, question)
			es.choiceAcknowledged = mode == 0
			return c.send(serverwire.EventFrame(6, subject, actor, mode, 0, question, op.Index, br.Index))
		}
		if op.NPCPath() {
			es.onComplete = resume
			return s.sendPathGroup(c, es, op)
		}
		speech := op.PlayerSpeech() || op.NPCSpeech()
		path := op.Code == world.ActionActor && (op.D2 == world.ActorActionPath || (s.actorCapability(c, es, op) && (op.D2 == world.ActorActionTurn || op.D2 == world.ActorActionConstellation)))
		movie := op.Code == world.ActionMovie && (op.D1 == 1 || op.D1 == 2)
		music := op.Code == world.ActionEffect
		effect := op.Code == world.ActionPlayer && (op.D1 == 6 || op.D1 == 7)
		if speech || path || movie || music || effect {
			es.onComplete = resume
			switch {
			case op.PlayerSpeech():
				return c.send(serverwire.SpeechFrame(7, 0, op.TalkID(), op.Index))
			case op.NPCSpeech():
				mode := byte(op.DW2)
				if mode == protocol.EventTalkCompatibilityDefault {
					mode = protocol.EventTalkTurnToPlayer
				}
				return c.send(serverwire.SpeechFrameWithMode(3, op.D1, op.TalkID(), op.Index, mode))
			}
			if movie {
				raw := op.Raw()
				for r := range s.World.StoryRules(assets.StoryMovieMode, c.character, c.view, world.StoryContext{Map: es.mapID, Event: es.ev.ClickID, Branch: br.Index, Raw: &raw}) {
					if r.LaterMovie != 0 && !laterMovie(br.Operations[es.index:], r.LaterMovie) {
						continue
					}
					s.Log.Debug("story movie policy", "rule", r.ID, "map", es.mapID, "event", es.ev.ClickID, "movie", op.Value(), "mode", r.Value)
					return c.send(serverwire.EventFrame(5, 0, 0, byte(r.Value), op.Value(), 0, op.Index, br.Index))
				}
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
		packets = append(packets, serverwire.EventFrame(4, 3, op.D1, byte(op.D3), op.Value(), 0, op.Index, branch))
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

func (s *Server) freeBagCells(c *game.Character) int {
	cells, err := c.Bag.Occupancy(s.Assets.Items)
	if err != nil {
		return 0
	}
	n := 0
	for _, anchor := range cells {
		if anchor == 0 {
			n++
		}
	}
	return n
}

func itemCapacityFailure(es *eventSession) bool {
	if es == nil || es.ev == nil || es.branch < 0 || es.branch >= len(es.ev.Branches) {
		return false
	}
	branches := es.ev.Branches
	end := min(len(branches), es.branch+1+world.DecodeCond(branches[es.branch].Condition).ExtraConditions())
	for _, branch := range branches[es.branch:end] {
		condition := world.DecodeCond(branch.Condition)
		if condition.Kind == world.ConditionFreeBagSlots && condition.W1 == world.BagCapacityCannotReceive {
			return true
		}
	}
	return false
}

// fallbackDialogue sends an untyped dialogue line; its acknowledgment ends the event.
// C# only released the client there and left the event active, which blocked the next
// click until a map change.
func (s *Server) fallbackDialogue(c *Session, es *eventSession, step, portrait, speaker byte, talk uint32) (bool, error) {
	if slices.Contains(s.Assets.Story.FullBagWarnings, talk) && !itemCapacityFailure(es) && s.freeBagCells(c.character) >= 1 {
		return true, nil // An inventory-full warning with space left.
	}
	es.onComplete = func() error { return s.finishEvent(c, es) }
	return true, s.sendAll(c, [][]byte{{protocol.CommandMovement, protocol.MovementMovementLock, 1}, serverwire.DialogueFrame(step, portrait, speaker, talk)})
}

// execute is ExecuteOpcode for the supported actions. It returns false to end the event.
func (s *Server) execute(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	char := c.character
	br := es.ev.Branches[es.branch]
	switch op.Code {
	case world.ActionMissionMarker:
		return true, nil
	case world.ActionMissionVariable:
		if op.D2 != world.MissionVariableAdd {
			return false, nil
		}
		c.view.MissionVariables[op.D1] += int32(op.Value())
		return true, nil
	case world.ActionTransform:
		return s.transformStoryCharacter(ctx, c, es, op)
	case world.ActionPlayer:
		switch {
		case op.D1 == world.PlayerActionAnimation:
			return true, c.send(serverwire.EventFrame(14, byte(op.D2), op.D3, 0, 0, 0, op.Index, br.Index))
		case op.D1 == world.PlayerActionEffect:
			frame := serverwire.EventFrame(9, 1, 0, 1, uint32(op.D2), op.D3, op.Index, br.Index)
			if op.D3 == world.PlayerEffectExpressionMode && op.D2 > 0 && op.D2 <= world.PlayerExpressionMaximumOperand {
				p := protocol.Builder{protocol.CommandPose, protocol.PoseEmote}.U32(c.character.ID).U8(byte(world.PlayerExpressionOffset + op.D2))
				s.broadcastWorld(c, p)
				return true, s.sendAll(c, [][]byte{p, frame})
			}
			return true, c.send(frame)
		case op.D1 == world.PlayerActionRecordPoint:
			// A Record site saves the current position as the return point.
			next := char.Clone()
			next.RecordPoint = &game.Location{Map: char.Map, X: char.X, Y: char.Y}
			if e := s.commit(ctx, c, next); e != nil {
				return false, e
			}
			return true, c.send(serverwire.RecordPointStatus(next))
		case op.D1 == world.PlayerActionSceneTransition:
			// Only the deck's chapter transition starts the shipwreck rescue.
			// Ordinary deck/cabin doors use their authored warp IDs, including the
			// cabin exit whose ID is also 1. A map-wide override strands arcade visitors.
			if r, ok := s.eventPolicy(assets.StoryTransition, c, es, op); ok {
				c.beachPending = true
				return true, s.commandTeleport(ctx, c, world.Destination{Map: r.Destination.Map, X: r.Destination.X, Y: r.Destination.Y})
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
		if op.D1 < world.ServiceTeleportMinimumMapID {
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
		for _, d := range s.Assets.Story.Cinematics {
			if d.Role == "movie_handshake" && slices.Contains(d.Maps, c.character.Map) && d.Trigger.Matches(op.Raw()) {
				if _, ok := s.cinematic(d.Role, c.character.Map); ok {
					return true, s.playStorm(c)
				}
			}
		}
		if op.D1 == 1 || op.D1 == 2 {
			mode := byte(protocol.EventMovieContinue)
			for _, raw := range br.Operations[es.index:] {
				if world.DecodeOp(raw).MapLoad() {
					mode = protocol.EventMovieHoldForMapLoad
					break
				}
			}
			return true, c.send(serverwire.EventFrame(5, 0, 0, mode, op.Value(), 0, op.Index, br.Index))
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
		return true, c.send(serverwire.EventFrame(15, byte(op.D2), 0, 0, op.Value(), 0, op.Index, br.Index))
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
	if policy, ok := s.storyReward(es.mapID, es.ev.Branches[es.branch], op); ok {
		return s.claimStoryReward(ctx, c, es, op, policy)
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
		if es.chestReward.Gold > 0 {
			return s.claimQuestChestGold(ctx, c, es, *pool, es.chestReward.Gold)
		}
		op.D3 = es.chestReward.Item
		amount = int32(es.chestReward.Count)
	}
	next := char.Clone()
	result, e := next.Bag.ApplyQuestItems([]game.ItemChange{{ID: op.D3, Count: int(amount)}}, s.stackLimit, s.Assets.Items)
	if e != nil {
		if amount > 0 {
			return false, c.send(serverwire.HeadBanner("Cannot receive reward. Check inventory space and item data."))
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
		packets = append(packets, serverwire.HeadBanner(fmt.Sprintf("Lost %s x%d", s.itemName(op.D3), -int64(amount))))
		return true, s.sendAll(c, packets)
	}
	packets = append(packets, serverwire.HeadBanner(fmt.Sprintf("Obtain %s x%d", s.itemName(op.D3), amount)), []byte{protocol.CommandEvent, protocol.EventStepComplete})
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
	return true, s.sendAll(c, s.questMarkPackets(c, id, q))
}

func (s *Server) questMarkPackets(c *Session, id uint32, q game.Quest) [][]byte {
	packets := s.World.QuestUpdate(c.view, id, q)
	for reward := range s.World.StoryRules(assets.StoryConstellation, nil, nil, world.StoryContext{}) {
		if id == reward.Mark {
			packets = append(packets, s.storyConstellations(c.character))
			break
		}
	}
	return packets
}

// actorAction is opcode 2: animations, paths, prop frames, actor visibility and lines.
func (s *Server) actorAction(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	br := es.ev.Branches[es.branch]
	mapID := es.mapID
	if s.actorCapability(c, es, op) {
		switch op.D2 {
		case world.ActorActionAnimation, world.ActorActionAnimationAlternate:
			if op.D3 > 0xff {
				return false, nil
			}
			if op.D2 == world.ActorActionAnimation {
				return true, c.send(protocol.Builder{protocol.CommandScene, protocol.SceneActorAnimation}.U16(op.D1).U8(byte(op.D3)).U8(byte(op.DW2)))
			}
			return true, c.send(protocol.Builder{protocol.CommandScene, protocol.SceneActorExpression}.U16(op.D1).U8(byte(op.D3)))
		case world.ActorActionTurn:
			return true, c.send(serverwire.EventFrame(11, byte(op.D1), op.D2, byte(op.D3), op.DW2, 0, op.Index, br.Index))
		}
		star := byte(op.Value())
		return true, s.sendAll(c, [][]byte{{protocol.CommandPetControl, protocol.PetControlWireCode20, star, byte(op.D1)}, serverwire.EventFrame(13, byte(op.D3), op.D1, byte(op.DW2), uint32(star), 0, op.Index, br.Index)})
	}
	switch op.D2 {
	case world.ActorActionPose:
		facing := byte(op.Value())
		if byte(op.DW2) != 0 {
			facing = byte(op.DW2)
		}
		packets := s.World.ActorPosePackets(c.view, mapID, op.D1, op.D3, facing)
		if len(packets) == 0 {
			return false, nil
		}
		return true, s.sendAll(c, packets)
	case world.ActorActionPath:
		return true, c.send(serverwire.EventFrame(4, 3, op.D1, byte(op.D3), op.Value(), 0, op.Index, br.Index))
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
			s.broadcastWorld(c, serverwire.PropFrame(target, byte(state)))
		}
		c.view.Props[target] = state
		return true, c.send(serverwire.PropFrame(target, byte(state)))

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
		node := store.ResourceKey{Kind: store.ResourceCollectionArea, MapID: mapID, NodeID: area.ClickID}
		i, err := s.findCollectionBranch(ctx, c, node, ev, time.Now())
		if err != nil {
			return err
		}
		if i >= 0 {
			return s.startEvent(ctx, c, 0, ev, i, true, node)
		}
	}
	return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
}

// tryRegion is TryExecuteRegion: a walk-in region containing the player (and, after a
// move, not containing the previous position) runs its first eligible event.
func (s *Server) tryRegion(ctx context.Context, c *Session, prevX, prevY uint16, moved bool) (bool, error) {
	if s.starterBeachQuestRaft(c) {
		return false, nil
	}
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
			node := store.ResourceKey{Kind: store.ResourceCollectionArea, MapID: mapID, NodeID: area.ClickID}
			i, err := s.findCollectionBranch(ctx, c, node, ev, time.Now())
			if err != nil {
				return true, err
			}
			if i >= 0 {
				return true, s.startEvent(ctx, c, 0, ev, i, true, node)
			}
		}
	}
	return false, nil
}

// tryEntry is TryExecuteEntry for the area ID an AC20:8 step reports. Doors accept a
// two-cell margin because the request can precede the last movement update.
const nativeEntryMarginCells = 2

func (s *Server) tryEntry(ctx context.Context, c *Session, id uint16) (bool, error) {
	area, ok := s.World.Entry(c.character.Map, id)
	if !ok {
		return false, nil
	}
	if c.event != nil {
		return true, nil
	}
	margin := int32(nativeEntryMarginCells)
	if !area.Inside(c.character.X, c.character.Y, margin) {
		return true, c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	return true, s.runArea(ctx, c, area)
}

// regionRequest is AC20:4 (ExecuteRegionRequest): the client names a region it entered.
func (s *Server) regionRequest(ctx context.Context, c *Session, data []byte) error {
	if s.starterBeachQuestRaft(c) {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
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
	if r, ok := s.World.StoryFirst(assets.StoryRegionMargin, c.character, c.view, world.StoryContext{Map: c.character.Map, Actor: id}); ok {
		margin = int32(r.Value)
	}
	if !ok || area.Kind != 1 || !area.Inside(c.character.X, c.character.Y, margin) {
		return c.send([]byte{protocol.CommandEvent, protocol.EventResume})
	}
	return s.runArea(ctx, c, area)
}

// storyArrival executes configured cleanup and authored PreEvent recovery routes.
func (s *Server) storyArrival(ctx context.Context, c *Session) (bool, error) {
	if c.event != nil {
		return true, nil
	}
	x := world.StoryContext{Map: c.character.Map}
	for r := range s.World.StoryRules(assets.StoryArrivalCleanup, c.character, c.view, x) {
		if ev, ok := s.World.Event(x.Map, r.TargetEvent); ok {
			for _, br := range ev.Branches {
				if br.Index == r.TargetBranch {
					for _, raw := range br.Operations {
						if r.TargetPattern.Matches(raw.Data) {
							_, err := s.questMark(ctx, c, world.DecodeOp(raw))
							return false, err
						}
					}
				}
			}
		}
		return false, nil
	}
	for r := range s.World.StoryRules(assets.StoryArrival, c.character, c.view, x) {
		var ev *assets.Event
		if r.UsePreEvent {
			for _, pre := range s.World.PreEvents(x.Map) {
				if pre.ClickID == r.TargetEvent {
					ev = pre
					break
				}
			}
		} else {
			ev, _ = s.World.Event(x.Map, r.TargetEvent)
		}
		if ev == nil {
			continue
		}
		if i := s.World.FindBranch(c.character, c.view, x.Map, ev, world.TriggerEntry, 0, 0, -1); i >= 0 {
			return true, s.startEvent(ctx, c, r.TargetActor, ev, i, false)
		}
	}
	return false, nil
}
