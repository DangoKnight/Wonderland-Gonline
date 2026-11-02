package server

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// Presentation watchdogs are engine policy, independent of story content.
var beachTimeout = 25 * time.Second

type beachRun struct {
	step       int
	definition assets.StoryCinematic
}

func (s *Server) cinematic(role string, mapID uint16) (assets.StoryCinematic, bool) {
	for _, d := range s.Assets.Story.Cinematics {
		if d.Role == role && slices.Contains(d.Maps, mapID) {
			return d, true
		}
	}
	return assets.StoryCinematic{}, false
}

func (s *Server) cinematicCues(c *Session, cues []assets.StoryCue) error {
	for _, cue := range cues {
		p, err := serverwire.StoryCuePacket(*c.character, cue)
		if err != nil {
			return err
		}
		if cue.Kind == "pose" {
			c.emote = byte(cue.Data[0])
		}
		if cue.Broadcast {
			s.broadcastWorld(c, p)
		}
		if cue.Self {
			if err := c.send(p); err != nil {
				return err
			}
		}
	}
	return nil
}

// Movie handshakes and ACK ownership are protocol semantics. The selected
// definition owns presentation and the durable destination.
func (s *Server) playStorm(c *Session) error {
	d, ok := s.cinematic("movie_handshake", c.character.Map)
	if !ok {
		return fmt.Errorf("movie handshake definition unavailable")
	}
	c.stormDefinition = &d
	c.storm = true
	c.stormAck = false
	c.stormStarted = time.Now()
	return s.cinematicCues(c, d.Steps[0].Cues)
}
func (s *Server) endStorm(ctx context.Context, c *Session) error {
	d := c.stormDefinition
	if d == nil {
		return fmt.Errorf("movie handshake definition unavailable")
	}
	c.storm = false
	c.stormAck = false
	c.stormDefinition = nil
	s.endEvent(c)
	c.beachPending = true
	if err := s.cinematicCues(c, d.Finish); err != nil {
		return err
	}
	return s.commandTeleport(ctx, c, world.Destination{Map: d.DestinationMap, X: d.DestinationX, Y: d.DestinationY})
}

func (s *Server) arrivalScript(c *Session) error {
	if c.beach != nil {
		return nil
	}
	d, configured := s.cinematic("arrival", c.character.Map)
	_, completed := c.character.Quests[d.CompletionMark]
	if !configured || (!c.beachPending && completed) {
		ctx := context.Background()
		if ran, err := s.storyArrival(ctx, c); ran || err != nil {
			return err
		}
		if _, ok := s.World.StoryFirst(assets.StoryArrivalRegion, c.character, c.view, world.StoryContext{Map: c.character.Map}); ok {
			_, err := s.tryRegion(ctx, c, 0, 0, false)
			return err
		}
		return nil
	}
	c.beachPending = false
	s.endEvent(c)
	run := &beachRun{step: 1, definition: d}
	c.beach = run
	s.beachIntro(c, run)
	return nil
}
func (s *Server) beachIntro(c *Session, run *beachRun) {
	if c.beach != run || run.step != 1 || !slices.Contains(run.definition.Maps, c.character.Map) {
		return
	}
	run.step = 2
	if s.cinematicCues(c, run.definition.Steps[0].Cues) != nil {
		return
	}
	time.AfterFunc(beachTimeout, func() {
		s.worldMu.Lock()
		defer s.worldMu.Unlock()
		if c.beach == run && slices.Contains(run.definition.Maps, c.character.Map) {
			s.advanceBeach(context.Background(), c, true)
		}
	})
}
func (s *Server) advanceBeach(ctx context.Context, c *Session, force bool) error {
	run := c.beach
	if run == nil || run.step < 2 {
		return nil
	}
	if force {
		run.step = len(run.definition.Steps) + 1
	}
	at := run.step - 1
	if at < len(run.definition.Steps) {
		run.step++
		return s.cinematicCues(c, run.definition.Steps[at].Cues)
	}
	c.beach = nil
	return s.cinematicCues(c, run.definition.Finish)
}
func (s *Server) acknowledgeStorm(c *Session) {
	if !c.storm || c.stormAck {
		return
	}
	c.stormAck = true
	started := c.stormStarted
	time.AfterFunc(beachTimeout, func() {
		s.worldMu.Lock()
		defer s.worldMu.Unlock()
		if c.storm && c.stormAck && c.stormStarted.Equal(started) && s.world[c.info.ID] == c {
			if err := s.endStorm(context.Background(), c); err != nil {
				s.Log.Error("cinematic recovery failed", "error", err)
			}
		}
	})
}

