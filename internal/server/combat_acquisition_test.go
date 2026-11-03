package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestCombatKillPropagatesOneDropToComboOwners(t *testing.T) {
	s, players, _ := combatSessions(t)
	a := battle.PlayerFighter(players[0].character, s.Assets.Items, 0)
	b := battle.PlayerFighter(players[1].character, s.Assets.Items, 1)
	pet := &battle.Fighter{Kind: battle.Pet, Owner: a.ID, X: 3, Y: 2, HP: 100, Pet: &game.Pet{ID: 10501}}
	run := &battleRun{b: battle.New([]*battle.Fighter{a, pet, b}, []battle.Enemy{{Template: 10500, HP: 100}}), members: []*battleMember{{c: players[0], self: a, pet: pet}, {c: players[1], self: b}}}
	for _, p := range players[:2] {
		p.battle = run
	}
	s.Assets.Items[46062] = game.ItemDefinition{ID: 46062, Type: 23}
	s.Assets.NPCs = map[uint16]assets.NPC{10500: {Drops: [5]uint16{46062}}}
	s.Assets.Drops = map[uint32][]assets.Drop{10500: {{Item: 46062, Min: 1, Max: 1, Rate: 10000}}}
	target := run.b.Defenders[0]
	target.HP = 0
	actors := []battle.Action{{Actor: a}, {Actor: pet}, {Actor: b}}
	step := s.commitCombatLoot(run, target, actors)
	if run.rewardError != nil {
		t.Fatal(run.rewardError)
	}
	for i, want := range []int{2, 1} {
		stored, err := s.Store.Characters(context.Background(), players[i].account.ID)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, item := range stored[0].Bag {
			if item.ID == 46062 {
				count += int(item.Count)
			}
		}
		if count != want {
			t.Fatalf("owner %d copies %d want %d", i, count, want)
		}
		packets := step.PacketsFor(players[i].character.ID)
		if len(packets) != want+1 || packets[0][1] != 6 || packets[len(packets)-1][1] != 4 {
			t.Fatal("grant/flight ordering", packets)
		}
	}
	if again := s.commitCombatLoot(run, target, actors); len(again.OwnerPackets) != 0 {
		t.Fatal("kill awarded twice")
	}
	s.stageBattleDisconnect(run, run.members[0])
	if err := s.saveBattleDisconnect(context.Background(), players[0]); err != nil {
		t.Fatal(err)
	}
	if players[0].character.Bag[0].ID == 0 {
		t.Fatal("disconnect lost committed rewards")
	}
}

func TestCombatCaptureCommitsBeforeRoundExitAndNotTwice(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	run := c.battle
	run.timer.Stop()
	catch := battle.Capture{Owner: c.character.ID, Template: 10500, Name: "Slime", Level: 50}
	step, ok := s.commitCombatCapture(run, catch)
	if !ok || len(step.PacketsFor(c.character.ID)) != 1 {
		t.Fatal("no recruitment", ok)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored[0].Pets) != 1 || stored[0].Pets[0].Level != 1 || stored[0].Pets[0].Exp != 0 {
		t.Fatal("capture not durable before exit", stored, err)
	}
	catch.Committed = true
	run.b.Captures = []battle.Capture{catch}
	next := c.character.Clone()
	s.petResults(run, run.member(c), &next, battle.Fled, c.pets)
	if len(next.Pets) != 1 {
		t.Fatal("settlement recruited again")
	}
}

func TestPetKnockoutPenalizesAmityOnly(t *testing.T) {
	s, c, _ := battlePetFixture(t)
	run := c.battle
	m := run.member(c)
	next := c.character.Clone()
	next.Pets[0].Exp = 123
	m.pet.Deaths = 1
	s.activePetResults(run, m, &next, battle.Defeat, c.pets)
	if next.Pets[0].Exp != 123 || next.Pets[0].Amity != c.character.Pets[0].Amity-1 {
		t.Fatal("pet knockout modified EXP or wrong amity", next.Pets[0])
	}
}
