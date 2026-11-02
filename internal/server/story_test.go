package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

// These scenarios use the exported native scripts in SQL, without editing them.
func TestNativeFredFinaleCompanionRewards(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, following := range []bool{false, true} {
		name := "not_following"
		if following {
			name = "following"
		}
		t.Run(name, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			next := c.character.Clone()
			next.Map = 11149
			next.Quests[13172] = game.Quest{ID: 13172, State: game.InProgress, Step: 4}
			next.Pets = []game.Pet{game.NewPet(14162, "Roca", 1, s.petTemplate(14162), catalog.Items)}
			next.ReservePets = []game.Pet{game.NewPet(14081, "Niss", 1, s.petTemplate(14081), catalog.Items)}
			if following {
				next.ActivePet = 14162
			}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			c.pets.register(14162)
			ev, ok := s.World.Event(11149, 2)
			if !ok {
				t.Fatal("native finale absent")
			}
			branch := s.World.FindBranch(c.character, c.view, 11149, ev, world.TriggerEntry, 0, 0, -1)
			wantBranch := byte(10)
			if following {
				wantBranch = 20
			}
			if branch < 0 || ev.Branches[branch].Index != wantBranch {
				t.Fatal("native finale gate", branch)
			}
			wire.Reset()
			if err := s.startEvent(context.Background(), c, 2, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			sawStar := false
			for i := 0; c.event != nil && i < 80; i++ {
				packets := wire.packets(t)
				if contains(packets, []byte{15, 20, 6, 3}) {
					sawStar = true
					if c.character.Quests[13173].Step != 0 || bagCount(c.character.Bag, 34064) != 0 {
						t.Fatal("reward before constellation completes")
					}
				}
				if c.event.onChoice != nil || c.event.onMinigame != nil || c.battle != nil {
					t.Fatal("unexpected native finale barrier")
				}
				if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			}
			if !sawStar || c.event != nil || c.character.Quests[13172].State != game.Completed || c.character.Quests[13173].Step != 1 || bagCount(c.character.Bag, 34064) != 1 {
				t.Fatal("native finale incomplete", sawStar, c.event, c.character.Quests[13173], bagCount(c.character.Bag, 34064))
			}
			if _, ok := c.character.Pet(12095); !ok {
				t.Fatal("Fred not recruited")
			}
			if len(c.character.ReservePets) != 1 || c.character.ReservePets[0].ID != 14081 {
				t.Fatal("Niss reserve lost")
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].Quests[13173].Step != 1 || bagCount(chars[0].Bag, 34064) != 1 || len(chars[0].Pets) != 2 {
				t.Fatal("finale not durable", err)
			}
		})
	}
}