// storyClick resumes SQL-authored checkpoints through their event routes.
func (s *Server) storyClick(ctx context.Context, c *Session, click uint16) (bool, error) {
	x := world.StoryContext{Map: c.character.Map, Actor: click}
	for r := range s.World.StoryRules(assets.StoryClick, c.character, c.view, x) {
		s.Log.Debug("story rule", "id", r.ID, "map", x.Map, "actor", click)
		if ev, ok := s.World.Event(x.Map, r.TargetEvent); ok {
			if branch := s.World.FindBranch(c.character, c.view, x.Map, ev, world.TriggerEntry, 0, 0, -1); branch >= 0 {
				return true, s.startEvent(ctx, c, click, ev, branch, true)
			}
		}
		if r.Text != "" {
			return true, s.sendAll(c, [][]byte{serverwire.HeadBanner(r.Text), {protocol.CommandEvent, protocol.EventResume}})
		}
	}
	for r := range s.World.StoryRules(assets.StoryClickMark, c.character, c.view, x) {
		changed := false
		next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
			if stored.Map != x.Map || !s.World.StoryGates(r.Gates, stored, c.view, x) {
				return nil
			}
			q := stored.Quests[r.Mark]
			q.ID = r.Mark
			q.Step = int(r.Value)
			stored.Quests[r.Mark] = q
			changed = true
			return nil
		}, *c.character)
		if err != nil {
			return false, err
		}
		s.adoptSavedCharacter(c, next)
		if changed {
			if err := s.sendAll(c, s.World.QuestUpdate(c.view, r.Mark, next.Quests[r.Mark])); err != nil {
				return false, err
			}
		}

	}
	return false, nil
}

// storyContinuation interprets typed links, preserving native branch order and
// callback ownership. Automatic chains are bounded by startEvent.
func (s *Server) storyContinuation(ctx context.Context, c *Session, es *eventSession) (bool, error) {
	x := world.StoryContext{Map: es.mapID, Event: es.ev.ClickID, Actor: es.click, Branch: es.ev.Branches[es.branch].Index}
	for r := range s.World.StoryRules(assets.StoryContinue, c.character, c.view, x) {
		ev, exclude, click := es.ev, es.branch, es.click
		if r.TargetEvent != 0 && r.TargetEvent != ev.ClickID {
			var ok bool
			ev, ok = s.World.Event(es.mapID, r.TargetEvent)
			if !ok {
				continue
			}
			exclude = -1
		}
		if r.TargetActor != 0 {
			click = r.TargetActor
		}
		// A required destination branch is tested using ordinary eligibility first.
		if r.TargetBranch != 0 {
			exclude = -1
		}
		next := s.World.FindBranch(c.character, c.view, es.mapID, ev, world.TriggerEntry, 0, 0, exclude)
		if next < 0 || (r.TargetBranch != 0 && ev.Branches[next].Index != r.TargetBranch) {
			continue
		}
		s.Log.Debug("story continuation", "rule", r.ID, "event", ev.ClickID, "branch", ev.Branches[next].Index)
		return true, s.startEvent(ctx, c, click, ev, next, false)
	}
	return false, nil
}

// storyCandidates prepends configured recovery routes without disturbing an
// existing authored event order or bypassing eligibility/disabled-event checks.
func (s *Server) storyCandidates(c *game.Character, click uint16, events []*assets.Event) []*assets.Event {
	for r := range s.World.StoryRules(assets.StoryCandidate, c, nil, world.StoryContext{Map: c.Map, Actor: click}) {
		ev, ok := s.World.Event(c.Map, r.TargetEvent)
		if !ok {
			continue
		}
		found := false
		for _, candidate := range events {
			found = found || candidate == ev
		}
		if !found {
			events = append([]*assets.Event{ev}, events...)
		}
	}
	return events
}

