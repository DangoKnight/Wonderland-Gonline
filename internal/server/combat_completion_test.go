package server

import (
	"context"
	"reflect"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

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