func TestNativeMonkeyRescueController(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, wire := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	next := c.character.Clone()
	next.Map = 11016
	npc, ok := s.World.NPC(11016, 1)
	if !ok {
		t.Fatal("native Monkey actor absent")
	}
	next.X, next.Y = npc.X, npc.Y
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	c.view = world.NewView()
	if err := s.worldCommand(context.Background(), c, []byte{20, 1, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if packets := wire.packets(t); !contains(packets, []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, 1, 224, 46, 0, 0, 0, 0, 7}) {
		t.Fatal("installed authored Monkey cinematic missing", packets)
	}
	for i := 0; c.event != nil && i < 20; i++ {
		if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
	}
	if c.event != nil || c.character.Quests[12002].State != game.Completed {
		t.Fatal("native Monkey rescue incomplete")
	}
	if _, ok := c.character.Pet(17162); !ok {
		t.Fatal("native Monkey absent")
	}
}

func TestNativeElinReplacementWeaponConditions(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, c, _ := eventFixture(t)
	s.Assets, s.World = catalog, world.New(catalog)
	native, ok := catalog.Maps[11167]
	if !ok {
		t.Fatal("native Elin weapon map absent")
	}
	matched := 0
	for _, ev := range native.Events {
		for _, br := range ev.Branches {
			rule := world.DecodeCond(br.Condition)
			if rule.Kind != 17 {
				continue
			}
			matched++
			copy := ev
			positive := &assets.Event{ClickID: copy.ClickID, Branches: []assets.Branch{{Index: 1, Condition: br.Condition, Operations: []assets.Operation{evOp(1, 2, 1, 0, 10000, 0)}}}}
			weapon := rule.W3
			if byte(rule.W4) == world.CompareNotEqual {
				weapon = 0
			}
			c.character.Pets = []game.Pet{{ID: 14230, Slot: 1, Equipment: game.Equipment{2: {ID: weapon, Count: 1}}}}
			if got := s.World.FindBranch(c.character, c.view, 11167, positive, 0, 0, 0, -1); got != 0 {
				t.Fatal("native equipped weapon gate", rule, got)
			}
			c.character.Pets = nil
			if got := s.World.FindBranch(c.character, c.view, 11167, positive, 0, 0, 0, -1); got >= 0 {
				t.Fatal("native missing Elin gate", rule, got)
			}
		}
	}
	if matched != 3 {
		t.Fatal("native Elin gate census", matched)
	}
}

func TestStoryDinnerCheckpointAndClickRedirect(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Map = 12000
	next.Quests[12018] = game.Quest{ID: 12018, State: game.InProgress, Step: 1}
	next.Quests[12043] = game.Quest{ID: 12043, State: game.InProgress, Step: 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Marks[12018] = 0
	s.Assets.Maps[12000] = assets.Map{ID: 12000, NPCs: []assets.MapNPC{{ClickID: 1, Template: 14140, X: 1050, Y: 1080}}}
	s.World = world.New(s.Assets)
	if handled, err := s.storyClick(ctx, c, 1); handled || err != nil || c.character.Quests[12018].Step != 2 || !c.view.Marks[12018] {
		t.Fatal("dinner repair", handled, err)
	}
	if !contains(wire.packets(t), protocol.Builder{24, 1}.U16(12018).U8(2)) {
		t.Fatal("missing journal refresh")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Quests[12018].Step != 2 {
		t.Fatal("checkpoint not saved", err)
	}
	// Clive resumes the offer event instead of the ordinary click branch.
	next = c.character.Clone()
	next.Map = 11040
	next.Quests[13102] = game.Quest{ID: 13102, State: game.InProgress, Step: 4}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	ev := assets.Event{ClickID: 2, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 2, 4, 0, 10001, 0)}}}}
	s.Assets.Maps[11040] = assets.Map{ID: 11040, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	if handled, err := s.storyClick(ctx, c, 4); !handled || err != nil || c.event == nil || c.event.ev.ClickID != 2 {
		t.Fatal("clive checkpoint", handled, err)
	}
}
func TestNissRestControllerPreservesCompanionUntilCheckpoint(t *testing.T) {
	s, c, _ := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Map = 12052
	next.Pets = []game.Pet{{ID: 14081, Slot: 1, Name: "Niss", Level: 1, Amity: 60, HP: 10, MaxHP: 100, SP: 5, MaxSP: 50}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.pets.register(14081)
	s.Assets.NPCs = map[uint16]assets.NPC{14081: {ID: 14081, Name: "Niss", Type: 4}}
	s.Assets.Marks[13052] = 0
	ev := assets.Event{ClickID: 6, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(5, 13052, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 3, 2, 14081, 0, 0), evOp(2, 5, 13052, 1, 0, 1)}},
		{Index: 6, Condition: evCond(5, 13052, 1, 0, 5, 1), Operations: []assets.Operation{evOp(1, 3, 2, 14081, 0, 0), evOp(2, 2, 1, 0, 10001, 0), evOp(3, 5, 13052, 1, 0, 1)}},
		{Index: 12, Condition: evCond(5, 13052, 1, 0, 5, 2), Operations: []assets.Operation{evOp(1, 10, 1, 0, 0, 0), evOp(2, 5, 13052, 1, 0, 1)}},
	}}
	s.Assets.Maps[12052] = assets.Map{ID: 12052, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	if err := s.startEvent(ctx, c, 1, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if c.event == nil || c.event.ev.Branches[c.event.branch].Index != 6 || len(c.character.Pets) != 1 || c.character.Quests[13052].Step != 1 {
		t.Fatal("premature dismissal")
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || len(c.character.Pets) != 0 || len(c.character.ReservePets) != 1 || c.character.Quests[13052].Step != 3 {
		t.Fatal("rest controller state", c.event, c.character)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].ReservePets) != 1 || chars[0].Quests[13052].Step != 3 {
		t.Fatal("rest recovery", err)
	}
}

func TestWaterfallMirrorContinuationAndVisitReset(t *testing.T) {
	for _, event := range []uint16{1, 2, 3, 5} {
		t.Run(fmt.Sprint(event), func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			c.character.Map = 12464
			ev := assets.Event{ClickID: event, Branches: []assets.Branch{
				{Index: 1, Operations: []assets.Operation{evOp(1, 2, 6, 6, 1, 0)}},
				{Index: 2, Condition: evCond(7, 1, 30, 0, 0, 0), Operations: []assets.Operation{
					evOp(1, 2, 6, 2, 1, 0), evOp(2, 2, 7, 3, 1, 0),
				}},
				{Index: 3, Operations: []assets.Operation{evOp(1, 2, 8, 3, 1, 0), evOp(2, 1, 2, 10000, 0, 0)}},
				{Index: 4, Operations: []assets.Operation{evOp(1, 2, 9, 3, 1, 0), evOp(2, 1, 2, 10001, 0, 0)}},
			}}
			// The entry layout must not undo this visit's mirror/beam/door changes.
			s.Assets.Maps[12464] = assets.Map{ID: 12464,
				NPCs:   []assets.MapNPC{{ClickID: 6, Flags: 1}, {ClickID: 7}, {ClickID: 8}, {ClickID: 9}},
				Events: []assets.Event{ev}, PreEvents: []assets.Event{{Branches: []assets.Branch{{
					Condition: evCond(1, 0, 0, 0, 0, 0), Operations: []assets.Operation{
						evOp(1, 2, 6, 3, 1, 0), evOp(2, 2, 7, 2, 1, 0),
						evOp(3, 2, 8, 2, 1, 0), evOp(4, 2, 9, 2, 1, 0),
					},
				}}}},
			}
			s.World = world.New(s.Assets)
			if err := s.startEvent(ctx, c, 6, &ev, 0, true); err != nil {
				t.Fatal(err)
			}
			if c.event == nil || c.event.onChoice == nil {
				t.Fatal("mirror choice absent")
			}
			wire.Reset()
			if err := s.worldCommand(ctx, c, []byte{20, 9, 30}); err != nil {
				t.Fatal(err)
			}
			if c.event == nil || c.event.branch != 2 || !c.view.Actors[8] {
				t.Fatal("mirror choice did not continue to beam rules")
			}
			if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event == nil || c.event.branch != 3 || !c.view.Actors[9] {
				t.Fatal("beam rules did not continue forward to door rules")
			}
			if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
				t.Fatal("mirror sequence replayed an earlier branch")
			}
			if s.World.VisibleIn(c.character, c.view, 12464, 6) || !s.World.VisibleIn(c.character, c.view, 12464, 7) || c.view.Hidden[7] || c.view.Hidden[8] || c.view.Hidden[9] {
				t.Fatal("event completion restored the entry layout over runtime actors")
			}
			c.view.Reset()
			if !s.World.VisibleIn(c.character, c.view, 12464, 6) || s.World.VisibleIn(c.character, c.view, 12464, 7) {
				t.Fatal("mirror swap survived a new map visit")
			}
		})
	}
}

