package server

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

func TestAdminAnnouncementChannelsAndRecipientIsolation(t *testing.T) {
	s, _, wires := chatFixture(t)
	for name, code := range map[string]byte{"notice": 4, "world": 1, "guild": 6, "whisper": 3, "banner": 16} {
		for _, w := range wires {
			w.Reset()
		}
		if err := s.AdminAnnouncement(" first | second\nthird ", name); err != nil {
			t.Fatal(err)
		}
		for _, w := range wires {
			got := w.packets(t)
			if len(got) != 3 {
				t.Fatal(got)
			}
			for i, text := range []string{"first", "second", "third"} {
				want := append([]byte{2, code, 0, 0, 0, 0}, []byte(text)...)
				if !bytes.Equal(got[i], want) {
					t.Fatal(got[i], want)
				}
			}
		}
	}
	for _, test := range []struct{ text, channel string }{{"hello", "unknown"}, {"\n| ", "notice"}, {"first|bad\x00", "world"}} {
		if err := s.AdminAnnouncement(test.text, test.channel); err == nil {
			t.Fatal("invalid announcement accepted")
		}
		for _, w := range wires {
			if w.Len() != 0 {
				t.Fatal("partial invalid announcement sent")
			}
		}
	}
}

func TestAdminLiveTabRefreshPreservesSQLFieldsAndWalking(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, skill := range c.character.Skills {
		s.Assets.Skills[skill.ID] = assets.Skill{}
	}
	row, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
	c.character.X += 10
	requested := row.State.Clone()
	requested.Bag = game.Inventory{}
	requested.Gold = 999
	requested.Map = 20000
	if err := s.EditAdminCharacterFields(ctx, c.character.ID, row.Version, requested, "inventory"); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != row.State.Gold || c.character.Map != row.State.Map || c.character.X != baseline.X+10 || c.character.Bag != (game.Inventory{}) {
		t.Fatal("unrelated state overwritten")
	}
	if c.autosaveBaseline == nil || c.autosaveBaseline.X != baseline.X {
		t.Fatal("pending walk checkpoint advanced")
	}
	if !contains(wires[0].packets(t), []byte{5, 4}) {
		t.Fatal("live refresh missing")
	}
	if err := s.EditAdminCharacterFields(ctx, c.character.ID, row.Version, requested, "inventory"); !errors.Is(err, store.ErrAdminConflict) {
		t.Fatal("stale edit accepted", err)
	}
	current, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	requested = current.State.Clone()
	requested.Bag[0] = game.Item{ID: 32176, Count: 1}
	wires[0].Reset()
	if err := s.EditAdminCharacterFields(canceled, c.character.ID, current.Version, requested, "inventory"); err == nil {
		t.Fatal("canceled edit committed")
	}
	if c.character.Bag != (game.Inventory{}) || wires[0].Len() != 0 {
		t.Fatal("failed edit published")
	}
	c.event = &eventSession{}
	if err := s.EditAdminCharacterFields(ctx, c.character.ID, current.Version, requested, "inventory"); !errors.Is(err, ErrAdminPlayerUnavailable) {
		t.Fatal("busy edit accepted", err)
	}
	c.event = nil
	if err := s.autosaveSession(ctx, c); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil || saved.State.X != baseline.X+10 || saved.State.Bag != (game.Inventory{}) {
		t.Fatal("checkpoint undid live edit", saved, err)
	}
}

