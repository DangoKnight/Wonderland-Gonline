package battle

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

// lowest makes every roll its minimum.
var lowest = Rules{Next: func(lo, _ int) int { return lo }, Float: func() float64 { return 0 },
	Skills: map[uint16]assets.Skill{11001: {ID: 11001, Name: "Fire Ball", SP: 10, EffectLayer: 2, StatMultiplier: 2, PowerPerLevel: 5}}}

func hero() *game.Character {
	return &game.Character{ID: 7, Name: "Hero", Level: 1, Element: game.Fire, Body: 1, Base: game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}, HP: 100, MaxHP: 201, SP: 50, MaxSP: 121,
		Skills: []game.LearnedSkill{{ID: 11001, Grade: 2}}}
}

func setup(hp int) (*Battle, *Fighter) {
	self := PlayerFighter(hero(), nil, 0)
	return New([]*Fighter{self}, []Enemy{{Template: 10500, Name: "Slime", Level: 5, HP: hp, ClickID: 3}}), self
}

func TestIntroAndSubmit(t *testing.T) {
	b, self := setup(50)
	intro := b.Intro(self, 1)
	if !bytes.Equal(intro[0], []byte{20, 12}) || intro[2][0] != 11 || intro[2][1] != 250 || len(intro[2]) != 2+2+32 || !bytes.Equal(intro[len(intro)-1], []byte{52, 1}) {
		t.Fatal("intro", intro)
	}
	// Side, kind, template 10500, click 3, owner 0, grid (2,2).
	enemy := intro[4]
	if len(enemy) != 2+32 || enemy[2] != 2 || enemy[3] != 7 || enemy[4] != 0x04 || enemy[5] != 0x29 || enemy[8] != 3 || enemy[14] != 2 || enemy[15] != 2 {
		t.Fatal("enemy record", enemy)
	}
	if b.Submit(lowest, 99, 1, []byte{4, 2, 2, 2}) != nil {
		t.Fatal("another player's fighter accepted")
	}
	if ack := b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0xf9, 0x2a}); !bytes.Equal(ack, []byte{53, 5, 4, 2}) || !b.Ready() || b.Pending[4<<8|2].Kind != "attack" || b.Pending[4<<8|2].Skill != 11001 {
		t.Fatal("skill command", b.Pending)
	}
	if b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2}) != nil {
		t.Fatal("duplicate command accepted")
	}
	b2, _ := setup(50)
	b2.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0x10, 0x27}) // Unknown skill 10000.
	if a := b2.Pending[4<<8|2]; a.Kind != "defend" || a.Skill != defendSkill {
		t.Fatal("invalid skill must defend", a)
	}
}

func TestSubmitNativeTrailerAndUnsupportedRequests(t *testing.T) {
	// The native AC50:1 builder appends opaque compatibility bytes after the
	// skill. They must not affect ownership, skill selection or replay checks.
	b, _ := setup(50)
	data := []byte{4, 2, 2, 2, 0xf9, 0x2a, 0xdc, 0xcd, 2}
	if ack := b.Submit(lowest, 7, 1, data); !bytes.Equal(ack, []byte{53, 5, 4, 2}) {
		t.Fatalf("native extended attack: %x", ack)
	}
	if a := b.Pending[4<<8|2]; a.Kind != "attack" || a.Skill != 11001 {
		t.Fatal("trailer changed action", a)
	}
	if ack := b.Submit(lowest, 7, 1, data); ack != nil || len(b.Pending) != 1 {
		t.Fatal("duplicate native request accepted", ack, b.Pending)
	}
	for _, request := range []struct {
		sub  byte
		data []byte
	}{
		{2, []byte{4, 2, 2, 2, 0xf9, 0x2a, 0xdc, 0xcd, 1, 0}},
		{0, []byte{4, 2, 2, 2, 0xf9, 0x2a}},
		{255, []byte{4, 2, 2, 2, 0xf9, 0x2a}},
		{1, []byte{4, 2, 2}},
		{1, []byte{4, 2, 2, 2, 0xf9}},
	} {
		b, _ := setup(50)
		if ack := b.Submit(lowest, 7, request.sub, request.data); ack != nil || len(b.Pending) != 0 {
			t.Fatalf("unsupported/malformed request %d/%x consumed turn: %x", request.sub, request.data, ack)
		}
	}
}

