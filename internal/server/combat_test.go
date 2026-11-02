package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func TestFailedBattleResultDoesNotPublishPetChanges(t *testing.T) {
	for _, capture := range []bool{false, true} {
		s, c, wire := battleFixture(t, 1000)
		run := c.battle
		if capture {
			run.b.Captures = []battle.Capture{{Owner: c.character.ID, Template: 10500, Name: "Slime", Level: 5}}
		} else {
			pet := game.Pet{ID: 10501, Slot: 1, Amity: 20, HP: 10, MaxHP: 10}
			c.character.Pets = []game.Pet{pet}
			c.character.ActivePet = pet.ID
			c.character.ActiveMount = pet.ID
			c.pets.register(pet.ID)
			run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
			run.members[0].pet.Deaths = 1
		}
		wire.Reset()
		before := c.character.Clone()
		s.Store.Close()
		s.endBattle(run, battle.Victory)
		if c.battle != nil || wire.Len() != 0 || len(c.character.Pets) != len(before.Pets) || c.character.ActivePet != before.ActivePet || c.character.ActiveMount != before.ActiveMount {
			t.Fatal("failed result published or adopted pet state")
		}
		if capture && len(c.pets.slots) != 0 {
			t.Fatal("failed capture registered client slot")
		}
		if !capture && c.pets.slot(10501) != 1 {
			t.Fatal("failed desertion removed client slot")
		}
	}
}

func TestCommittedDesertionAdoptsRoster(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	pet := game.Pet{ID: 10501, Slot: 1, Amity: 20, HP: 10, MaxHP: 10}
	c.character.Pets = []game.Pet{pet}
	c.character.ActivePet = pet.ID
	c.character.ActiveMount = pet.ID
	c.pets.register(pet.ID)
	run := c.battle
	run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
	run.members[0].pet.Deaths = 1
	wire.Reset()
	s.endBattle(run, battle.Defeat)
	settle(t, s, c)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored[0].Pets) != 0 || stored[0].ActivePet != 0 || stored[0].ActiveMount != 0 || c.pets.slot(pet.ID) != 0 {
		t.Fatal("desertion not durably adopted", err)
	}
	if !contains(wire.packets(t), []byte{19, 2}) {
		t.Fatal("committed desertion not sent")
	}
}

func combatSessions(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := chatFixture(t)
	oldSleep, oldTimeout := battleSleep, turnTimeout
	battleSleep = func(time.Duration) {}
	turnTimeout = time.Hour
	t.Cleanup(func() {
		battleSleep = oldSleep
		turnTimeout = oldTimeout
		for _, c := range players {
			if c.battle != nil && c.battle.timer != nil {
				c.battle.timer.Stop()
			}
		}
	})
	return s, players, wires
}

func pkRequest(id uint32) []byte { return protocol.Builder{11, 2, 3}.U32(id).U16(0) }