func TestFairyAltarMovieResumesDialogue(t *testing.T) {
	for _, tc := range []struct {
		mapID, event uint16
		mode         byte
	}{{12465, 2, 1}, {12465, 3, 1}, {12465, 1, 2}, {12464, 2, 2}} {
		t.Run(fmt.Sprintf("%d/%d", tc.mapID, tc.event), func(t *testing.T) {
			s, c, wire := eventFixture(t)
			c.character.Map = tc.mapID
			ev := assets.Event{ClickID: tc.event, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{
				evOp(1, 8, 1, 0, 0, 11076), evOp(2, 1, 2, 10000, 0, 0),
			}}}}
			if err := s.startEvent(context.Background(), c, 1, &ev, 0, true); err != nil {
				t.Fatal(err)
			}
			// Independent native AC20:1 bytes, including the mode at offset 10.
			want := []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, tc.mode, 0x44, 0x2b, 0, 0, 0, 0, 1}
			if !contains(wire.packets(t), want) || c.event == nil || c.event.onComplete == nil {
				t.Fatal("movie mode or completion barrier", wire.packets(t))
			}
			wire.Reset()
			if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if !contains(wire.packets(t), []byte{20, 1, 0, 0, 0, 2, 1, 7, 0, 0, 1, 0, 0, 0, 0, 0x10, 0x27, 0}) {
				t.Fatal("movie completion did not resume dialogue", wire.packets(t))
			}
			if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || c.character.Map != tc.mapID {
				t.Fatal("dialogue did not finish in its original map")
			}
		})
	}
}

