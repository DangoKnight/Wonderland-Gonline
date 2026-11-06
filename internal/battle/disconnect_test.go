package battle

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestDisconnectWithdrawsOwnerAndPetWithoutChangingVitals(t *testing.T) {
	b, self := setup(10000)
	p := &Fighter{Kind: Pet, Side: Attacker, ID: 10501, Owner: self.ID, X: 3, Y: 2, HP: 43, MaxHP: 100, SP: 7, MaxSP: 30, Deaths: 1, Pet: &game.Pet{ID: 10501}}
	otherChar := hero().Clone()
	otherChar.ID = 8
	other := PlayerFighter(&otherChar, nil, 1)
	b.Attackers = append(b.Attackers, p, other)
	b.Pending[self.key()] = Action{Actor: self, Player: self.ID, Kind: "attack", Skill: 10001, TX: 2, TY: 2}
	b.Pending[p.key()] = Action{Actor: p, Player: self.ID, Kind: "attack", Skill: 10001, TX: 2, TY: 2}
	b.Idle(other)
	hp, sp := self.HP, self.SP
	b.Leave(self.ID)
	if !self.Withdrawn || !p.Withdrawn || !self.Absent || !p.Absent || len(b.Pending) != 1 || b.Expected() != 1 || !b.Ready() {
		t.Fatal("disconnect left commands/fighters active", b.Pending)
	}
	if self.HP != hp || self.SP != sp || p.HP != 43 || p.SP != 7 || p.Deaths != 1 {
		t.Fatal("withdrawal destroyed settlement state")
	}
	if at(b.Attackers, self.X, self.Y, false) != nil || at(b.Attackers, p.X, p.Y, false) != nil {
		t.Fatal("departed units still targetable")
	}
	if len(living(b.Attackers)) != 1 {
		t.Fatal("departed units counted as living")
	}
	if _, outcome := b.Round(lowest); outcome == Fled {
		t.Fatal("disconnect became successful escape")
	}
	if self.HP != hp || p.HP != 43 || p.Deaths != 1 {
		t.Fatal("later round damaged departed units")
	}
}

func TestDisconnectLastLivingOwnerLeavesDefeatForKnockedOutTeammate(t *testing.T) {
	b, self := setup(1000)
	otherChar := hero().Clone()
	otherChar.ID = 8
	other := PlayerFighter(&otherChar, nil, 1)
	other.HP = 0
	b.Attackers = append(b.Attackers, other)
	b.Leave(self.ID)
	if b.Forfeit() != Defeat {
		t.Fatal("knocked-out teammate left waiting for impossible commands")
	}
}