func TestNativePvPEntryActionsSettlementAndPersistence(t *testing.T) {
	s, players, wires := combatSessions(t)
	a, d := players[0], players[1]
	beforeA, beforeD := a.character.Clone(), d.character.Clone()
	if err := s.worldCommand(context.Background(), a, pkRequest(d.character.ID)); err != nil {
		t.Fatal(err)
	}
	run := a.battle
	if run == nil || d.battle != run || !run.b.PvP || len(run.members) != 2 {
		t.Fatal("PK not registered")
	}
	for i, wire := range wires[:2] {
		packets := wire.packets(t)
		found := false
		for _, p := range packets {
			if len(p) > 17 && p[0] == 11 && p[1] == 250 {
				found = true
				if p[4] != byte(i+1) || p[16] != byte(4-i*3) || p[17] != 2 {
					t.Fatal("native formation", p)
				}
			}
		}
		if !found {
			t.Fatal("missing self formation")
		}
	}
	// Defending player kills the challenger; there must be no automatic NPC turn.
	run.members[0].self.HP = 1
	run.members[1].self.Atk = 1000
	run.members[1].self.Spd = 1000
	if err := s.worldCommand(context.Background(), a, []byte{50, 4, 4, 2, 1, 2}); err != nil {
		t.Fatal(err)
	}
	if run.b.Processing {
		t.Fatal("round executed before defender submitted")
	}
	if err := s.worldCommand(context.Background(), d, []byte{50, 1, 1, 2, 4, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, a)
	settle(t, s, d)
	for i, before := range []game.Character{beforeA, beforeD} {
		c := players[i]
		if c.battle != nil || c.character.EXP != before.EXP || c.character.Gold != before.Gold || !reflect.DeepEqual(c.character.Bag, before.Bag) {
			t.Fatal("PvP granted rewards", i)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || stored[0].HP != c.character.HP || stored[0].SP != c.character.SP {
			t.Fatal("PvP vitals not saved", err)
		}
		if !contains(wires[i].packets(t), []byte{11, 12, 1}) {
			t.Fatal("missing PvP departure")
		}
	}
}

func TestNativePvPRejectsUnavailableAndMalformedTargets(t *testing.T) {
	s, players, _ := combatSessions(t)
	a, d := players[0], players[1]
	for _, p := range [][]byte{{11, 2}, {11, 2, 3, 1}, {11, 2, 3, 1, 0, 0, 0, 0, 0, 0}} {
		if err := s.worldCommand(context.Background(), a, p); err != protocol.ErrMalformed || a.battle != nil {
			t.Fatal("truncated PK", err)
		}
	}
	settings := d.character.Preferences()
	settings.PKAllowed = false
	d.character.Settings = &settings
	if err := s.worldCommand(context.Background(), a, pkRequest(d.character.ID)); err != nil || a.battle != nil || d.battle != nil {
		t.Fatal("PK preference ignored", err)
	}
	settings.PKAllowed = true
	d.character.Settings = &settings
	for _, id := range []uint32{a.character.ID, players[2].character.ID, 999999} {
		if err := s.worldCommand(context.Background(), a, pkRequest(id)); err != nil || a.battle != nil {
			t.Fatal("invalid target accepted", id, err)
		}
	}
}

func TestPvPDisconnectForfeitsWithoutReward(t *testing.T) {
	s, players, _ := combatSessions(t)
	a, d := players[0], players[1]
	gold, exp := a.character.Gold, a.character.EXP
	if err := s.startPvP(a, d); err != nil {
		t.Fatal(err)
	}
	s.abandonBattle(d)
	settle(t, s, a)
	if a.battle != nil || d.battle != nil || a.character.Gold != gold || a.character.EXP != exp {
		t.Fatal("forfeit failed or rewarded")
	}
}

func trialFixture(t *testing.T) (*Server, *Session, *captureConn) {
	s, players, wires := combatSessions(t)
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, Name: "Guardian", HP: 100, Level: 5, Skills: [3]uint16{11001}}}
	s.Assets.CombatTrials = []assets.CombatTrial{{Stage: 1, Name: "Test Palace", Map: 10017, Guardian: 10500, HP: 100, Attack: 50, Reward: 10002, Count: 1}}
	s.World = world.New(s.Assets)
	return s, players[0], wires[0]
}

