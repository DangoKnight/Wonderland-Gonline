package battle

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestManaRestorationNativeClassificationAreaAndCost(t *testing.T) {
	b, actor := setup(1000)
	actor.Atk, actor.Matk, actor.Level = 10, 100, 5
	actor.SP, actor.MaxSP = 50, 1000
	actor.Char.Skills = []game.LearnedSkill{{ID: 15075, Grade: 10}}
	ally := &Fighter{Side: Attacker, Kind: Pet, X: 3, Y: 2, HP: 20, MaxHP: 100, SP: 900, MaxSP: 1000}
	dead := &Fighter{Side: Attacker, X: 4, Y: 1, HP: 0, SP: 0, MaxSP: 1000}
	b.Attackers = append(b.Attackers, ally, dead)
	r := lowest
	r.Skills = map[uint16]assets.Skill{15075: {ID: 15075, Name: "Magic Recovery", EffectLayer: 6, SP: 10, NativePattern109: 7}}
	if ack := b.Submit(r, 7, 1, []byte{4, 2, 4, 2, 227, 58}); ack == nil || b.Pending[actor.key()].Kind != "heal" {
		t.Fatal("mana was submitted as an attack")
	}
	step := r.support(b, b.Pending[actor.key()])
	if actor.SP != 240 || ally.SP != 1000 || ally.HP != 20 || dead.SP != 0 || len(step.SkillUses) != 1 || step.SkillUses[0].SkillEXP != 40 {
		t.Fatal("mana restoration/cost", actor.SP, ally, step.SkillUses)
	}
	// Two recipients; actor +200 SP and pet +100 SP, both recovery mode 3.
	want := []byte{50, 1, 28, 0, 4, 2, 227, 58, 0, 2, 4, 2, 1, 0, 1, 26, 200, 0, 0, 0, 3, 3, 2, 1, 0, 1, 26, 100, 0, 0, 0, 3}
	if !bytes.Equal(step.Packets[2], want) {
		t.Fatal("mana native result", step.Packets[2])
	}
	if isHeal(r.Skills[15075]) {
		t.Fatal("mana classified as HP healing")
	}
}

func TestSelectiveCleansingNativeCodes(t *testing.T) {
	for _, tc := range []struct {
		cure    byte
		removed []byte
	}{
		{1, []byte{1, 177}}, {6, []byte{6}}, {171, []byte{171}}, {173, []byte{173}}, {7, []byte{7, 8}}, {4, []byte{3, 4, 178, 7, 8}}, {80, []byte{1, 177, 6, 171, 173, 7, 8, 3, 4, 178, 52}},
	} {
		b, actor := setup(1000)
		actor.Char.Skills = []game.LearnedSkill{{ID: 15042, Grade: 1}}
		ally := &Fighter{Side: Attacker, Kind: Pet, X: 3, Y: 2, HP: 20, MaxHP: 100, SP: 7, MaxSP: 100}
		b.Attackers = append(b.Attackers, ally)
		r := lowest
		r.Skills = map[uint16]assets.Skill{15042: {ID: 15042, Name: "translated cure", EffectLayer: 5, NativeEffectCode52: tc.cure, SP: 10}}
		for i, code := range []byte{1, 177, 6, 171, 173, 7, 8, 3, 4, 178, 52} {
			id := uint16(50000 + i)
			r.Skills[id] = assets.Skill{ID: id, NativeEffectCode52: code}
			ally.Effects = append(ally.Effects, ActiveEffect{Source: id, Turns: 3, Definition: assets.SkillEffect{Target: assets.EffectEnemy, BlockActions: true}})
		}
		step := r.support(b, Action{Actor: actor, Player: 7, Kind: "heal", Skill: 15042, TX: 3, TY: 2})
		if len(ally.Effects) != 11-len(tc.removed) || actor.SP != 40 || ally.HP != 20 || ally.SP != 7 || len(step.SkillUses) != 1 {
			t.Fatal("selective cleanse", tc, ally.Effects)
		}
		for _, effect := range ally.Effects {
			for _, code := range tc.removed {
				if r.Skills[effect.Source].NativeEffectCode52 == code {
					t.Fatal("uncleansed status", tc, code)
				}
			}
		}
		if len(step.Packets) != 3 || len(step.Packets[2]) != 21 || step.Packets[2][15] != 0 {
			t.Fatal("cleanse animation", step.Packets)
		}
	}
}

