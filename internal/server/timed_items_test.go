package server

import (
	"context"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestTimedItemConsumptionAndRejectionAreDurable(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.Items[34101] = game.ItemDefinition{ID: 34101, TimedKind: game.TimedItemStatue, TimedMinutes: 480, EXPTenths: 20}
	s.Assets.Items[34173] = game.ItemDefinition{ID: 34173, TimedKind: game.TimedItemPotion, TimedMinutes: 120, EXPTenths: 100}
	next := c.character.Clone()
	next.Bag[5] = game.Item{ID: 34101, Count: 2}
	next.Bag[6] = game.Item{ID: 34173, Count: 2}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(ctx, c, []byte{23, 96, 6}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[5].Count != 1 || c.character.TimedEffects.EXPSeconds != 28800 {
		t.Fatal("activation", c.character.TimedEffects)
	}
	wires[0].Reset()
	before := c.character.TimedEffects
	if err := s.worldCommand(ctx, c, []byte{23, 96, 7}); err != nil {
		t.Fatal(err)
	}
	if c.character.Bag[6].Count != 2 || c.character.TimedEffects != before {
		t.Fatal("incompatible item consumed")
	}
	if got := wires[0].packets(t); !reflect.DeepEqual(got, [][]byte{{5, 17, 2, 2}}) {
		t.Fatal("native rejection", got)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].TimedEffects != before || stored[0].Bag[5].Count != 1 {
		t.Fatal("reconnect state", err, stored)
	}
}

func TestTimedStopwatchNativePauseConsumesOneAndResumeConsumesNone(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	c := players[0]
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.Items[34180] = game.ItemDefinition{ID: 34180, TimedKind: game.TimedItemStopwatch}
	next := c.character.Clone()
	next.Bag[5] = game.Item{ID: 34180, Count: 2}
	next.TimedEffects = game.TimedEffects{EXPKind: game.TimedItemPotion, EXPTenths: 100, EXPSeconds: 7200}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{23, 110, 6}); err != nil {
		t.Fatal(err)
	}
	if !c.character.TimedEffects.EXPPaused || c.character.Bag[5].Count != 1 {
		t.Fatal("pause did not consume exactly one Stopwatch")
	}
	if got := wires[0].packets(t); !reflect.DeepEqual(got, [][]byte{{23, 9, 6, 1}, {23, 15}, {23, 204, 2, 1}}) {
		t.Fatal(got)
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, []byte{23, 110, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.TimedEffects.EXPPaused || c.character.Bag[5].Count != 1 {
		t.Fatal("resume consumed a Stopwatch")
	}
	if got := wires[0].packets(t); !reflect.DeepEqual(got, [][]byte{{23, 204, 2, 0}}) {
		t.Fatal(got)
	}
}

func TestTimedBoostScalesRegularEXPButNotTrainingBonus(t *testing.T) {
	for _, formula := range []game.BattleEXPFormula{game.BattleEXPSimple, game.BattleEXPParticipation} {
		s, c, _, _ := participationVictoryFixture(t)
		run := c.battle
		m := run.member(c)
		s.battleEXPFormula = formula
		s.expRateMultiplier = 1
		run.backdropLocation.Map = 13056
		s.Assets.Maps[13056] = assets.Map{TrainingArea: true, BattleVictoryEXP: 200}
		next := c.character.Clone()
		next.TimedEffects = game.TimedEffects{EXPKind: game.TimedItemPotion, EXPTenths: 100, EXPSeconds: 7200, TrainingSeconds: 3600}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		s.combatRules(run)
		player, pet := uint64(c.character.EXP), uint64(c.character.Pets[0].ClientTotalExp())
		regular, _ := run.b.Rewards()
		wantPlayer, wantPet := regular*10+200, regular*10+200
		if formula == game.BattleEXPParticipation {
			bonus := run.b.ParticipationBonus(200)
			wantPlayer = run.b.ParticipantEXP(m.self)*10 + bonus.Participant(m.self)
			wantPet = run.b.ParticipantEXP(m.pet)*10 + bonus.Pet(c.character.ID, c.character.Pets[0].ID)
		}
		s.endBattle(run, battle.Victory)
		settle(t, s, c)
		if uint64(c.character.EXP) != player+wantPlayer || uint64(c.character.Pets[0].ClientTotalExp()) != pet+wantPet {
			t.Fatal("boost/flat bonus", formula, c.character.EXP, c.character.Pets[0].ClientTotalExp(), wantPlayer, wantPet)
		}
	}
}

func TestTrainingExpiredVictoryGrantsNoRewards(t *testing.T) {
	s, c, _, _ := participationVictoryFixture(t)
	run := c.battle
	run.backdropLocation.Map = 13056
	s.Assets.Maps[13056] = assets.Map{TrainingArea: true, BattleVictoryEXP: 200}
	s.combatRules(run)
	before := c.character.Clone()
	s.endBattle(run, battle.Victory)
	settle(t, s, c)
	if c.character.EXP != before.EXP || c.character.Gold != before.Gold || c.character.Pets[0].ClientTotalExp() != before.Pets[0].ClientTotalExp() || c.character.Bag != before.Bag {
		t.Fatal("expired pass granted rewards")
	}
}

func TestTrainingExpiredBlocksAutomaticButAllowsExplicitMonsterChallenge(t *testing.T) {
	s, c, _ := fieldFixture(t)
	m := s.Assets.Maps[c.character.Map]
	m.TrainingArea = true
	s.Assets.Maps[c.character.Map] = m
	n, _ := s.World.NPC(c.character.Map, 4)
	if err := s.proximityEncounter(c, n); err != nil || c.battle != nil {
		t.Fatal("expired automatic encounter", err)
	}
	// Native AC11:2:3: template 17100, actor 4. No entitlement is active.
	if err := s.worldCommand(context.Background(), c, []byte{11, 2, 3, 204, 66, 0, 0, 4, 0}); err != nil || c.battle == nil {
		t.Fatal("manual monster challenge rejected", err)
	}
}

func TestTimedNativeGoldenPackets(t *testing.T) {
	for _, tt := range []struct {
		state game.TimedEffects
		want  [][]byte
	}{
		{game.TimedEffects{EXPKind: game.TimedItemStatue, EXPSeconds: 28800, EXPTenths: 20}, [][]byte{{5, 14, 1, 224, 1, 0, 0}}},
		{game.TimedEffects{EXPKind: game.TimedItemPotion, EXPSeconds: 7200, EXPTenths: 100}, [][]byte{{5, 17, 3, 1, 120, 0, 0, 0, 100}}},
		{game.TimedEffects{TrainingSeconds: 3600}, [][]byte{{23, 106, 1, 1, 60, 0, 1}}},
	} {
		if got := timedPackets(tt.state, true); !reflect.DeepEqual(got, tt.want) {
			t.Fatal(got, tt.want)
		}
	}
	if timedMinutes(game.MaxTimedSeconds) != 71582789 {
		t.Fatal("duration overflow")
	}
}

func TestTimedSessionTrainingClock(t *testing.T) {
	s, c, _, _ := participationVictoryFixture(t)
	s.Assets.Maps[c.character.Map] = assets.Map{TrainingArea: false}
	c.character.TimedEffects = game.TimedEffects{EXPKind: game.TimedItemPotion, EXPTenths: 100, EXPSeconds: 7200, TrainingSeconds: 3600}
	start := time.Now()
	c.timedTickAt = start
	s.advanceTimedSession(c, start.Add(60*time.Second), false)
	if c.character.TimedEffects.EXPSeconds != 7140 || c.character.TimedEffects.TrainingSeconds != 3600 {
		t.Fatal(c.character.TimedEffects)
	}
	s.Assets.Maps[c.character.Map] = assets.Map{TrainingArea: true}
	s.advanceTimedSession(c, start.Add(120*time.Second), false)
	if c.character.TimedEffects.TrainingSeconds != 3540 {
		t.Fatal(c.character.TimedEffects)
	}
	c.character.TimedEffects.TrainingSeconds = 0
	if s.trainingEligible(c) {
		t.Fatal("expired pass permits automatic encounters")
	}
}
