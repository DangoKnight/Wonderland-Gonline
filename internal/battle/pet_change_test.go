package battle

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
)

func TestTimeoutNoDefenseOrSkillUse(t *testing.T) {
	var damage [2]int
	for i, defend := range []bool{false, true} {
		b, self := setup(10000)
		self.HP, self.MaxHP = 1000, 1000
		b.Defenders[0].Atk = 100
		if defend {
			b.Submit(lowest, self.ID, 4, []byte{4, 2, 4, 2, 117, 234})
		}
		b.Timeout()
		steps, _ := b.Round(lowest)
		damage[i] = 1000 - self.HP
		if self.SP != 50 || b.Defenders[0].HP != 10000 || len(steps) != 1 || len(steps[0].SkillUses) != 0 {
			t.Fatal("timeout performed an action", self.SP, b.Defenders[0].HP, steps)
		}
		for _, p := range steps[0].Packets {
			if len(p) > 13 && p[0] == 50 && p[1] == 1 && (p[13] != 0) != defend {
				t.Fatal("incorrect native defended flag", p)
			}
		}
	}
	if damage[0] <= damage[1] || damage[1] == 0 {
		t.Fatal("idle gained defense", damage)
	}
}

func TestBattlePetReplacementUnavailableUntilNextRound(t *testing.T) {
	for _, hp := range []int{1000, 1} {
		b, owner := setup(10000)
		owner.HP, owner.MaxHP = hp, 1000
		b.Defenders[0].Atk = 100
		next := &Fighter{Kind: Pet, ID: 102, Owner: owner.ID, HP: 1000, MaxHP: 1000, SP: 8, MaxSP: 30}
		b.ReplacePet(owner, nil, next)
		// Put the pet first: deterministic enemy selection would hit it if the
		// joining flag were ignored. Both direct and area selection must reject it.
		b.Attackers = []*Fighter{next, owner}
		if at(b.Attackers, next.X, next.Y, true) != nil || at(b.Attackers, next.X, next.Y, false) != nil || len(formationTargets(b.Attackers, next, assets.SkillTargeting{All: true}, true)) != 1 {
			t.Fatal("joining pet can be targeted")
		}
		_, outcome := b.Round(lowest)
		if next.HP != 1000 || next.SP != 8 || next.Joining || !next.CanAct() || owner.HP == hp || outcome != Continue {
			t.Fatal("switch round targeted pet, failed to unlock it, or ended with a living incoming pet", hp, next, owner.HP, outcome)
		}
		b.Timeout()
		b.Round(lowest)
		if next.HP == 1000 {
			t.Fatal("replacement stayed untargetable in the next round")
		}
	}
}

func TestBattlePetReplacementLegacyCapture(t *testing.T) {
	owner := &Fighter{ID: 117671, Kind: Player, Side: Attacker, X: 4, Y: 2, HP: 2000}
	old := &Fighter{ID: 14081, Kind: Pet, Side: Attacker, X: 3, Y: 2, HP: 233}
	b := &Battle{Attackers: []*Fighter{owner, old}, Pending: map[int]Action{}}
	next := &Fighter{ID: 14162, Kind: Pet, ClickID: 2, Owner: owner.ID, MaxHP: 247, HP: 247, MaxSP: 129, SP: 129, Level: 16, Element: 3}
	steps := b.ReplacePet(owner, old, next)
	// Actual successful legacy-server trace, 2026-11-02; independent raw bytes.
	want := []byte{11, 5, 5, 4, 82, 55, 0, 0, 2, 0, 167, 203, 1, 0, 3, 2, 247, 0, 0, 0, 129, 0, 247, 0, 0, 0, 129, 0, 16, 3, 0, 0, 0, 0}
	if len(steps) != 2 || !bytes.Equal(steps[0].Packets[0], []byte{11, 1, 3, 2}) || !bytes.Equal(steps[1].Packets[0], want) {
		t.Fatal("replacement differs from the reference trace", steps)
	}
}

func TestBattlePetReplacementConsumesCommandsOnBothSides(t *testing.T) {
	for _, side := range []Side{Attacker, Defender} {
		b, owner := setup(10000)
		if side == Defender {
			b.Attackers, b.Defenders = b.Defenders, b.Attackers
			Place(owner, side, 0)
		}
		old := &Fighter{Kind: Pet, Side: side, ID: 101, Owner: owner.ID, HP: 60, MaxHP: 100, SP: 12, X: owner.X - 1, Y: owner.Y}
		if side == Defender {
			old.X = owner.X + 1
		}
		if side == Attacker {
			b.Attackers = append(b.Attackers, old)
		} else {
			b.Defenders = append(b.Defenders, old)
		}
		b.Pending[old.key()] = Action{Actor: old, Kind: "attack", Skill: 10001}
		next := &Fighter{Kind: Pet, ID: 102, ClickID: 2, Owner: owner.ID, HP: 37, MaxHP: 100, SP: 8, MaxSP: 30, Level: 1, Element: 3}
		steps := b.ReplacePet(owner, old, next)
		if next.Side != side || next.X != old.X || next.Y != old.Y || !b.Ready() || b.Pending[owner.key()].Kind != ActionIdle || b.Pending[next.key()].Kind != ActionIdle {
			t.Fatal("replacement commands/formation", side, b.Pending, next)
		}
		if len(steps) != 2 || !bytes.Equal(steps[0].Packets[0], []byte{11, 1, old.X, old.Y}) || steps[0].Delay != 0 || steps[1].Delay != 350*time.Millisecond || len(steps[1].Packets) != 1 || !next.Joining || next.CanAct() {
			t.Fatal("native exit/join order", steps)
		}
		if next.HP != 37 || next.SP != 8 || len(b.Pending) != 2 {
			t.Fatal("replacement healed or retained outgoing action")
		}
		// Independent native AC11:5 record: wait-next-turn/kind, ID/click/owner,
		// formation, max/current HP/SP, level/element and reserved fields.
		want := []byte{11, 5, 5, 4, 102, 0, 0, 0, 2, 0, 7, 0, 0, 0, 3, 2, 100, 0, 0, 0, 30, 0, 37, 0, 0, 0, 8, 0, 1, 3, 0, 0, 0, 0}
		if side == Defender {
			want = []byte{11, 5, 5, 4, 102, 0, 0, 0, 2, 0, 7, 0, 0, 0, 2, 2, 100, 0, 0, 0, 30, 0, 37, 0, 0, 0, 8, 0, 1, 3, 0, 0, 0, 0}
		}
		if !bytes.Equal(steps[1].Packets[0], want) {
			t.Fatal("native pet participant changed", steps[1].Packets[0])
		}
		b.Pending = map[int]Action{}
		b.ReplacePet(owner, next, nil)
		if !b.Ready() || len(b.Pending) != 1 || b.Expected() != 1 {
			t.Fatal("rest still waits for the pet")
		}
	}
}