func TestSupportMonsterAIChoosesManaAndCleansing(t *testing.T) {
	b, _ := setup(1000)
	m := b.Defenders[0]
	m.Skills = [3]uint16{15075}
	m.SP, m.MaxSP = 10, 100
	r := lowest
	r.Skills = map[uint16]assets.Skill{15075: {ID: 15075, EffectLayer: 6}}
	a, ok := r.monsterSkill(b, m)
	if !ok || a.Kind != "heal" || a.TX != m.X {
		t.Fatal("monster mana target", a, ok)
	}
	m.SP = 100
	if _, ok := r.monsterSkill(b, m); ok {
		t.Fatal("monster wasted mana at full SP")
	}
	m.Skills = [3]uint16{12043}
	r.Skills[12043] = assets.Skill{ID: 12043, EffectLayer: 5, NativeEffectCode52: 6}
	r.Skills[12003] = assets.Skill{ID: 12003, NativeEffectCode52: 6}
	m.Effects = []ActiveEffect{{Source: 12003, Turns: 2}}
	a, ok = r.monsterSkill(b, m)
	if !ok || a.Kind != "heal" {
		t.Fatal("monster cleanse target", a, ok)
	}
}

func TestSupportSkillsPetAndDefendingSideAllegiance(t *testing.T) {
	for _, side := range []Side{Attacker, Defender} {
		owner := hero()
		owner.Pets = []game.Pet{{ID: 14162, Level: 5, HP: 100, MaxHP: 100, SP: 50, MaxSP: 100, Skills: []game.PetSkill{{ID: 15075, Grade: 1}, {ID: 12043, Grade: 1}}}}
		actor := PetFighter(owner.Pets[0], owner.ID, nil, 0)
		Place(actor, side, 0)
		ally := PlayerFighter(owner, nil, 0)
		Place(ally, side, 0)
		ally.SP = 0
		enemy := &Fighter{Side: Attacker, Kind: Player, X: 4, Y: 3, HP: 100, SP: 0, MaxSP: 100}
		b := NewPvP([]*Fighter{actor, ally}, []*Fighter{enemy})
		if side == Defender {
			enemy.Side = Attacker
			b.Attackers, b.Defenders = []*Fighter{enemy}, []*Fighter{actor, ally}
		} else {
			enemy.Side = Defender
		}
		r := lowest
		r.Skills = map[uint16]assets.Skill{15075: {ID: 15075, EffectLayer: 6, SP: 10, NativePattern109: 7}, 12043: {ID: 12043, EffectLayer: 5, NativeEffectCode52: 6}, 12003: {ID: 12003, NativeEffectCode52: 6}}
		before := actor.SP
		step := r.support(b, Action{Actor: actor, Player: owner.ID, Kind: "heal", Skill: 15075, TX: ally.X, TY: ally.Y})
		if ally.SP == 0 || enemy.SP != 0 || len(step.SkillUses) != 1 || actor.SP < before-10 {
			t.Fatal("pet mana allegiance", side, ally.SP, enemy.SP)
		}
		ally.Effects = []ActiveEffect{{Source: 12003, Turns: 3, Definition: assets.SkillEffect{BlockActions: true}}}
		enemy.Effects = append([]ActiveEffect(nil), ally.Effects...)
		r.support(b, Action{Actor: actor, Player: owner.ID, Kind: "heal", Skill: 12043, TX: ally.X, TY: ally.Y})
		if !ally.CanAct() || enemy.CanAct() {
			t.Fatal("cleanse crossed sides", side)
		}
		actor.SP = 0
		ally.Effects = append([]ActiveEffect(nil), enemy.Effects...)
		cure := r.Skills[12043]
		cure.SP = 10
		r.Skills[12043] = cure
		if step := r.support(b, Action{Actor: actor, Kind: "heal", Skill: 12043, TX: ally.X, TY: ally.Y}); len(step.Packets) != 0 || ally.CanAct() {
			t.Fatal("cleanse bypassed SP cost")
		}
	}
}
