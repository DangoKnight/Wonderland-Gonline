package battle

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func pvpFixture() (*Battle, *Fighter, *Fighter) {
	a := PlayerFighter(hero(), nil, 0)
	c := *hero()
	c.ID = 8
	d := PlayerFighter(&c, nil, 0)
	Place(d, Defender, 0)
	return NewPvP([]*Fighter{a}, []*Fighter{d}), a, d
}

func TestPvPFormationCommandsAndRewards(t *testing.T) {
	b, a, d := pvpFixture()
	intro := b.Intro(d, 1)
	// Independently authored AC11:250 defending self record, including native grid.
	want := []byte{11, 250, 1, 0, 2, 2, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2}
	if !bytes.Equal(intro[2][:18], want) {
		t.Fatal("defending formation", intro[2])
	}
	count := 0
	for _, p := range intro {
		if len(p) > 1 && p[0] == 11 && p[1] == 5 {
			count++
			if p[4] == 8 {
				t.Fatal("duplicated self")
			}
		}
	}
	if count != 1 || b.Expected() != 2 {
		t.Fatal(count, b.Expected())
	}
	b.Submit(lowest, a.ID, 1, []byte{4, 2, 1, 2})
	if b.Ready() {
		t.Fatal("did not wait for defending player")
	}
	if b.Submit(lowest, a.ID, 1, []byte{1, 2, 4, 2}) != nil {
		t.Fatal("opponent ownership bypass")
	}
	b.Submit(lowest, d.ID, 1, []byte{1, 2, 4, 2})
	if !b.Ready() {
		t.Fatal("defender command missing")
	}
	ahp, dhp := a.HP, d.HP
	steps, outcome := b.Round(lowest)
	if outcome != Continue || a.HP >= ahp || d.HP >= dhp || len(steps) != 2 {
		t.Fatal("symmetric attacks", a.HP, d.HP, steps, outcome)
	}
	if exp, gold := b.Rewards(); exp != 0 || gold != 0 {
		t.Fatal("PvP farming", exp, gold)
	}
	if len(b.NextRound(d.ID)) != 2 {
		t.Fatal("defender menu missing")
	}
	if _, ok := lowest.capture(b, Action{Actor: a, Player: a.ID, TX: d.X, TY: d.Y}); ok {
		t.Fatal("captured another player")
	}
}

func TestPvPDefenderVictoryTimeoutAndForfeit(t *testing.T) {
	b, a, d := pvpFixture()
	a.HP = 1
	d.Atk = 1000
	d.Spd = 1000
	b.Submit(lowest, d.ID, 1, []byte{1, 2, 4, 2})
	b.Timeout()
	if len(b.Pending) != 2 || !b.Ready() {
		t.Fatal("timeout did not defend both sides")
	}
	_, outcome := b.Round(lowest)
	if outcome != Defeat {
		t.Fatal("defender victory attributed to attacker", outcome)
	}
	b, a, d = pvpFixture()
	b.Leave(d.ID)
	if b.Expected() != 1 || !b.Present() || b.Forfeit() != Victory {
		t.Fatal("defender disconnect")
	}
	b.Leave(a.ID)
	if b.Present() {
		t.Fatal("absent players remain present")
	}
}

func TestPvPDefenderHealingAndPets(t *testing.T) {
	b, a, d := pvpFixture()
	d.Char.Skills = append(d.Char.Skills, game.LearnedSkill{ID: 50001, Grade: 1})
	r := lowest
	r.Skills = map[uint16]assets.Skill{50001: {ID: 50001, Name: "Heal", SP: 1}}
	d.HP = 1
	a.HP = 1
	r.support(b, Action{Actor: d, Skill: 50001, TX: a.X, TY: a.Y})
	if d.HP <= 1 || a.HP != 1 {
		t.Fatal("healed enemy instead of defending ally")
	}
	p := PetFighter(game.Pet{ID: 10500, HP: 10, MaxHP: 10}, d.ID, nil, 1)
	Place(p, Defender, 1)
	if p.X != 2 || p.Y != 3 {
		t.Fatal("defender pet grid", p.X, p.Y)
	}
	b.Defenders = append(b.Defenders, p)
	if b.Expected() != 3 {
		t.Fatal("defending pet not expected")
	}
	found := false
	for _, packet := range b.Intro(d, 1) {
		if len(packet) > 15 && packet[0] == 11 && packet[1] == 5 && packet[3] == 4 {
			found = packet[2] == 5 && packet[14] == 2 && packet[15] == 3
		}
	}
	if !found {
		t.Fatal("friendly pet record")
	}
}

func TestMonsterNativeSkillAIAndFallback(t *testing.T) {
	b, a := setup(1000)
	m := b.Defenders[0]
	r := lowest
	m.Skills = [3]uint16{11001, 11001, 65535}
	sp, hp := m.SP, a.HP
	steps := r.monster(b, m, map[int]bool{})
	if len(steps) == 0 || m.SP != sp-10 || a.HP >= hp {
		t.Fatal("native magical cast", m.SP, a.HP)
	}
	if got := steps[0].Packets[2]; !bytes.Equal(got[:8], []byte{50, 1, 17, 0, 2, 2, 249, 42}) {
		t.Fatal("skill animation", got)
	}
	m.SP = 0
	hp = a.HP
	steps = r.monster(b, m, map[int]bool{})
	if len(steps) == 0 || m.SP != 0 || a.HP >= hp || steps[0].Packets[1][6] != 17 || steps[0].Packets[1][7] != 39 {
		t.Fatal("basic fallback", steps)
	}
	m.HP = 0
	if len(r.monster(b, m, map[int]bool{})) != 0 {
		t.Fatal("dead monster attacked")
	}
}

func TestMonsterGenericBuffHealAndControl(t *testing.T) {
	b, a := setup(1000)
	m := b.Defenders[0]
	r := lowest
	r.Skills = map[uint16]assets.Skill{
		50001: {ID: 50001, SP: 7, Effects: []assets.SkillEffect{{Target: assets.EffectAlly, Rounds: 2, Modifiers: []assets.StatModifier{{Stat: assets.EffectATK, Flat: 10}}}}},
		50002: {ID: 50002, Name: "Heal", SP: 5},
		50003: {ID: 50003, SP: 3, Effects: []assets.SkillEffect{{Target: assets.EffectEnemy, Rounds: 2, BlockActions: true}}},
	}
	m.Skills = [3]uint16{50001}
	before := m.SP
	r.monster(b, m, map[int]bool{})
	if !m.HasEffect(50001) || a.HasEffect(50001) || m.SP != before-7 {
		t.Fatal("monster buff target/cost")
	}
	m.Skills = [3]uint16{50002}
	m.HP = 1
	r.monster(b, m, map[int]bool{})
	if m.HP <= 1 {
		t.Fatal("monster did not heal ally")
	}
	m.Skills = [3]uint16{50003}
	r.monster(b, m, map[int]bool{})
	if a.CanAct() || !m.CanAct() {
		t.Fatal("control target")
	}
	m.ApplyEffect(50004, 0, assets.SkillEffect{Target: assets.EffectSelf, Rounds: 2, BlockActions: true})
	before = m.SP
	if len(r.monster(b, m, map[int]bool{})) != 0 || m.SP != before {
		t.Fatal("blocked monster acted")
	}
}