func TestPalaceTrialVictoryOnlyAndDurableReward(t *testing.T) {
	for _, outcome := range []battle.Outcome{battle.Victory, battle.Defeat, battle.Fled} {
		t.Run(string(rune('0'+outcome)), func(t *testing.T) {
			s, c, wire := trialFixture(t)
			before := adminTestItemCount(c.character.Bag, 10002)
			if err := s.worldCommand(context.Background(), c, []byte{77, 1, 1}); err != nil {
				t.Fatal(err)
			}
			if c.battle == nil || adminTestItemCount(c.character.Bag, 10002) != before {
				t.Fatal("trial did not fight before reward")
			}
			if !contains(wire.packets(t), []byte{77, 1, 1, 1}) {
				t.Fatal("trial receipt")
			}
			run := c.battle
			if run.b.Defenders[0].Atk != 50 || run.b.Defenders[0].HP != 100 || run.b.Defenders[0].Skills[0] != 11001 {
				t.Fatal("trial SQL boss parameters ignored")
			}
			run.b.Defenders[0].HP = 0
			s.endBattle(run, outcome)
			settle(t, s, c)
			want := before
			if outcome == battle.Victory {
				want++
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || adminTestItemCount(c.character.Bag, 10002) != want || adminTestItemCount(stored[0].Bag, 10002) != want {
				t.Fatal("trial rewards", err, want, adminTestItemCount(c.character.Bag, 10002))
			}
			s.endBattle(run, battle.Victory)
			if adminTestItemCount(c.character.Bag, 10002) != want {
				t.Fatal("trial replay minted chest")
			}
		})
	}
}

func TestPalaceTrialRejectsMissingStageWrongMapAndFullBag(t *testing.T) {
	s, c, wire := trialFixture(t)
	if err := s.worldCommand(context.Background(), c, []byte{77, 1, 12}); err != nil || c.battle != nil || !contains(wire.packets(t), []byte{77, 1, 12, 0}) {
		t.Fatal("unconfigured trial accepted", err)
	}
	s.Assets.CombatTrials[0].Map = 20000
	if err := s.worldCommand(context.Background(), c, []byte{77, 1, 1}); err != nil || c.battle != nil {
		t.Fatal("wrong location accepted", err)
	}
	s.Assets.CombatTrials[0].Map = 10017
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 10001, Count: game.MaxItemStack}
	}
	if err := s.worldCommand(context.Background(), c, []byte{77, 1, 1}); err != nil || c.battle != nil {
		t.Fatal("full bag accepted", err)
	}
}

func TestCombatTrialAndLegacyRewardsSaveFailure(t *testing.T) {
	for _, trial := range []bool{false, true} {
		s, c, wire := trialFixture(t)
		if trial {
			if err := s.challengeTrial(c, 1, false); err != nil {
				t.Fatal(err)
			}
		} else {
			c.character.Quests[1012] = game.Quest{ID: 1012, State: game.InProgress, Step: 1}
			if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 17437, Name: "Wild Wolf", HP: 1, Level: 1}}); err != nil {
				t.Fatal(err)
			}
		}
		run := c.battle
		run.b.Defenders[0].HP = 0
		wire.Reset()
		before := c.character.Clone()
		s.Store.Close()
		s.endBattle(run, battle.Victory)
		if !reflect.DeepEqual(before, *c.character) || wire.Len() != 0 {
			t.Fatal("failed save published rewards")
		}
	}
}

func TestLegacyCombatRewardsCompletionReplayCaptureAndEventOwnership(t *testing.T) {
	for _, mode := range []string{"victory", "captured", "native_event", "defeat"} {
		t.Run(mode, func(t *testing.T) {
			s, players, _ := combatSessions(t)
			c := players[0]
			c.character.Quests[1012] = game.Quest{ID: 1012, State: game.InProgress, Step: 1}
			if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 17437, Name: "Wild Wolf", HP: 1, Level: 1}}); err != nil {
				t.Fatal(err)
			}
			run := c.battle
			run.b.Defenders[0].HP = 0
			outcome := battle.Victory
			if mode == "captured" {
				run.b.Defenders[0].Captured = true
			}
			if mode == "native_event" {
				run.event = &eventSession{}
			}
			if mode == "defeat" {
				outcome = battle.Defeat
			}
			s.endBattle(run, outcome)
			settle(t, s, c)
			q := c.character.Quests[1012]
			if mode == "victory" {
				if q.State != game.Completed || q.CompletedAt == nil || c.character.Gold != 408 {
					t.Fatal("legacy quest reward", q, c.character.Gold)
				}
				if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 17437, HP: 1, Level: 1}}); err != nil {
					t.Fatal(err)
				}
				run = c.battle
				run.b.Defenders[0].HP = 0
				s.endBattle(run, battle.Victory)
				settle(t, s, c)
				if c.character.Gold != 416 {
					t.Fatal("quest reward repeated", c.character.Gold)
				}
			} else if q.State != game.InProgress {
				t.Fatal("legacy reward bypassed ownership", mode, q)
			}
		})
	}
}