// Exercise the unmodified native EVE branches as well as the synthetic sequence.
func TestNativeWaterfallMirrorChoices(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []uint16{1, 2, 3, 5} {
		t.Run(fmt.Sprint(event), func(t *testing.T) {
			s, c, _ := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			ctx := context.Background()
			next := c.character.Clone()
			next.Map = 12464
			next.Quests[13160] = game.Quest{ID: 13160, State: game.InProgress, Step: 1}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			ev, ok := s.World.Event(12464, event)
			if !ok {
				t.Fatal("native mirror event absent")
			}
			first, second := uint16(6), uint16(7)
			if event == 3 || event == 5 {
				first, second = 3, 4
			}
			for _, choice := range []byte{30, 31} {
				branch := s.World.FindBranch(c.character, c.view, 12464, ev, 0, 0, 0, -1)
				if branch != 0 {
					t.Fatal("native mirror choice gate", branch)
				}
				if err := s.startEvent(ctx, c, first, ev, branch, true); err != nil {
					t.Fatal(err)
				}
				if err := s.worldCommand(ctx, c, []byte{20, 9, choice}); err != nil {
					t.Fatal(err)
				}
				if c.event != nil {
					t.Fatal("native mirror sequence restarted or stalled")
				}
				if _, ok := c.view.Actors[8]; !ok {
					t.Fatal("native beam rules did not run")
				}
				if s.World.VisibleIn(c.character, c.view, 12464, first) != (choice == 31) || s.World.VisibleIn(c.character, c.view, 12464, second) != (choice == 30) {
					t.Fatal("native mirror swap lost after visibility synchronization", choice)
				}
			}
		})
	}
}

func TestStoryDefinitionsControlArbitraryContent(t *testing.T) {
	s, c, wire := eventFixture(t)
	const mapID = 65000
	ev := assets.Event{ClickID: 77, Branches: []assets.Branch{
		{Index: 1, Operations: []assets.Operation{evOp(1, 8, 1, 0, 0, 12345)}},
		{Index: 2, Operations: []assets.Operation{evOp(1, 2, 8, 3, 1, 0)}},
	}}
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, Events: []assets.Event{ev}, NPCs: []assets.MapNPC{{ClickID: 8}}}
	s.Assets.Story = assets.StoryDefinitions{Rules: []assets.StoryRule{
		{ID: "custom_movie", Kind: assets.StoryMovieMode, Scope: assets.StoryScope{Maps: []uint16{mapID}, Events: []uint16{77}}, Value: 1},
		{ID: "custom_link", Kind: assets.StoryContinue, Scope: assets.StoryScope{Maps: []uint16{mapID}, Events: []uint16{77}}},
		{ID: "custom_order", Kind: assets.StoryForwardOnly, Scope: assets.StoryScope{Maps: []uint16{mapID}, Events: []uint16{77}}},
	}}
	if err := assets.ValidateStoryCatalog(s.Assets); err != nil {
		t.Fatal(err)
	}
	s.World = world.New(s.Assets)
	c.character.Map = mapID
	if err := s.startEvent(context.Background(), c, 8, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, 1, 57, 48, 0, 0, 0, 0, 1}) {
		t.Fatal("custom movie did not use policy mode")
	}
	if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || !c.view.Actors[8] {
		t.Fatal("custom continuation not executed")
	}
	// Empty SQL definitions disable overrides; runtime must not reload seed files.
	s.World.ReloadStory(assets.StoryDefinitions{})
	c.view.Reset()
	wire.Reset()
	if err := s.startEvent(context.Background(), c, 8, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, 2, 57, 48, 0, 0, 0, 0, 1}) {
		t.Fatal("empty definitions restored a seed override")
	}
	if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || c.view.Actors[8] {
		t.Fatal("deleted continuation remained active")
	}
}