const (
	nativeMoviePlay          = 1
	nativeMoviePlayAlternate = 2
	nativeMovieFrameKind     = 5
	nativePlayerPortrait     = 7
)

func (s *Server) recruitmentPolicy(c *Session, click uint16) (assets.StoryRecruitment, bool) {
	x := world.StoryContext{Map: c.character.Map, Actor: click}
	for _, p := range s.Assets.Story.Recruitments {
		if world.StoryScopeMatches(p.Scope, x) {
			return p, true
		}
		if n, ok := s.World.NPC(x.Map, click); ok && n.Template == p.Template {
			return p, true
		}
	}
	return assets.StoryRecruitment{}, false
}

type recruitMovieStep struct {
	op     world.Op
	branch byte
}

// recruitmentMovies preserves the movie operations from the authored rescue branch.
// Recruitment is still committed atomically by finishRecruitment, rather than by the
// authored branch's separate companion and quest-mark writes.
func (s *Server) recruitmentMovies(mapID, click uint16, policy assets.StoryRecruitment) []recruitMovieStep {
	if !world.StoryScopeMatches(policy.Scope, world.StoryContext{Map: mapID, Actor: click}) {
		return nil
	}
	for _, ev := range s.World.NPCEvents(mapID, click) {
		for _, br := range ev.Branches {
			var movies []recruitMovieStep
			for _, raw := range br.Operations {
				op := world.DecodeOp(raw)
				if op.Code == world.ActionMovie && (op.D1 == nativeMoviePlay || op.D1 == nativeMoviePlayAlternate) {
					movies = append(movies, recruitMovieStep{op: op, branch: br.Index})
				}
			}
			if len(movies) != 0 {
				return movies
			}
		}
	}
	return nil
}