func TestLegacyCombatCompanionRewardsUseCorrectTemplates(t *testing.T) {
	for _, test := range []struct {
		quest, monster, pet uint32
		name                string
	}{{1005, 11066, 14081, "Niss"}, {1010, 14155, 14156, "Xaolan"}} {
		s, players, _ := combatSessions(t)
		c := players[0]
		s.Assets.NPCs = map[uint16]assets.NPC{uint16(test.pet): {ID: uint16(test.pet), Name: test.name, Type: 2}}
		s.World = world.New(s.Assets)
		c.character.Quests[test.quest] = game.Quest{ID: test.quest, State: game.InProgress, Step: 1}
		if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: test.monster, HP: 1, Level: 1}}); err != nil {
			t.Fatal(err)
		}
		run := c.battle
		run.b.Defenders[0].HP = 0
		s.endBattle(run, battle.Victory)
		settle(t, s, c)
		if c.character.Quests[test.quest].State != game.Completed || len(c.character.Pets) != 1 || c.character.Pets[0].ID != test.pet {
			t.Fatal("wrong companion", c.character.Pets)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || len(stored[0].Pets) != 1 || stored[0].Pets[0].ID != test.pet {
			t.Fatal("companion/quest not saved together", err)
		}
	}
}

func TestPvPPartyParticipationAndDefendingGMWin(t *testing.T) {
	s, players, _ := combatSessions(t)
	a, d, ally := players[0], players[1], players[2]
	ally.character.Map = a.character.Map
	// Use the native party builder to retain member ordering and ownership.
	party := &party{members: []*Session{d, ally}}
	d.party = party
	ally.party = party
	if err := s.startPvP(a, d); err != nil {
		t.Fatal(err)
	}
	run := a.battle
	if len(run.members) != 3 || run.b.Expected() != 3 || run.members[2].self.X != 1 || run.members[2].self.Y != 3 {
		t.Fatal("defending party formation")
	}
	if !s.AdminBattles()[0].PvP {
		t.Fatal("admin battle kind missing")
	}
	if err := s.gmCombat(context.Background(), d, "winbattle", nil); err != nil {
		t.Fatal(err)
	}
	settle(t, s, a)
	settle(t, s, d)
	settle(t, s, ally)
	if run.members[0].self.HP != 0 || run.members[1].self.HP == 0 || run.members[2].self.HP == 0 {
		t.Fatal("GM killed their own side")
	}
}

func TestLegacyCompanionRewardCapacityAndFailedSave(t *testing.T) {
	for _, failSave := range []bool{false, true} {
		s, players, wires := combatSessions(t)
		c := players[0]
		s.Assets.NPCs = map[uint16]assets.NPC{14081: {ID: 14081, Name: "Niss", Type: 2}}
		s.World = world.New(s.Assets)
		c.character.Quests[1005] = game.Quest{ID: 1005, State: game.InProgress, Step: 1}
		if !failSave {
			for i := 0; i < game.MaxPets; i++ {
				c.character.Pets = append(c.character.Pets, game.Pet{ID: uint32(10500 + i), Slot: byte(i + 1)})
			}
		}
		if err := s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: 11066, HP: 1, Level: 1}}); err != nil {
			t.Fatal(err)
		}
		run := c.battle
		run.b.Defenders[0].HP = 0
		wires[0].Reset()
		before := c.character.Clone()
		if failSave {
			s.Store.Close()
		}
		s.endBattle(run, battle.Victory)
		settle(t, s, c)
		if c.character.Quests[1005].State != game.InProgress || len(c.character.Pets) != len(before.Pets) {
			t.Fatal("companion quest completed without recruitment")
		}
		if failSave && (!reflect.DeepEqual(before, *c.character) || wires[0].Len() != 0 || len(c.pets.slots) != 0) {
			t.Fatal("failed save published companion success")
		}
	}
}