func TestAdminLinkedPropEditVersionsAndMapScope(t *testing.T) {
	s, players, wires := chatFixture(t)
	ctx := context.Background()
	c := players[0]
	m := s.Assets.Maps[10017]
	m.NPCs = []assets.MapNPC{{ClickID: 7, Flags: 1, Template: 42, Events: []byte{9}, X: 100, Y: 200}}
	m.Events = []assets.Event{{ClickID: 9, Name: "Chest", Branches: []assets.Branch{{Index: 1, Condition: evCond(5, 900, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 7, 5, 1, 0)}}}}}
	s.Assets.Maps[10017] = m
	s.Assets.Marks = map[uint16]uint16{900: 0}
	s.World = world.New(s.Assets)
	reports := s.AdminNPCReport(42)
	if len(reports) != 1 || len(reports[0].Events) != 1 || len(reports[0].LinkedQuestIDs) != 1 || reports[0].LinkedQuestIDs[0] != 900 || reports[0].Events[0].Branches[0].Operations[0].Meaning != "prop frame / break animation" {
		t.Fatal(reports)
	}
	for _, skill := range c.character.Skills {
		s.Assets.Skills[skill.ID] = assets.Skill{}
	}
	row, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit := AdminActorEdit{Version: row.Version, ClickID: 7, Action: "open", Scope: "map"}
	if err = s.EditAdminActor(ctx, c.character.ID, edit); err != nil {
		t.Fatal(err)
	}
	if c.character.Quests[900].State != game.Completed || players[1].character.Quests[900].State == game.Completed {
		t.Fatal("quest ownership leaked")
	}
	want := []byte{22, 1, 7, 0, 1}
	if !contains(wires[0].packets(t), want) || !contains(wires[1].packets(t), want) || wires[2].Len() != 0 {
		t.Fatal("map publication scope")
	}
	if err = s.EditAdminActor(ctx, c.character.ID, edit); !errors.Is(err, store.ErrAdminConflict) {
		t.Fatal("stale prop edit", err)
	}
	row, err = s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	edit.Version = row.Version
	edit.Action = "close"
	edit.Scope = "session"
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err = s.EditAdminActor(canceled, c.character.ID, edit); err == nil {
		t.Fatal("failed mutation accepted")
	}
	if c.character.Quests[900].State != game.Completed || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("failed prop edit published")
	}
	if err = s.EditAdminActor(ctx, c.character.ID, edit); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.character.Quests[900]; ok {
		t.Fatal("linked quest not cleared")
	}
	if !contains(wires[0].packets(t), []byte{22, 1, 7, 0, 0}) || wires[1].Len() != 0 {
		t.Fatal("selected publication scope")
	}
	row, _ = s.Store.AdminCharacter(ctx, c.character.ID)
	edit.Version = row.Version
	edit.Action = "hide"
	edit.Scope = "map"
	if err = s.EditAdminActor(ctx, c.character.ID, edit); err != nil {
		t.Fatal(err)
	}
	if !c.view.Hidden[7] || !players[1].view.Hidden[7] || players[2].view.Hidden[7] {
		t.Fatal("map hide scope")
	}
	c.view.Reset()
	if len(c.view.AdminActors) != 0 {
		t.Fatal("actor override leaked into another map")
	}
}

func TestCustomQuestKillsPersistWithVictoryAndIgnoreCaptures(t *testing.T) {
	s, players, _ := combatSessions(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.QuestDefinitions = map[uint32]assets.QuestDefinition{700: {ID: 700, Title: "Slimes", Type: assets.QuestMonsterBattle, BattleMonsterID: 42, RequiredKillCount: 2}, 701: {ID: 701, Title: "Steps", Type: assets.QuestDialogue, Steps: []assets.QuestStep{{Index: 1, Type: assets.QuestMonsterBattle, BattleMonsterName: "slime", RequiredKillCount: 1}}}}
	c.character.Quests[700] = game.Quest{ID: 700, State: game.InProgress, Step: 1}
	c.character.Quests[701] = game.Quest{ID: 701, State: game.InProgress, Step: 1}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 42, Name: "Slime", HP: 1, Level: 1}, {Template: 42, Name: "Slime", HP: 1, Level: 1}}); err != nil {
		s.worldMu.Unlock()
		t.Fatal(err)
	}
	run := c.battle
	run.b.Defenders[0].HP = 0
	run.b.Defenders[1].Captured = true
	run.b.Defenders[1].HP = 0
	s.endBattle(run, battle.Victory)
	s.worldMu.Unlock()
	settle(t, s, c)
	row, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil || row.State.Quests[700].Kills != 1 || row.State.Quests[701].Kills != 1 || row.State.Quests[701].State != game.InProgress {
		t.Fatal("kill persistence or dormant progression", row, err)
	}
	s.worldMu.Lock()
	s.endBattle(run, battle.Victory)
	s.worldMu.Unlock()
	settle(t, s, c)
	row, _ = s.Store.AdminCharacter(ctx, c.character.ID)
	if row.State.Quests[700].Kills != 1 {
		t.Fatal("duplicate settlement counted")
	}
	next := row.State.Clone()
	pvp := &battleRun{b: &battle.Battle{PvP: true}}
	if len(s.customQuestKills(&next, pvp)) != 0 || next.Quests[700].Kills != 1 {
		t.Fatal("PvP counted")
	}
}