func TestStoryAutomaticContinuationLimit(t *testing.T) {
	s, c, wire := eventFixture(t)
	ev := assets.Event{ClickID: 77, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 9, 0, 0, 0, 0)}}}}
	other := ev
	other.ClickID = 78
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Events: []assets.Event{ev, other}}
	s.Assets.Story = assets.StoryDefinitions{Rules: []assets.StoryRule{
		{ID: "cycle_first", Kind: assets.StoryContinue, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{77}}, TargetEvent: 78},
		{ID: "cycle_second", Kind: assets.StoryContinue, Scope: assets.StoryScope{Maps: []uint16{10017}, Events: []uint16{78}}, TargetEvent: 77},
	}}
	s.World = world.New(s.Assets)
	if err := s.startEvent(context.Background(), c, 1, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("automatic cycle did not release interaction")
	}
}

func TestStoryRecruitmentRechecksSQLAndKeepsWalking(t *testing.T) {
	s, c, _ := monkeyFixture(t)
	ctx := context.Background()
	policy, ok := s.recruitmentPolicy(c, 1)
	if !ok {
		t.Fatal("missing seed policy")
	}
	es := &eventSession{mapID: c.character.Map, click: 1, ev: &assets.Event{Branches: []assets.Branch{{Index: 1}}}}
	c.event = es
	setAutosaveBaseline(c)
	c.character.X++
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Gold = 123; return nil }); err != nil {
		t.Fatal(err)
	}
	if err := s.finishRecruitment(ctx, c, es, policy); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != 123 || c.character.X != 1043 || len(c.character.Pets) != 1 {
		t.Fatal("fresh SQL state or walking lost", c.character.X, c.character.Gold)
	}
	c.event = es
	if err := s.finishRecruitment(ctx, c, es, policy); err != nil {
		t.Fatal(err)
	}
	if len(c.character.Pets) != 1 {
		t.Fatal("replayed recruitment duplicated companion")
	}
}

func TestStoryCinematicTriggersStayInTheirConfiguredMap(t *testing.T) {
	s, c, wire := eventFixture(t)
	code := byte(8)
	first, second := uint16(256), uint16(512)
	s.Assets.Story.Cinematics = []assets.StoryCinematic{
		{ID: "first", Role: "movie_handshake", Maps: []uint16{65000}, Trigger: assets.StoryPattern{Code: &code, D4: &first}, Steps: []assets.StoryCinematicStep{{Cues: []assets.StoryCue{{Kind: "event_resume", Self: true}}}}},
		{ID: "second", Role: "movie_handshake", Maps: []uint16{65001}, Trigger: assets.StoryPattern{Code: &code, D4: &second}, Steps: []assets.StoryCinematicStep{{Cues: []assets.StoryCue{{Kind: "event_resume", Self: true}}}}},
	}
	c.character.Map = 65001
	ev := &assets.Event{ClickID: 77, Branches: []assets.Branch{{Index: 1}}}
	es := &eventSession{mapID: 65001, ev: ev}
	if _, err := s.execute(context.Background(), c, es, world.DecodeOp(evOp(1, 8, 1, 0, 0, 1))); err != nil {
		t.Fatal(err)
	}
	if c.storm || contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("foreign map's trigger started a cinematic")
	}
	wire.Reset()
	if _, err := s.execute(context.Background(), c, es, world.DecodeOp(evOp(1, 8, 1, 0, 0, 2))); err != nil {
		t.Fatal(err)
	}
	if !c.storm || c.stormDefinition == nil || c.stormDefinition.ID != "second" || !contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("local trigger did not select its configured cinematic")
	}
}

const (
	beachMap   = 10035
	beachQuest = 12040
)