func TestRoundDamageAndOutcomes(t *testing.T) {
	b, self := setup(1000)
	b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2})
	steps, outcome := b.Round(lowest)
	m := b.Defenders[0]
	// Hero (fire) ATK = round(1*2 + 5*2) = 12; slime DEF = round(5*1.2+3) = 9.
	// Basic damage = 12*2 - 9 + roll 1 = 16; slime ATK 12 hits for max(5, 14 - DEF 11) = 5.
	if outcome != Continue || m.HP != 1000-16 || self.HP != 95 || len(steps) != 2 {
		t.Fatal(outcome, m.HP, self.HP, len(steps))
	}
	hit := steps[0].Packets[1]
	if hit[0] != 50 || hit[1] != 1 || hit[2] != 0x11 || hit[15] != 0x19 || hit[16] != 16 || hit[20] != 1 {
		t.Fatalf("hit record %v", hit)
	}
	// A magic skill spends SP and uses MAT against MDF.
	b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0xf9, 0x2a})
	b.Round(lowest)
	if self.SP != 40 {
		t.Fatal("SP not spent", self.SP)
	}
	weak, _ := setup(1)
	weak.Submit(lowest, 7, 1, []byte{4, 2, 2, 2})
	if _, outcome := weak.Round(lowest); outcome != Victory {
		t.Fatal("victory", outcome)
	}
	if exp, gold := weak.Rewards(); exp != 75 || gold != 40 {
		t.Fatal(exp, gold)
	}
	lost, me := setup(1000)
	me.HP = 1
	lost.Submit(lowest, 7, 4, []byte{4, 2, 0, 0})
	if _, outcome := lost.Round(lowest); outcome != Defeat {
		t.Fatal("defeat", outcome)
	}
	run, _ := setup(1000)
	run.Submit(lowest, 7, 5, []byte{4, 2, 0, 0})
	if _, outcome := run.Round(lowest); outcome != Fled {
		t.Fatal("flee", outcome)
	}
}

func TestTimeoutDefends(t *testing.T) {
	b, _ := setup(100)
	if b.Ready() {
		t.Fatal("ready without commands")
	}
	b.Timeout()
	if !b.Ready() || b.Pending[4<<8|2].Kind != "defend" {
		t.Fatal(b.Pending)
	}
}

func TestDropsAndElements(t *testing.T) {
	table := []assets.Drop{{Item: 1, Min: 1, Max: 1, Rate: 50}, {Item: 2, Min: 2, Max: 4, Rate: 10}}
	known := func(uint16) bool { return true }
	if got := lowest.RollDrops(table, [5]uint16{2}, known); len(got) != 1 || got[0] != (Drop{2, 2}) {
		t.Fatal("only native drops roll", got)
	}
	never := Rules{Float: func() float64 { return 0.99 }}
	if got := never.RollDrops(table, [5]uint16{1, 2}, known); len(got) != 0 {
		t.Fatal("rate ignored", got)
	}
	if Elemental(3, 4) != 1.5 || Elemental(4, 3) != 0.6 || Elemental(2, 3) != 1.7 || Elemental(0, 3) != 1 {
		t.Fatal("elements")
	}
}

func TestPetFighterAndCapture(t *testing.T) {
	owner := hero()
	self := PlayerFighter(owner, nil, 0)
	pet := game.NewPet(14156, "Xaolan", 1, game.PetTemplate{Stats: game.Attributes{Strength: 5, Constitution: 5, Intelligence: 5, Wisdom: 5, Agility: 5}}, nil)
	pf := PetFighter(pet, owner.ID, nil, 0)
	b := New([]*Fighter{self, pf}, []Enemy{{Template: 17100, Name: "Boar", Level: 1, HP: 500}})
	if b.Expected() != 2 || pf.X != 3 || pf.Y != 2 || pf.Owner != 7 {
		t.Fatal("pet fighter", b.Expected(), pf)
	}
	if intro := b.Intro(self, 1); intro[4][2] != 5 || intro[4][3] != byte(Pet) {
		t.Fatal("pet record", intro[4])
	}
	// The pet's command comes from its owner; the round waits for both.
	b.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0x18, 0x27}) // Capture (10008) by the player.
	if b.Ready() {
		t.Fatal("round ran without the pet's command")
	}
	b.Submit(lowest, 7, 4, []byte{3, 2, 0, 0})
	_, outcome := b.Round(lowest)
	if outcome != Victory || len(b.Captures) != 1 || b.Captures[0].Template != 17100 || !b.Defenders[0].Captured {
		t.Fatal("capture", outcome, b.Captures)
	}
	if exp, _ := b.Rewards(); exp != 0 {
		t.Fatal("captured monsters give no EXP", exp)
	}
	// A duplicate in the roster always fails, whatever the roll.
	dup := New([]*Fighter{PlayerFighter(owner, nil, 0)}, []Enemy{{Template: 17100, Level: 1, HP: 500}})
	dup.Roster[7] = []uint32{17100}
	dup.Submit(lowest, 7, 1, []byte{4, 2, 2, 2, 0x18, 0x27})
	steps, _ := dup.Round(lowest)
	if len(dup.Captures) != 0 || !bytes.Contains(steps[1].Packets[0], []byte("already have")) {
		t.Fatal("duplicate capture", dup.Captures)
	}
}