func TestAdminLiveStatsSettingsAndQuestRefresh(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, skill := range c.character.Skills {
		s.Assets.Skills[skill.ID] = assets.Skill{TableOrder: skill.ID}
	}
	s.Assets.Marks = map[uint16]uint16{900: 0}
	for _, scope := range []string{"stats", "player_settings", "quests"} {
		row, err := s.Store.AdminCharacter(ctx, c.character.ID)
		if err != nil {
			t.Fatal(err)
		}
		requested := row.State.Clone()
		requested.Gold = 999
		switch scope {
		case "stats":
			requested.Skills = nil
			requested.StatPoints = 7
		case "player_settings":
			prefs := requested.Preferences()
			prefs.TradeAllowed = false
			requested.Settings = &prefs
		case "quests":
			requested.Quests[900] = game.Quest{ID: 900, State: game.InProgress, Step: 2}
		}
		wires[0].Reset()
		if err := s.EditAdminCharacterFields(ctx, c.character.ID, row.Version, requested, scope); err != nil {
			t.Fatal(scope, err)
		}
		packets := wires[0].packets(t)
		if !contains(packets, []byte{5, 4}) || c.character.Gold != row.State.Gold {
			t.Fatal("unrelated state or missing refresh", scope)
		}
		switch scope {
		case "stats":
			want := skillSnapshotWithRemovals(row.State, *c.character)
			packet, err := want.BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
			if err != nil || !contains(packets, packet) || len(c.character.Skills) != 0 {
				t.Fatal("removed skills were not cleared", err)
			}
		case "player_settings":
			if !contains(packets, c.character.Preferences().Packet()) || c.character.Preferences().TradeAllowed {
				t.Fatal("settings refresh")
			}
		case "quests":
			if !contains(packets, []byte{24, 6, 1, 132, 3, 2}) {
				t.Fatal("journal refresh", packets)
			}
		}
	}
}

func TestCustomQuestKillFailedSaveDoesNotPublish(t *testing.T) {
	s, players, wires := combatSessions(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.QuestDefinitions = map[uint32]assets.QuestDefinition{700: {ID: 700, Title: "Bounty", Type: assets.QuestMonsterBattle, BattleMonsterID: 42}}
	c.character.Quests[700] = game.Quest{ID: 700, State: game.InProgress, Step: 1}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 42, Name: "Slime", HP: 1, Level: 1}}); err != nil {
		s.worldMu.Unlock()
		t.Fatal(err)
	}
	run := c.battle
	run.b.Defenders[0].HP = 0
	wires[0].Reset()
	if err := s.Store.Close(); err != nil {
		s.worldMu.Unlock()
		t.Fatal(err)
	}
	s.endBattle(run, battle.Victory)
	s.worldMu.Unlock()
	settle(t, s, c)
	if c.character.Quests[700].Kills != 0 {
		t.Fatal("failed kill saved in memory")
	}
	for _, p := range wires[0].packets(t) {
		if len(p) > 2 && p[0] == 23 && p[1] == 57 {
			t.Fatal("uncommitted quest receipt", p)
		}
	}
}
