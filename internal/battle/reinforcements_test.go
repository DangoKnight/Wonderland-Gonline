package battle

import (
	"bytes"
	"testing"
)

func TestReinforcementWavesRetainRewardsAndSpawnAtRoundEnd(t *testing.T) {
	self := &Fighter{Side: Attacker, Kind: Player, ID: 10001, X: 4, Y: 2, HP: 10000, MaxHP: 10000}
	var enemies []Enemy
	for i := 0; i < 17; i++ {
		enemies = append(enemies, Enemy{Template: uint32(10000 + i), Level: 2, HP: 200, Element: 1})
	}
	b := New([]*Fighter{self}, enemies)
	if len(b.Defenders) != 8 || !b.HasReinforcements() {
		t.Fatal("formation was truncated")
	}
	for _, f := range b.Defenders {
		f.HP = 0
	}
	steps, outcome := b.Round(Rules{})
	if outcome != Continue || len(steps) == 0 || len(b.DefenderHistory()) != 16 || b.Defenders[0].ID != 10008 || !b.HasReinforcements() {
		t.Fatal("early victory or wrong wave")
	}
	var spawn []byte
	for _, step := range steps {
		for _, p := range step.Packets {
			if len(p) > 1 && p[0] == 11 && p[1] == 5 {
				spawn = p
				break
			}
		}
		if spawn != nil {
			break
		}
	}
	want := []byte{11, 5, 2, 7, 0x18, 0x27, 0, 0, 0xd0, 7, 0, 0, 0, 0, 2, 2, 200, 0, 0, 0, 90, 0, 200, 0, 0, 0, 90, 0, 2, 1, 0, 0, 0, 0}
	if !bytes.Equal(spawn, want) {
		t.Fatal("reinforcement spawn wire", spawn)
	}
	for _, f := range b.Defenders {
		if f.HP != 200 {
			t.Fatal("reinforcement acted immediately")
		}
		f.HP = 0
	}
	_, outcome = b.Round(Rules{})
	if outcome != Continue || b.HasReinforcements() || b.Defenders[0].ID != 10016 || len(b.DefenderHistory()) != 17 {
		t.Fatal("second queue advancement")
	}
	for _, f := range b.Defenders {
		f.HP = 0
	}
	_, outcome = b.Round(Rules{})
	if outcome != Victory {
		t.Fatal("finished formation did not win")
	}
	if exp, gold := b.Rewards(); exp != 510 || gold != 272 {
		t.Fatal("prior waves lost rewards", exp, gold)
	}
}

func TestReinforcementsDoNotInheritStatusesOrRewardCapturedWave(t *testing.T) {
	self := &Fighter{Side: Attacker, Kind: Player, ID: 10001, X: 4, Y: 2, HP: 10000, MaxHP: 10000}
	var enemies []Enemy
	for i := 0; i < 9; i++ {
		enemies = append(enemies, Enemy{Template: uint32(10000 + i), Level: 2, HP: 200})
	}
	b := New([]*Fighter{self}, enemies)
	b.Defenders[0].Captured = true
	b.Defenders[0].Effects = []ActiveEffect{{Source: 50001, Turns: 3}}
	b.Defenders[0].HP = 0
	b.spawnReinforcements()
	if len(b.Defenders[0].Effects) != 0 || b.Defenders[0].Captured || b.Defenders[0].HP != 200 {
		t.Fatal("old occupant state leaked")
	}
	if exp, gold := b.Rewards(); exp != 240 || gold != 128 {
		t.Fatal("captured wave rewarded", exp, gold)
	}
	b.DiscardReinforcements()
	if b.HasReinforcements() {
		t.Fatal("force finish left queues")
	}
}