// startRecruitment plays the authored rescue movies when available, retaining the
// sixteen-step dialogue for legacy definitions without movie operations. Only a linked,
// enabled native event may invoke it; range/visibility checks still precede it.
func (s *Server) startRecruitment(ctx context.Context, c *Session, click uint16) error {
	policy, configured := s.recruitmentPolicy(c, click)
	if !configured {
		return nil
	}
	_, owned := c.character.Pet(policy.Template)
	done := c.character.Quests[policy.CompletedMark].State == game.Completed
	lines := append([]assets.StoryLine(nil), policy.RepeatLines...)
	recruit := !owned && !done
	var movies []recruitMovieStep
	if recruit {
		if _, known := s.World.Template(policy.Template); !known {
			return s.sendAll(c, [][]byte{serverwire.HeadBanner("The companion definition is unavailable."), {protocol.CommandEvent, protocol.EventResume}})
		}
		count := len(policy.Lines)
		if len(c.character.Pets) >= game.MaxPets {
			count = policy.FullPartyPrefix
		}
		lines = append([]assets.StoryLine(nil), policy.Lines[:count]...)
		movies = s.recruitmentMovies(c.character.Map, click, policy)
		if len(movies) != 0 {
			// The cinematic already contains the rescue dialogue.
			lines = nil
		}
		if count < len(policy.Lines) {
			if len(movies) > 1 {
				movies = movies[:1]
			}
			lines = append(lines, policy.FullPartyLines...)
			recruit = false
		}
	}
	s.endEvent(c)
	ev := assets.Event{Branches: []assets.Branch{{Index: 1}}}
	es := &eventSession{mapID: c.character.Map, click: click, ev: &ev}
	c.event = es
	if err := c.send([]byte{protocol.CommandMovement, protocol.MovementMovementLock, 1}); err != nil {
		return err
	}
	index, movieIndex := 0, 0
	var advance func() error
	advance = func() error {
		if c.event != es || c.character.Map != es.mapID {
			return nil
		}
		if movieIndex < len(movies) {
			movie := movies[movieIndex]
			op := movie.op
			movieIndex++
			es.onComplete = advance
			packets := [][]byte{serverwire.EventFrame(nativeMovieFrameKind, 0, 0, policy.MovieMode, op.Value(), 0, op.Index, movie.branch)}
			if movieIndex == 1 && op.Value() == policy.PostureMovie {
				es.cinematicPosture = true
				packets = append(packets, protocol.Builder{protocol.CommandMovie, protocol.MovieWireCode12}.U16(uint16(policy.PostureMovie)), protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateWireCode30, 0}.U32(c.character.ID).U8(1))
			}
			return s.sendAll(c, packets)
		}
		if index == len(lines) {
			if recruit {
				return s.finishRecruitment(ctx, c, es, policy)
			}
			return s.finishEvent(c, es)
		}
		line := lines[index]
		index++
		speaker := byte(click)
		if line.Portrait == nativePlayerPortrait {
			speaker = 0
		}
		es.onComplete = advance
		return c.send(serverwire.DialogueFrame(byte(index), line.Portrait, speaker, line.Talk))
	}
	return advance()
}
func (s *Server) finishRecruitment(ctx context.Context, c *Session, es *eventSession, policy assets.StoryRecruitment) error {
	template := s.petTemplate(policy.Template)
	var pet game.Pet
	granted := false
	errUnavailable := errors.New("recruitment unavailable")
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		if stored.Map != es.mapID {
			return fmt.Errorf("recruitment map changed")
		}
		if _, owned := stored.Pet(policy.Template); owned || stored.Quests[policy.CompletedMark].State == game.Completed {
			return nil
		}
		if stored.FreePetSlot() == 0 || slices.ContainsFunc(stored.HotelPets, func(p game.Pet) bool { return game.SamePet(p.ID, policy.Template) }) {
			return errUnavailable
		}
		pet = game.NewPet(policy.Template, policy.Name, stored.FreePetSlot(), template, s.Assets.Items)
		if i := slices.IndexFunc(stored.ReservePets, func(p game.Pet) bool { return game.SamePet(p.ID, policy.Template) }); i >= 0 {
			pet = stored.ReservePets[i]
			pet.Slot = stored.FreePetSlot()
			stored.ReservePets = slices.Delete(stored.ReservePets, i, i+1)
		}
		pet.EnsureSkills(template, s.hasSkill)
		pet.Normalize(s.Assets.Items, true)
		stored.Pets = append(stored.Pets, pet)
		if err := applyStoryMarks(stored, policy.Marks, time.Now().UTC()); err != nil {
			return err
		}
		granted = true
		return nil
	}, *c.character)
	if errors.Is(err, errUnavailable) {
		if sendErr := c.send(serverwire.HeadBanner(policy.UnavailableText)); sendErr != nil {
			return sendErr
		}
		return s.finishEvent(c, es)
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	if !granted {
		return s.finishEvent(c, es)
	}
	c.view.Hidden[es.click] = true
	packets := [][]byte{protocol.Builder{protocol.CommandScene, protocol.SceneActorHide}.U16(es.click).U16(0xffff)}
	if c.pets.register(pet.ID) {
		packets = append(packets, pet.RecruitPacket(next.ID, template))
	}
	slot := c.pets.slot(pet.ID)
	packets = append(packets, pet.ProgressionPackets(slot, s.Assets.Items)...)
	packets = append(packets, serverwire.PetNamePacket(next.ID, slot, s.companionName(pet.ID, pet.Name)))
	if list := s.petListPacket(c.character, c.pets); list != nil {
		packets = append(packets, list)
	}
	for _, mark := range policy.Marks {
		packets = append(packets, s.World.QuestUpdate(c.view, mark.ID, next.Quests[mark.ID])...)
	}
	packets = append(packets, serverwire.SystemLine(policy.JoinedText), []byte{protocol.CommandEvent, protocol.EventStepComplete})
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.finishEvent(c, es)
}

const maxStoryMarkStep = 255

