package battle

import (
	"bytes"
	"math"
	"testing"
)

func TestOverkillStrictRoundDamageThreshold(t *testing.T) {
	for _, tc := range []struct {
		name        string
		maximum, hp int
		hits        []int
		want        bool
	}{
		{"ordinary lethal", 100, 100, []int{100}, false},
		{"exactly150percent", 100, 100, []int{150}, false},
		{"above150percent", 100, 100, []int{151}, true},
		{"low remaining HP", 100, 1, []int{1}, false},
		{"cumulative", 100, 100, []int{80, 80}, true},
		{"odd maximum below", 101, 101, []int{151}, false},
		{"odd maximum above", 101, 101, []int{152}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &Fighter{MaxHP: tc.maximum, HP: tc.hp}
			for _, hit := range tc.hits {
				f.hurt(hit)
			}
			if f.Overkilled != tc.want {
				t.Fatalf("overkill %v, damage %d", f.Overkilled, f.RoundDamage)
			}
			if f.Deaths != 1 || f.HP != 0 {
				t.Fatal("knockout changed", f.HP, f.Deaths)
			}
		})
	}
}

func TestOverkillRoundResetAndHealing(t *testing.T) {
	b, self := setup(1000)
	self.RoundDamage = 1000
	b.Defenders[0].RoundDamage = 1000
	b.Submit(lowest, self.ID, 1, []byte{4, 2, 2, 2})
	b.Round(lowest)
	if self.RoundDamage != 3 || b.Defenders[0].RoundDamage != 10 || self.Overkilled || b.Defenders[0].Overkilled {
		t.Fatal("damage carried between rounds")
	}
	f := &Fighter{HP: 100, MaxHP: 100}
	f.hurt(80)
	f.HP = 100
	f.hurt(80)
	if !f.Overkilled || f.HP != 0 {
		t.Fatal("healing erased received damage")
	}
}

func TestOverkillComboAndPacket(t *testing.T) {
	for _, maximum := range []int{23, 24} {
		b, r, actions := comboFixture(150, 60, 30)
		f := b.Defenders[0]
		f.MaxHP, f.HP = maximum, maximum
		steps, _ := r.attack(b, b.order(r, actions)[0], map[int]bool{})
		if maximum == 24 {
			if f.Overkilled || len(steps) != 1 {
				t.Fatal("exact threshold vanished")
			}
		} else {
			if !f.Overkilled || len(steps) != 2 || !bytes.Equal(steps[1].Packets[0], []byte{53, 3, 2, 2}) || steps[1].Delay != OverkillFlightDuration || len(steps[1].Overkills) != 1 {
				t.Fatal("native overkill step", steps)
			}
			if len(b.appendOverkill(nil, f)) != 0 {
				t.Fatal("duplicate overkill")
			}
		}
	}
}

func TestOverkillWithdrawsOwnedPetAndCannotRevive(t *testing.T) {
	b, self := setup(1000)
	self.MaxHP, self.HP = 100, 100
	pet := &Fighter{Kind: Pet, Side: Attacker, ID: 14162, Owner: self.ID, X: 3, Y: 2, HP: 100, MaxHP: 100}
	other := &Fighter{Kind: Player, Side: Attacker, ID: 8, X: 4, Y: 3, HP: 100, MaxHP: 100}
	b.Attackers = append(b.Attackers, pet, other)
	self.hurt(151)
	b.appendOverkill(nil, self)
	if !pet.Withdrawn || pet.HP != 100 || pet.Deaths != 0 || b.Expected() != 1 {
		t.Fatal("owned pet not withdrawn intact")
	}
	if at(b.Attackers, self.X, self.Y, false) != nil || at(b.Attackers, pet.X, pet.Y, false) != nil {
		t.Fatal("removed fighter can be revived/selected")
	}
}

func TestOverkillAccumulatesWithoutOverflow(t *testing.T) {
	f := &Fighter{HP: math.MaxInt, MaxHP: math.MaxInt}
	f.hurt(math.MaxInt)
	f.HP = math.MaxInt
	f.hurt(math.MaxInt)
	if !f.Overkilled {
		t.Fatal("received damage overflowed")
	}
	// HP damage retains its existing signed clamp; the overkill counter uses
	// the full sum of the separately encoded combo hits.
	f = &Fighter{HP: math.MaxInt32 + 1, MaxHP: math.MaxInt32 + 1}
	f.hurtWithRoundDamage(math.MaxInt32, 2*uint64(math.MaxInt32))
	if !f.Overkilled || f.HP != 0 {
		t.Fatal("combo clamp hid overkill")
	}
}

func TestOverkillAreaTargetsFlyTogether(t *testing.T) {
	b, _ := setup(1000)
	first := &Fighter{Kind: Monster, X: 2, Y: 2, HP: 100, MaxHP: 100}
	second := &Fighter{Kind: Monster, X: 2, Y: 3, HP: 100, MaxHP: 100}
	first.hurt(151)
	second.hurt(151)
	steps := b.appendOverkills(nil, first, second)
	if len(steps) != 1 || steps[0].Delay != OverkillFlightDuration || len(steps[0].Overkills) != 2 || !bytes.Equal(steps[0].Packets[0], []byte{53, 3, 2, 2}) || !bytes.Equal(steps[0].Packets[1], []byte{53, 3, 2, 3}) {
		t.Fatal("area removals were serialized", steps)
	}
}