func TestSpiderStoryArrivalRecovery(t *testing.T) {
	for _, deathSaved := range []bool{false, true} {
		t.Run(map[bool]string{false: "farewell", true: "cleanup"}[deathSaved], func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			farewell := assets.Event{ClickID: 12, Branches: []assets.Branch{{Index: 1,
				Condition:  evCond(5, 13086, 1, 0, 5, 2),
				Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), evOp(2, 5, 13086, 2, 0, 0)},
			}}}
			s.Assets.Maps[11077] = assets.Map{ID: 11077, Events: []assets.Event{farewell}}
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Map = 11077
			next.Quests[13086] = game.Quest{ID: 13086, State: game.InProgress, Step: 2}
			if deathSaved {
				next.Quests[13087] = game.Quest{ID: 13087, State: game.InProgress, Step: 1}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			ran, err := s.storyArrival(ctx, c)
			if err != nil {
				t.Fatal(err)
			}
			if !deathSaved {
				if !ran || c.event == nil || c.event.click != 13 || c.event.transition || c.event.ev.ClickID != 12 {
					t.Fatal("farewell was not resumed", ran, c.event)
				}
				if !contains(wire.packets(t), serverwire.EventFrame(1, 3, 5, 1, 0, 10001, 1, 1)) {
					t.Fatal("missing farewell dialogue")
				}
				if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
			} else if ran || c.event != nil {
				t.Fatal("cleanup replayed the farewell")
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || chars[0].Quests[13086].State != game.Completed {
				t.Fatal("completion was not persisted", err)
			}
			wire.Reset()
			if ran, err := s.storyArrival(ctx, c); ran || err != nil || len(wire.packets(t)) != 0 {
				t.Fatal("completed story replayed", ran, err)
			}
		})
	}
}

