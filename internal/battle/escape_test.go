package battle

import (
	"bytes"
	"testing"
)

func TestEscapeLevelRuleAndStrongestLivingEnemy(t *testing.T) {
	for _, tc := range []struct {
		actor, enemy, roll int
		success            bool
	}{
		{1, 5, 24, false}, {1, 5, 25, true}, {10, 60, 34, false}, {10, 60, 35, true},
		{10, 120, 44, false}, {10, 120, 45, true}, {60, 60, 0, true}, {61, 60, 0, true},
	} {
		b, actor := setup(10000)
		actor.Level, b.Defenders[0].Level = byte(tc.actor), byte(tc.enemy)
		dead := *b.Defenders[0]
		dead.Level, dead.HP = 200, 0
		b.Defenders = append(b.Defenders, &dead)
		r := lowest
		calls := 0
		r.Next = func(lo, hi int) int {
			calls++
			if lo != 0 || hi != 100 {
				t.Fatal("bad roll bounds")
			}
			return tc.roll
		}
		if got := b.canEscape(r, actor); got != tc.success || (calls == 0) != (tc.actor >= tc.enemy) {
			t.Fatal("escape level rule", tc, got, calls)
		}
		stronger := *b.Defenders[0]
		stronger.Level = 120
		b.Defenders = append(b.Defenders, &stronger)
		if tc.actor == 10 && tc.enemy == 60 && tc.roll == 35 && b.canEscape(r, actor) {
			t.Fatal("ignored strongest living enemy")
		}
	}
}

func escapeParty() (*Battle, *Fighter, *Fighter, *Fighter) {
	b, leader := setup(10000)
	leader.HP, leader.MaxHP = 1000, 1000
	member := &Fighter{Kind: Player, Side: Attacker, ID: 8, X: 4, Y: 3, HP: 80, MaxHP: 100, SP: 20, MaxSP: 30, Level: 50}
	pet := &Fighter{Kind: Pet, Side: Attacker, ID: 102, Owner: 8, X: 3, Y: 3, HP: 70, MaxHP: 100, SP: 10, MaxSP: 20, Level: 50}
	b.Attackers = append(b.Attackers, member, pet)
	b.EscapeLeaders = map[uint32]bool{leader.ID: true}
	return b, leader, member, pet
}

func TestEscapeEitherActorWithdrawsOnlyOwnerAndPet(t *testing.T) {
	for _, petActs := range []bool{false, true} {
		b, leader, member, pet := escapeParty()
		actor, other := member, pet
		if petActs {
			actor, other = pet, member
		}
		b.Idle(leader)
		b.Pending[actor.key()] = Action{Actor: actor, Kind: "flee"}
		b.Pending[other.key()] = Action{Actor: other, Kind: "attack", Skill: 10001, TX: 2, TY: 2}
		steps, outcome := b.Round(lowest)
		if outcome != Continue || !member.Withdrawn || !pet.Withdrawn || leader.Withdrawn || member.HP != 80 || pet.HP != 70 || b.Defenders[0].HP != 10000 {
			t.Fatal("escape affected teammates or executed companion command", petActs, outcome)
		}
		if len(steps) < 2 || len(steps[0].Escapes) != 1 || steps[0].Escapes[0] != 8 || !bytes.Equal(steps[0].Packets[0], []byte{11, 1, 3, 3}) || !bytes.Equal(steps[0].Packets[1], []byte{11, 1, 4, 3}) {
			t.Fatal("owner/pet departure record", steps)
		}
		if b.Expected() != 1 || len(living(b.Attackers)) != 1 {
			t.Fatal("escaped fighters remained active")
		}
	}
}

func TestEscapeLeaderEndsBattleButOverkillAndDisconnectDoNot(t *testing.T) {
	b, leader, member, pet := escapeParty()
	leader.Level = 50
	b.Pending[leader.key()] = Action{Actor: leader, Kind: "flee"}
	if _, outcome := b.Round(lowest); outcome != Fled {
		t.Fatal("leader escape did not end battle")
	}
	for _, overkill := range []bool{false, true} {
		b, leader, member, pet = escapeParty()
		if overkill {
			leader.HP = 0
			leader.Overkilled = true
		} else {
			b.Leave(leader.ID)
		}
		b.Idle(member)
		b.Idle(pet)
		if _, outcome := b.Round(lowest); outcome == Fled {
			t.Fatal("leader departure counted as explicit escape")
		}
	}
}

func TestEscapeFailureNativeFallRecord(t *testing.T) {
	b, actor := setup(10000)
	actor.Level = 1
	b.Defenders[0].Level = 60
	beforeSP := actor.SP
	b.Pending[actor.key()] = Action{Actor: actor, Kind: "flee"}
	steps, outcome := b.Round(lowest)
	want := []byte{50, 1, 11, 0, 4, 2, 138, 234, 0, 1, 4, 2, 1, 0, 0}
	if outcome == Fled || actor.Withdrawn || actor.SP != beforeSP || len(steps) < 2 || !bytes.Equal(steps[0].Packets[1], want) || len(steps[0].SkillUses) != 0 {
		t.Fatal("failed flee missing native trip or consumed resources", steps, outcome)
	}
}

func TestEscapeKeepsKnockedOutTeammateForDefeatSettlement(t *testing.T) {
	b, leader, member, pet := escapeParty()
	leader.HP = 0
	b.Pending[member.key()] = Action{Actor: member, Kind: "flee"}
	b.Idle(pet)
	steps, outcome := b.Round(lowest)
	if outcome != Defeat || len(steps) != 1 || len(steps[0].Escapes) != 1 {
		t.Fatal("knocked-out teammate was treated as having escaped", outcome, steps)
	}
}
