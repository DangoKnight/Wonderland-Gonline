package server

import (
	"bytes"
	"context"
	"testing"
	"time"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestPartySpeedChainPersistsEveryParticipantsSkill(t *testing.T) {
	battleSleep, turnTimeout = func(time.Duration) {}, time.Hour
	t.Cleanup(func() { battleSleep, turnTimeout = time.Sleep, 30*time.Second })
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
		if len(stored) == 0 || len(stored[0].Skills) != 1 || stored[0].Skills[0].Grade != 2 || stored[0].Skills[0].EXP != 0 || !stored[0].Reborn {
			t.Fatalf("member %d skill not saved: %+v", i, stored)
		}
	}
}