func TestFinishedBattleStopsAnimationPlayback(t *testing.T) {
	s, players, wires := combatSessions(t)
	if err := s.startPvP(players[0], players[1]); err != nil {
		t.Fatal(err)
	}
	run := players[0].battle
	run.b.Finished = true
	run.timer.Stop()
	wires[0].Reset()
	wires[1].Reset()
	if s.play(run, []battle.Step{{Packets: [][]byte{{50, 1, 1}}}}) || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("animation continued after ending")
	}
}

func TestPalaceTrialRejectsBusyPartyMember(t *testing.T) {
	s, players, wires := combatSessions(t)
	c, teammate := players[0], players[1]
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, HP: 100, Level: 5}}
	s.Assets.CombatTrials = []assets.CombatTrial{{Stage: 1, Map: 10017, Guardian: 10500, HP: 100, Attack: 50, Reward: 10002, Count: 1}}
	p := &party{members: []*Session{c, teammate}}
	c.party = p
	teammate.party = p
	teammate.storm = true
	if err := s.worldCommand(context.Background(), c, []byte{77, 1, 1}); err != nil || c.battle != nil || teammate.battle != nil {
		t.Fatal("trial took ownership of busy teammate", err)
	}
	if !contains(wires[0].packets(t), []byte{77, 1, 1, 0}) {
		t.Fatal("busy party was accepted")
	}
}

// battleFixture: NPC 5 next to Alice fights monster template 10500 (level 5, HP hp)
// and has victory (result 1) and defeat (result 2) callback branches.
func battleFixture(t *testing.T, hp uint32) (*Server, *Session, *captureConn) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, defaultTurnTimeout })
	s, players, wires := worldFixture(t)
	fight := evOp(2, 6, 10500, 0, 5, hp<<8)
	fight.Data[7], fight.Data[8] = 0, 0 // Value packs HP in dword1; dialog4 high byte 0.
	copy(fight.Data[9:13], []byte{byte(hp), byte(hp >> 8), byte(hp >> 16), byte(hp >> 24)})
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(0, 0, 0, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), fight}},
		{Index: 2, Condition: evCond(4, 10500, 1, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10002, 0)}},
		{Index: 3, Condition: evCond(4, 10500, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10003, 0)}},
	}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 5, Flags: 1, X: 1050, Y: 1080, Events: []byte{1}}}, Events: []assets.Event{ev}}
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, Name: "Slime"}}
	s.World = world.New(s.Assets)
	c := players[0]
	ctx := context.Background()
	for _, p := range [][]byte{{12, 1}, {20, 1, 5, 0}, {20, 6}} {
		if err := s.worldCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	return s, c, wires[0]
}