func applyStoryMarks(c *game.Character, marks []assets.StoryMark, now time.Time) error {
	if c.Quests == nil {
		c.Quests = map[uint32]game.Quest{}
	}
	for _, m := range marks {
		q := c.Quests[m.ID]
		if m.Mode == "ensure_active" && (q.State == game.Completed || q.State == game.InProgress && q.Step >= m.Value) {
			continue
		}
		if q.State != game.InProgress && !(m.Mode == "complete" && m.PreserveStep) {
			q.Step = 0
		}
		q.ID = m.ID
		q.Kills = 0
		if q.StartedAt.IsZero() {
			q.StartedAt = now
		}
		switch m.Mode {
		case "complete":
			q.State = game.Completed
			if m.PreserveStep {
				q.Step = max(q.Step, m.Value)
			} else {
				q.Step = m.Value
			}
			if !m.PreserveStep || q.CompletedAt == nil {
				q.CompletedAt = &now
			}
		case "advance":
			q.State = game.InProgress
			q.Step += m.Value
			q.CompletedAt = nil
		case "set", "ensure_active":
			q.State = game.InProgress
			q.Step = m.Value
			q.CompletedAt = nil
		default:
			return fmt.Errorf("unknown story mark mode %q", m.Mode)
		}
		if q.Step < 0 || q.Step > maxStoryMarkStep {
			return fmt.Errorf("story checkpoint exceeds native mark range")
		}
		c.Quests[m.ID] = q
	}
	return nil
}

// Rebirth checkpoints are authored rules, applied to an owned SQL snapshot.
// The same capability repairs older characters before their login snapshot.
func (s *Server) applyRebornCheckpoints(c *game.Character, now time.Time) ([]uint32, error) {
	if !c.Reborn {
		return nil, nil
	}
	var changed []uint32
	for rule := range s.World.StoryRules(assets.StoryRebornCheckpoint, c, nil, world.StoryContext{Map: c.Map}) {
		if err := applyStoryMarks(c, rule.Marks, now); err != nil {
			return nil, err
		}
		for _, mark := range rule.Marks {
			changed = append(changed, mark.ID)
		}
	}
	return changed, nil
}
func (s *Server) reconcileRebornCheckpoints(ctx context.Context, account uint32, snapshot game.Character) (game.Character, error) {
	if !snapshot.Reborn {
		return snapshot, nil
	}
	if _, ok := s.World.StoryFirst(assets.StoryRebornCheckpoint, &snapshot, nil, world.StoryContext{Map: snapshot.Map}); !ok {
		return snapshot, nil
	}
	return s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: account, ID: snapshot.ID}, func(current *game.Character) error {
		_, err := s.applyRebornCheckpoints(current, time.Now().UTC())
		return err
	})
}

func (s *Server) storyReward(mapID uint16, branch assets.Branch, op world.Op) (assets.StoryReward, bool) {
	if int32(op.Value()) <= 0 {
		return assets.StoryReward{}, false
	}
	for _, p := range s.Assets.Story.Rewards {
		if world.StoryScopeMatches(p.Scope, world.StoryContext{Map: mapID, Branch: branch.Index}) && p.Condition.Matches(branch.Condition) && p.Pattern.Matches(op.Raw()) {
			for _, raw := range branch.Operations {
				if world.DecodeOp(raw).Code == world.ActionQuestMark && world.DecodeOp(raw).D1 == uint16(p.CompletedMark) && world.DecodeOp(raw).D2 == 2 {
					return p, true
				}
			}
		}
	}
	return assets.StoryReward{}, false
}

