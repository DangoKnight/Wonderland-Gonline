package game

import (
	"bytes"
	"errors"
	"math"
	"testing"

	"wonderland-go/internal/protocol"
)

func TestPotentialBonusAndDerivedStats(t *testing.T) {
	// Independent native table: cumulative points per attribute, not the level.
	for level, want := range []uint16{0, 1, 2, 3, 4, 6, 9, 13, 18, 24, 31, 39, 48} {
		c := Character{Level: 10, Base: Attributes{Strength: 10, Constitution: 20, Intelligence: 30, Wisdom: 40, Agility: 50}, Potential: uint16(level)}
		p := Pet{Level: 10, Base: c.Base, Potential: c.Potential}
		a := c.Attributes()
		expected := Attributes{10 + want, 20 + want, 30 + want, 40 + want, 50 + want}
		if a != expected || p.Attributes() != expected || PotentialBonus(uint16(level)) != want {
			t.Fatal(level, a, p.Attributes(), want)
		}
		base := c
		base.Potential = 0
		basePet := p
		basePet.Potential = 0
		if c.Combat(nil).ATK != base.Combat(nil).ATK+int32(want)*2 || p.Combat(nil).MAT != basePet.Combat(nil).MAT+int32(want)*2 {
			t.Fatal("potential missing from combat", level)
		}
		if level > 0 && (c.Combat(nil).MaxHP <= base.Combat(nil).MaxHP || c.Combat(nil).MaxSP <= base.Combat(nil).MaxSP || p.CalculatedMaxHP() <= basePet.CalculatedMaxHP() || p.CalculatedMaxSP() <= basePet.CalculatedMaxSP()) {
			t.Fatal("potential missing from vitals", level)
		}
	}
	p := Pet{Potential: math.MaxUint16, Base: Attributes{math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16}}
	if p.Attributes() != p.Base || PotentialBonus(p.Potential) != 48 {
		t.Fatal("potential overflow")
	}
}

func TestPotentialPillRules(t *testing.T) {
	for level := uint16(0); level < 12; level++ {
		next, err := NextPotential(34350, level, 99)
		if err != nil || next != level+1 {
			t.Fatal(level, next, err)
		}
	}
	for _, tc := range []struct {
		item, level uint16
		err         error
	}{

		{34289, 9, ErrPotentialGoldenMinimum},
		{34350, 12, ErrPotentialMaximum}, {34350, 300, ErrPotentialMaximum}, {1, 0, ErrPotentialPill},
	} {
		next, err := NextPotential(tc.item, tc.level, 0)
		if !errors.Is(err, tc.err) || next != tc.level {
			t.Fatal(tc, next, err)
		}
	}
}

func TestPotentialPetPacketsUseEffectiveAttributes(t *testing.T) {
	p := Pet{ID: 12032, Slot: 3, Level: 10, Potential: 12, Base: Attributes{1, 2, 3, 4, 5}}
	list := p.ListRecord(nil, 2, "", PetTemplate{})
	if !bytes.Equal(list[14:24], []byte{51, 0, 49, 0, 50, 0, 53, 0, 52, 0}) {
		t.Fatal("roster stats", list)
	}
	hotel := p.HotelRecord(nil, PetTemplate{})
	if !bytes.Equal(hotel[14:24], list[14:24]) {
		t.Fatal("hotel stats", hotel)
	}
	recruit := p.RecruitPacket(10001, PetTemplate{})
	if !bytes.Equal(recruit[11:21], []byte{49, 0, 50, 0, 51, 0, 52, 0, 53, 0}) {
		t.Fatal("recruit stats", recruit)
	}
	wanted := protocol.Builder{8, 2, 4, 2, 0, 28, 1}.U32(49).U32(0)
	found := false
	for _, packet := range p.ProgressionPackets(2, nil) {
		if bytes.Equal(packet, wanted) {
			found = true
		}
	}
	if !found {
		t.Fatal("progression missing potential bonus")
	}
	if p.Base != (Attributes{1, 2, 3, 4, 5}) {
		t.Fatal("base points changed")
	}
}

func TestPotentialPillProbabilityBoundariesAndFailure(t *testing.T) {
	for level, chance := range []int{100, 100, 100, 70, 60, 55, 55, 50, 35, 20, 18, 15} {
		for roll := 0; roll < 100; roll++ {
			next, err := NextPotential(34269, uint16(level), roll)
			want := uint16(level + 1)
			if roll >= chance {
				want = uint16(level - 1)
			}
			if err != nil || next != want {
				t.Fatal(level, roll, chance, next, err)
			}
			if level >= 10 {
				next, err = NextPotential(34289, uint16(level), roll)
				if roll >= chance {
					want = uint16(level)
				}
				if err != nil || next != want {
					t.Fatal("golden", level, roll, next, err)
				}
			}
		}
	}
	for _, roll := range []int{-1, 100} {
		if _, err := NextPotential(34269, 4, roll); !errors.Is(err, ErrPotentialRoll) {
			t.Fatal("invalid random roll", err)
		}
	}
}

func TestPotentialFailureForgetsOnlyUnqualifiedSkills(t *testing.T) {
	c := Character{Element: Water, Potential: 4, Base: Attributes{Strength: 9}, Skills: []LearnedSkill{{ID: 11001, Grade: 5, EXP: 17}, {ID: 15091, Grade: 1}, {ID: 102, Grade: 5}}}
	if c.ForgetUnqualifiedSkills() {
		t.Fatal("forgot qualified Icicle Attack")
	}
	c.Potential = 3
	if !c.ForgetUnqualifiedSkills() || len(c.Skills) != 2 || c.Skills[0].ID != 15091 || c.Skills[1].ID != 102 {
		t.Fatal("did not forget only stat-dependent skill", c.Skills)
	}
	c.Potential = 4
	c.UnlockQualifiedSkills(false, func(id uint16) bool { return id == 11001 })
	if len(c.Skills) != 3 || c.Skills[2].ID != 11001 || c.Skills[2].Grade != 1 || c.Skills[2].EXP != 0 {
		t.Fatal("skill not relearned", c.Skills)
	}
}

func TestPetNativePotentialAndJobFooter(t *testing.T) {
	for level := uint16(0); level <= 12; level++ {
		p := Pet{ID: 12032, Level: 10, Reborn: true, Potential: level, Job: 6}
		// aLogin FUN_0040b1e0/00409820 reads rebirth, potential, then job.
		// Distinct values detect the previous job-as-potential shift.
		want := []byte{0, 0, 1, byte(level), 6, 0, 0, 0, 0, 0, 0}
		list := p.ListRecord(nil, 2, "Robinson", PetTemplate{})
		if !bytes.Equal(list[len(list)-11:], want) {
			t.Fatal("native roster footer", level, list[len(list)-11:])
		}
		recruit := p.RecruitPacket(10001, PetTemplate{})
		if !bytes.Equal(recruit[len(recruit)-10:], want[1:]) {
			t.Fatal("native recruitment footer", level, recruit[len(recruit)-10:])
		}
	}
}