// settle waits for the background animation player to release the battle.
func settle(t *testing.T, s *Server, c *Session) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		s.worldMu.Lock()
		done := c.battle == nil || (!c.battle.b.Processing && !c.battle.b.Finished)
		s.worldMu.Unlock()
		if done {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("battle did not settle")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestQuestBattleVictory(t *testing.T) {
	s, c, wire := battleFixture(t, 1)
	got := wire.packets(t)
	if !contains(got, []byte{20, 12}) || !contains(got, battleState(c.character.ID, true)) || !contains(got, []byte{52, 1}) || c.battle == nil {
		t.Fatal("battle start", got)
	}
	ctx := context.Background()
	// Movement and portals are ignored during battle.
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 0}.U16(1).U16(1)); err != nil || c.character.X != 1042 {
		t.Fatal("moved during battle", err)
	}
	if err := s.worldCommand(ctx, c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	got = wire.packets(t)
	if !contains(got, []byte{53, 5, 4, 2}) || !contains(got, []byte{53, 3, 2, 2}) || !contains(got, []byte{11, 12, 1}) || !contains(got, battleState(c.character.ID, false)) {
		t.Fatal("victory", got)
	}
	// The victory callback branch speaks next.
	if !bytes.Equal(got[len(got)-1], serverwire.EventFrame(1, 3, 5, 1, 0, 10002, 1, 2)) || c.battle != nil || c.event == nil {
		t.Fatal("victory branch", got[len(got)-1])
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	// Level 5 grants 75 EXP (level 3 at 81 total) and 40 gold.
	if err != nil || chars[0].EXP != 81 || chars[0].Level != 3 || chars[0].Gold != 40 || chars[0].StatPoints != 6 {
		t.Fatal("rewards", chars[0].EXP, chars[0].Level, chars[0].Gold, err)
	}
}

func TestQuestBattleDefeat(t *testing.T) {
	s, c, wire := battleFixture(t, 50000)
	wire.Reset()
	s.worldMu.Lock()
	c.battle.members[0].self.HP = 1
	s.worldMu.Unlock()
	if err := s.worldCommand(context.Background(), c, []byte{50, 4, 4, 2, 0, 0}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	got := wire.packets(t)
	if !bytes.Equal(got[len(got)-1], serverwire.EventFrame(1, 3, 5, 1, 0, 10003, 1, 3)) || c.character.HP < 10 {
		t.Fatal("defeat branch", got[len(got)-1], c.character.HP)
	}
}

func TestComboDamageStartupSelectionAndRestart(t *testing.T) {
	s, _, _ := worldFixture(t)
	for _, enabled := range []bool{false, true} {
		cfg := config.Default()
		cfg.ComboDamagePerParticipant = enabled
		selected := New(cfg, s.Store, s.Assets, s.Log)
		if selected.rules().ComboDamagePerParticipant != enabled {
			t.Fatal("startup option did not reach battle rules")
		}
		path := filepath.Join(t.TempDir(), "config.json")
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		selected.SetConfigurationPath(path)
		snap, err := selected.StartupConfiguration()
		if err != nil {
			t.Fatal(err)
		}
		next := snap.Configuration
		next.ComboDamagePerParticipant = !enabled
		if err = selected.SaveStartupConfiguration(snap.Version, next); err != nil {
			t.Fatal(err)
		}
		if selected.rules().ComboDamagePerParticipant != enabled || selected.Config.ComboDamagePerParticipant != enabled {
			t.Fatal("configuration file edit changed running combo rules")
		}
		loaded, err := config.Load(path)
		if err != nil || loaded.ComboDamagePerParticipant == enabled {
			t.Fatal("new setting not saved", err)
		}
		restarted := New(loaded, s.Store, s.Assets, s.Log)
		if restarted.rules().ComboDamagePerParticipant == enabled {
			t.Fatal("restart did not apply setting")
		}
		if err = selected.LoadRuntimeSettings(context.Background()); err != nil {
			t.Fatal(err)
		}
		if selected.rules().ComboDamagePerParticipant != enabled {
			t.Fatal("runtime settings changed combo damage")
		}
	}
}

func TestPartySpeedChainPersistsEveryParticipantsSkill(t *testing.T) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, defaultTurnTimeout })
	s, players, wires := partyFixture(t)
	ctx := context.Background()
	leader := players[0]
	for _, p := range players[1:] {
		if err := s.partyCommand(ctx, p, protocol.Builder{13, 1}.U32(leader.character.ID)); err != nil {
			t.Fatal(err)
		}
		if err := s.partyCommand(ctx, leader, protocol.Builder{13, 3, 1}.U32(p.character.ID)); err != nil {
			t.Fatal(err)
		}
	}
	single := false
	s.Assets.Skills = map[uint16]assets.Skill{11001: {ID: 11001, SP: 10, EffectLayer: 2, StatMultiplier: 2, AreaAttack: &single}}
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {ID: 10500, Name: "Slime"}}
	for _, p := range players {
		p.character.Reborn = true
		p.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	}
	s.worldMu.Lock()
	err := s.startBattle(leader, &battleRun{}, []battle.Enemy{{Template: 10500, Name: "Slime", Level: 75, HP: 10000}})
	if err == nil {
		for i, m := range leader.battle.members {
			m.self.Spd = []int{30, 60, 150}[i]
			m.self.Level = 1
			m.self.SP = 50
		}
		leader.battle.b.Defenders[0].Spd = 0
	}
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	defer s.abandonBattle(leader)
	for _, w := range wires {
		w.Reset()
	}
	for _, p := range players {
		var f *battle.Fighter
		for _, m := range leader.battle.members {
			if m.c == p {
				f = m.self
			}
		}
		if f == nil {
			t.Fatal("missing party member")
		}
		if err := s.worldCommand(ctx, p, []byte{50, 1, f.X, f.Y, 2, 2, 249, 42}); err != nil {
			t.Fatal(err)
		}
	}
	settle(t, s, leader)
	// Each hit removes less than 1% of 10,000 HP: 10 base + ceil(percent) = 11 EXP.
	// Starting at 99 leaves 10 EXP after the 100-point grade threshold.
	for i, p := range players {
		combos := 0
		for _, packet := range wires[i].packets(t) {
			if bytes.HasPrefix(packet, []byte{50, 1}) && len(packet) == 59 {
				combos++
			}
		}
		if combos != 1 {
			t.Fatalf("member %d received %d full-party animations", i, combos)
		}
		stored, err := s.Store.Characters(ctx, p.account.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored) == 0 || len(stored[0].Skills) != 1 || stored[0].Skills[0].Grade != 2 || stored[0].Skills[0].EXP != 10 || !stored[0].Reborn {
			t.Fatalf("member %d skill not saved: %+v", i, stored)
		}
	}
}

func TestSQLControlAbilityBlocksWithoutDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11051] // Native Freezing Spell: code 1, control layer.
	if len(skill.Effects) != 1 || !skill.Effects[0].BlockActions {
		t.Fatal("SQL control definition missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11051, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 120
	// Cast before the monster so the restriction suppresses this round's turn.
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 2, 2, 43, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	blocked, targetHP, remainingSP, ownerHP := !run.b.Defenders[0].CanAct(), run.b.Defenders[0].HP, actor.SP, actor.HP
	s.worldMu.Unlock()
	if !blocked || targetHP != 1000 || remainingSP != 120-int(skill.SP) || ownerHP != hp {
		t.Fatal("control became damage or failed to suppress enemy turn", blocked, targetHP, remainingSP, ownerHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11051 {
			// A successful status cast awards 15 support EXP plus 25 buff EXP.
			found = sk.EXP == 40
		}
	}
	if !found {
		t.Fatal("successful control cast did not persist proficiency")
	}
}

func TestSQLVanishProtectsAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[15190]
	if len(skill.Effects) != 1 || !skill.Effects[0].MissPhysicalAttacks || !skill.Effects[0].MissAreaAttacks {
		t.Fatal("SQL Vanish protection missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 15190, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 86, 59}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP := actor.HasEffect(15190), actor.HP, actor.SP
	s.worldMu.Unlock()
	if !protected || remainingHP != hp || remainingSP != 300-int(skill.SP) {
		t.Fatal("Vanish cast failed to protect", protected, remainingHP, remainingSP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15190 {
			// A successful status cast awards 15 support EXP plus 25 buff EXP.
			found = sk.EXP == 40
		}
	}
	if !found {
		t.Fatal("Vanish proficiency not saved")
	}
}

func TestSQLStoneWallProtectsWithoutDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11055]
	if len(skill.Effects) != 1 || !skill.Effects[0].MissPhysicalAttacks || skill.Effects[0].MissAreaAttacks {
		t.Fatal("SQL Stone Wall protection missing", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11055, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	hp := actor.HP
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 47, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11055), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != hp || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Stone Wall failed to protect or damaged an enemy", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11055 {
			// A successful status cast awards 15 support EXP plus 25 buff EXP.
			found = sk.EXP == 40
		}
	}
	if !found {
		t.Fatal("Stone Wall proficiency not saved")
	}
}

func TestSQLWaterShieldAllowsPhysicalDamageAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11080]
	if len(skill.Effects) != 1 || len(skill.Effects[0].Modifiers) != 1 || skill.Effects[0].Modifiers[0].Stat != "magical_damage_taken" {
		t.Fatal("SQL Water Shield scope incorrect", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11080, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	actor.HP, actor.MaxHP = 1000, 1000
	actor.Def = 20
	run.b.Defenders[0].Atk = 100 // Native monster basic attack deals 100 physical damage.
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 72, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11080), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != 900 || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Water Shield blocked physical damage or cast incorrectly", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11080 {
			// A successful status cast awards 15 support EXP plus 25 buff EXP.
			found = sk.EXP == 40
		}
	}
	if !found {
		t.Fatal("Water Shield proficiency not saved")
	}
}