// claimStoryReward rechecks ownership and completion against SQL, then delivers
// the reward and its recovery marks in a single transaction before receipts.
func (s *Server) claimStoryReward(ctx context.Context, c *Session, es *eventSession, op world.Op, p assets.StoryReward) (bool, error) {
	ops := es.ev.Branches[es.branch].Operations
	if es.index+len(p.Tail) != len(ops) {
		return false, c.send(serverwire.HeadBanner("This reward has an unsupported quest tail."))
	}
	for i, tail := range p.Tail {
		got, want := world.DecodeOp(ops[es.index+i]), world.DecodeOp(tail.Operation())
		if got.Code != want.Code || got.D1 != want.D1 || got.D2 != want.D2 || got.D3 != want.D3 || got.Value() != want.Value() {
			return false, c.send(serverwire.HeadBanner("This reward has an unsupported quest tail."))
		}
	}
	if c.character.Quests[p.CompletedMark].State != game.Completed {
		bag := c.character.Bag
		changes := []game.ItemChange{{ID: op.D3, Count: int(int32(op.Value()))}}
		for _, item := range p.ExtraItems {
			changes = append(changes, game.ItemChange{ID: item.ItemID, Count: item.Count})
		}
		if _, err := bag.ApplyQuestItems(changes, s.stackLimit, s.Assets.Items); err != nil {
			return false, c.send(serverwire.HeadBanner(p.FullText))
		}
	}
	var result game.QuestItemResult
	granted := false
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		if stored.Map != es.mapID {
			return fmt.Errorf("story reward map changed")
		}
		if stored.Quests[p.CompletedMark].State == game.Completed {
			return nil
		}
		changes := []game.ItemChange{{ID: op.D3, Count: int(int32(op.Value()))}}
		for _, item := range p.ExtraItems {
			changes = append(changes, game.ItemChange{ID: item.ItemID, Count: item.Count})
		}
		var err error
		result, err = stored.Bag.ApplyQuestItems(changes, s.stackLimit, s.Assets.Items)
		if err != nil {
			return err
		}
		if err = applyStoryMarks(stored, p.Marks, time.Now().UTC()); err != nil {
			return err
		}
		granted = true
		return nil
	}, *c.character)
	if err != nil {
		if errors.Is(err, game.ErrInventoryFull) {
			return false, c.send(serverwire.HeadBanner(p.FullText))
		}
		return false, err
	}
	s.adoptSavedCharacter(c, next)
	es.index += len(p.Tail)
	if !granted {
		return true, nil
	}
	packets := [][]byte{next.Bag.AdditionPacket(result.Added), serverwire.HeadBanner(strings.ReplaceAll(p.Message, "{quantity}", strconv.FormatUint(uint64(op.Value()), 10))), {protocol.CommandEvent, protocol.EventStepComplete}}
	for _, mark := range p.Marks {
		packets = append(packets, s.World.QuestUpdate(c.view, mark.ID, next.Quests[mark.ID])...)
	}
	return true, s.sendAll(c, packets)
}

func (s *Server) hasStoryReward(mapID uint16, branch assets.Branch, op world.Op) bool {
	_, ok := s.storyReward(mapID, branch, op)
	return ok
}

var errStoryTransformUnavailable = errors.New("story transformation unavailable")

var errStoryExchangeUnavailable = errors.New("story exchange unavailable")

func (s *Server) redeemStoryExchange(ctx context.Context, c *Session, es *eventSession, changes []game.ItemChange) error {
	var result game.QuestItemResult
	next, err := s.Store.MutateOwnedCharacter(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, func(stored *game.Character) error {
		if stored.Map != es.mapID || s.World.FindBranch(stored, c.view, es.mapID, es.ev, world.TriggerEntry, 0, 0, -1) != es.branch {
			return errStoryExchangeUnavailable
		}
		var err error
		result, err = stored.Bag.ApplyQuestItems(changes, s.stackLimit, s.Assets.Items)
		if err != nil {
			return fmt.Errorf("%w: %v", errStoryExchangeUnavailable, err)
		}
		return nil
	}, *c.character)
	if err != nil {
		if !errors.Is(err, errStoryExchangeUnavailable) {
			return errors.Join(err, s.cancelInteraction(c))
		}
		r, _ := s.World.StoryFirst(assets.StoryExchange, c.character, c.view, world.StoryContext{Map: es.mapID, Event: es.ev.ClickID, Branch: es.ev.Branches[es.branch].Index})
		return c.send(serverwire.HeadBanner(r.Text))
	}
	s.adoptSavedCharacter(c, next)
	es.index = len(es.ev.Branches[es.branch].Operations)
	var packets [][]byte
	for _, removed := range result.Removed {
		packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, removed.Slot, removed.Count})
	}
	packets = append(packets, next.Bag.AdditionPacket(result.Added))
	for _, change := range changes {
		verb, count := "Obtain", change.Count
		if count < 0 {
			verb, count = "Lost", -count
		}
		packets = append(packets, serverwire.HeadBanner(fmt.Sprintf("%s %s x%d", verb, s.itemName(change.ID), count)))
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return s.sendAll(c, packets)
}