func TestStormAndBeachRescue(t *testing.T) {
	beachTimeout = time.Hour
	defer func() { beachTimeout = 25 * time.Second }()
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	storm := evOp(2, 8, 1, 0, 0, 0)
	storm.Data[7], storm.Data[8] = 0x00, 0x7b // dialog4 = 0x7B00
	captain := assets.Event{ClickID: 11, Branches: []assets.Branch{{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 10, 0, 30100, 0), storm}}}}
	robinson := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Condition: evCond(5, beachQuest, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 1, 0, 20001, 0)}}}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 10, Flags: 1, X: 1050, Y: 1080, Events: []byte{11}}}, Events: []assets.Event{captain}}
	s.Assets.Maps[beachMap] = assets.Map{ID: beachMap, NPCs: []assets.MapNPC{{ClickID: 1, Flags: 1, X: 1000, Y: 2200}}, Events: []assets.Event{robinson}}
	s.World = world.New(s.Assets)
	c, wire := players[0], wires[0]
	do := func(p ...byte) [][]byte {
		t.Helper()
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
		return wire.packets(t)
	}
	do(12, 1)
	do(20, 1, 10, 0)
	if got := do(20, 6); len(got) < 2 || !bytes.Equal(got[0], []byte{20, 1, 0, 0, 0, 3, 5, 0, 0, 0, 2, 0x7b, 0, 0, 0, 0, 0, 0}) || !bytes.Equal(got[1], []byte{186, 12, 1, 0, 0, 0, 0}) || !c.storm {
		t.Fatal("storm", got)
	}
	if got := do(20, 6); len(got) != 0 || !c.storm {
		t.Fatal("premature storm ACK", got)
	}
	do(186, 9, 1, 0)
	got := do(20, 6)
	if !contains(got, protocol.Builder{12}.U32(c.character.ID).U16(beachMap).U16(1038).U16(2235).U16(0).U8(0)) || c.storm || !c.beachPending {
		t.Fatal("storm did not end at the beach", got)
	}
	got = do(12, 1)
	if !contains(got, protocol.Builder{32, 2}.U32(c.character.ID).U8(9)) || !contains(got, []byte{22, 11, 6, 0, 0xff, 0xff}) || c.beach == nil {
		t.Fatal("rescue start", got)
	}
	if !contains(got, []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, 1, 0xe8, 0x2e, 0, 0, 0, 0, 1}) || !contains(got, []byte{22, 12, 1, 11, 0, 5}) {
		t.Fatal("immediate wake-up and actor", got)
	}
	for _, want := range [][]byte{{24, 1, 8, 47, 1}, {20, 10}, {24, 5, 97, 0, 1}, {22, 12, 1, 1, 0, 6}} {
		if got = do(20, 6); !bytes.Equal(got[0], want) {
			t.Fatal("rescue step", got)
		}
	}
	got = do(20, 6)
	if !contains(got, []byte{20, 8}) || !contains(got, []byte{5, 4}) || c.beach != nil || c.emote != 0 || c.event != nil {
		t.Fatal("rescue end", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Map != beachMap {
		t.Fatal("beach destination", err)
	}
	if _, exists := chars[0].Quests[beachQuest]; exists {
		t.Fatal("presentation started Robinson quest")
	}
	if got = do(20, 1, 1, 0); c.event == nil || !contains(got, serverwire.EventFrame(1, 3, 1, 1, 0, 20001, 1, 0)) {
		t.Fatal("manual Robinson dialogue", got)
	}

}

func TestStarterShipSceneTransitions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		from, entry uint16
		destination world.Destination
		rescue      bool
	}{
		{"deck to arcade cabin", 10017, 2, world.Destination{Map: 10027, X: 674, Y: 1067}, false},
		{"cabin to deck", 10027, 1, world.Destination{Map: 10017, X: 1814, Y: 904}, false},
		{"shipwreck chapter", 10017, 1, world.Destination{Map: 10035, X: 1038, Y: 2235}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			c.character.Map = tc.from
			warpTarget := tc.destination.Map
			if tc.rescue {
				warpTarget = 10037
			} // Native chapter destination is normalized to rescue map 10035.
			s.Assets.Maps[tc.from] = assets.Map{ID: tc.from, Warps: []assets.Warp{{ClickID: tc.entry, MapID: warpTarget, X: uint32(tc.destination.X), Y: uint32(tc.destination.Y)}}}
			s.Assets.Maps[tc.destination.Map] = assets.Map{ID: tc.destination.Map}
			s.World = world.New(s.Assets)
			// Independent opcode bytes: player scene transition, with the warp ID.
			op := assets.Operation{Index: 1, Data: [21]byte{1, 3, 0, byte(tc.entry), 0}}
			ev := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{op}}}}
			s.worldMu.Lock()
			_, err := s.execute(context.Background(), c, &eventSession{ev: &ev}, world.DecodeOp(op))
			s.worldMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if c.character.Map != tc.destination.Map || c.character.X != tc.destination.X || c.character.Y != tc.destination.Y || c.beachPending != tc.rescue {
				t.Fatalf("transition went to %d (%d,%d), rescue=%v", c.character.Map, c.character.X, c.character.Y, c.beachPending)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || len(stored) != 1 || stored[0].Map != tc.destination.Map {
				t.Fatalf("transition not persisted: %v %v", stored, err)
			}
			if got := wires[0].packets(t); !contains(got, protocol.Builder{12}.U32(c.character.ID).U16(tc.destination.Map).U16(tc.destination.X).U16(tc.destination.Y).U16(0).U8(0)) {
				t.Fatal("missing destination packet")
			}
		})
	}
}

func TestNativeStarterShipCabinTransitions(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ from, to, x, y uint16 }{{10017, 10027, 674, 1067}, {10027, 10017, 1814, 904}} {
		t.Run(fmt.Sprintf("%d_to_%d", tc.from, tc.to), func(t *testing.T) {
			s, players, _ := worldFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			c := players[0]
			c.character.Map = tc.from
			ev, ok := s.World.Event(tc.from, 1)
			if !ok {
				t.Fatal("native door event missing")
			}
			s.worldMu.Lock()
			err := s.startEvent(context.Background(), c, 0, ev, 0, true)
			s.worldMu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			if c.character.Map != tc.to || c.character.X != tc.x || c.character.Y != tc.y || c.beachPending {
				t.Fatalf("native door went to %d (%d,%d), rescue=%v", c.character.Map, c.character.X, c.character.Y, c.beachPending)
			}
		})
	}
}