func TestSQLShieldDefenseBuffsStatsAndSavesProgress(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	skill := catalog.Skills[11057]
	if len(skill.Effects) != 1 || len(skill.Effects[0].Modifiers) != 2 || skill.Effects[0].Modifiers[0].Stat != "def" || skill.Effects[0].Modifiers[1].Stat != "mdef" {
		t.Fatal("SQL Shield Defense scope incorrect", skill)
	}
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills = catalog.Skills
	c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: 11057, Grade: 1})
	run := c.battle
	actor := run.members[0].self
	actor.Char = c.character
	actor.SP = 300
	actor.Spd = 100
	actor.HP, actor.MaxHP = 1000, 1000
	actor.Def = 20
	run.b.Defenders[0].Atk = 100 // Native monster basic attack deals 100 physical damage.
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 4, 2, 49, 43}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	s.worldMu.Lock()
	protected, remainingHP, remainingSP, enemyHP := actor.HasEffect(11057), actor.HP, actor.SP, run.b.Defenders[0].HP
	s.worldMu.Unlock()
	if !protected || remainingHP != 902 || remainingSP != 300-int(skill.SP) || enemyHP != 1000 {
		t.Fatal("Shield Defense failed to increase defense or cast incorrectly", protected, remainingHP, remainingSP, enemyHP)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 11057 {
			// A successful status cast awards 15 support EXP plus 25 buff EXP.
			found = sk.EXP == 40
		}
	}
	if !found {
		t.Fatal("Shield Defense proficiency not saved")
	}
}

func TestCompiledElementalGrowthClinicAndPersistence(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	c.character.Base = game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}
	c.character.Element = game.Earth
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	c.character.HP = 10
	if err := s.openService(c, 6); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{31, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character.HP != 201 || c.character.SP != 121 || c.character.MaxHP != 201 || c.character.MaxSP != 121 {
		t.Fatal("clinic ignored compiled growth", c.character)
	}
	found := false
	for _, p := range wires[0].packets(t) {
		if len(p) == 12 && p[0] == 8 && p[2] == 41 {
			found = true
			if binary.LittleEndian.Uint32(p[4:8]) != 10 {
				t.Fatal("native ATK contribution", p)
			}
		}
	}
	if !found {
		t.Fatal("missing compiled stat update")
	}
	assertProgressSaved(t, s, c)
}

func TestCompiledElementalGrowthLoginPersistsWithoutHealing(t *testing.T) {
	s, players, _ := worldFixture(t)
	if err := s.Store.UpdateCharacter(context.Background(), players[0].account.ID, players[0].character.ID, func(stored *game.Character) error {
		stored.Element = game.Water
		stored.Base.Wisdom = 5
		stored.HP, stored.SP = 100, 50
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	c := &Session{account: players[0].account, conn: &captureConn{}, info: SessionInfo{ID: 99}}
	if err := s.dispatch(context.Background(), c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character == nil || c.character.MaxHP != 181 || c.character.MaxSP != 121 || c.character.HP != 100 || c.character.SP != 50 {
		t.Fatalf("login did not apply growth without healing: %+v", c.character)
	}
	assertProgressSaved(t, s, c)
}