// The acceptance branch's marks and model are committed together. Read fresh
// player state inside the transaction, including the outfit replacement.
func (s *Server) transformStoryCharacter(ctx context.Context, c *Session, es *eventSession, op world.Op) (bool, error) {
	policy, allowed := s.World.Transformation(es.mapID, es.ev, es.branch, op)
	if !allowed {
		return false, nil
	}
	var next game.Character
	var beforeBag game.Inventory
	now := time.Now().UTC()
	err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, stored); err != nil {
			return err
		}
		if stored.Map != es.mapID || !s.World.StoryGates(policy.Gates, stored, c.view, world.StoryContext{Map: es.mapID, Event: es.ev.ClickID, Actor: es.click, Branch: es.ev.Branches[es.branch].Index}) {
			return errStoryTransformUnavailable
		}
		beforeBag = stored.Bag
		if err := stored.ReplaceStarterOutfit(s.Assets.Items, policy.Body, policy.Head); err != nil {
			return err
		}
		stored.Body, stored.Head = policy.Body, policy.Head
		if err := applyStoryMarks(stored, policy.Marks, now); err != nil {
			return err
		}
		full := stored.Combat(s.Assets.Items)
		stored.MaxHP, stored.MaxSP = uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
		stored.HP, stored.SP = min(stored.HP, stored.MaxHP), min(stored.SP, stored.MaxSP)
		next = stored.Clone()
		return nil
	})
	if errors.Is(err, errStoryTransformUnavailable) {
		return false, c.send(serverwire.HeadBanner(policy.UnavailableText))
	}
	if errors.Is(err, game.ErrInventoryFull) || errors.Is(err, game.ErrItemLocked) {
		return false, c.send(serverwire.HeadBanner("Cannot transform. Free inventory space and release reserved equipment."))
	}
	if err != nil {
		return false, errors.Join(err, s.cancelInteraction(c))
	}
	s.adoptSavedCharacter(c, next)
	appearance, err := next.AppearancePacket(true)
	if err != nil {
		return false, err
	}
	model := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateModelTransform}.U32(next.ID).U8(policy.Model)
	s.broadcastWorld(c, appearance)
	s.broadcastWorld(c, model)
	packets := [][]byte{s.World.HideActor(c.view, es.mapID, policy.HideActor), model}
	var additions []game.Addition
	for i, previous := range beforeBag {
		if previous == next.Bag[i] {
			continue
		}
		if !previous.Empty() {
			packets = append(packets, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(i + 1), previous.Count})
		}
		if !next.Bag[i].Empty() {
			additions = append(additions, game.Addition{Slot: byte(i + 1), Count: next.Bag[i].Count})
		}
	}
	if len(additions) > 0 {
		packets = append(packets, next.Bag.AdditionPacket(additions))
	}
	packets = append(packets, next.EquipmentPacket())
	packets = append(packets, next.StatPackets(s.Assets.Items)...)
	for _, mark := range policy.Marks {
		packets = append(packets, s.World.QuestUpdate(c.view, mark.ID, next.Quests[mark.ID])...)
	}
	packets = append(packets, []byte{protocol.CommandEvent, protocol.EventStepComplete})
	return true, s.sendAll(c, packets)
}

func (s *Server) transformationAllowed(mapID uint16, ev *assets.Event, branch int, op world.Op) bool {
	_, ok := s.World.Transformation(mapID, ev, branch, op)
	return ok
}
