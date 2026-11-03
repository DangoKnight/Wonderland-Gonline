package battle

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func itemBattle(t *testing.T) (*Battle, Rules, *Fighter) {
	t.Helper()
	b, _ := setup(1000)
	f := b.Attackers[0]
	f.HP, f.MaxHP, f.SP, f.MaxSP = 20, 100, 5, 40
	f.Char.Bag[0] = game.Item{ID: 32003, Count: 2}
	r := lowest
	r.Items = map[uint16]assets.BattleItem{32003: {ID: 32003, HP: 80, SP: 20}}
	r.CommitItem = func(a Action, target *Fighter, hp, sp int) bool { return f.Char.Bag.Remove(a.ItemSlot, 1) == nil }
	return b, r, f
}

func TestBattleItemNativeRequestAndTwoStatGolden(t *testing.T) {
	b, r, f := itemBattle(t)
	// Independent native request: actor, target, item ID, random bytes, slot, flag.
	data := []byte{4, 2, 4, 2, 3, 125, 220, 205, 1, 0}
	if ack := b.Submit(r, 7, 2, data); !bytes.Equal(ack, []byte{53, 5, 4, 2}) {
		t.Fatal("item ack", ack)
	}
	if f.HP != 20 || f.Char.Bag[0].Count != 2 {
		t.Fatal("selection granted/consumed item early")
	}
	if b.Submit(r, 7, 2, data) != nil {
		t.Fatal("duplicate choice accepted")
	}
	steps := b.useItem(r, b.Pending[f.key()])
	// Item Usage (10024), two recovery stats, capped HP +80, SP +20.
	want := []byte{50, 1, 23, 0, 4, 2, 40, 39, 0, 1, 4, 2, 1, 0, 2, 25, 80, 0, 0, 0, 3, 26, 20, 0, 0, 0, 3}
	if len(steps) != 1 || !bytes.Equal(steps[0].Packets[1], want) || f.HP != 100 || f.SP != 25 || f.Char.Bag[0].Count != 1 {
		t.Fatal("item result", steps, f)
	}
	if !bytes.Equal(steps[0].OwnerPackets[7][0], []byte{23, 9, 1, 1}) {
		t.Fatal("private consumption receipt", steps[0].OwnerPackets)
	}
	if len(steps[0].SkillUses) != 0 {
		t.Fatal("items awarded skill experience")
	}
}

func TestBattleItemCapturedLegacyCompatibilityTrailer(t *testing.T) {
	b, r, f := itemBattle(t)
	f.Char.Bag[2] = game.Item{ID: 32176, Count: 50}
	r.Items[32176] = assets.BattleItem{ID: 32176, HP: 300, SP: 300}
	// Actual 52-byte Spicy Hot Pot request captured from legacy aLogin.
	p := []byte{50, 2, 4, 2, 4, 2, 176, 125, 60, 153, 3, 0, 70, 86, 2, 1, 3, 0, 0, 0, 186, 156, 67, 116, 200, 9, 211, 225, 52, 93, 168, 53, 193, 159, 223, 121, 77, 50, 211, 134, 97, 101, 4, 8, 135, 42, 168, 240, 93, 52, 168, 222}
	for _, size := range []int{11, 13, 51, 53} {
		malformed := append(append([]byte(nil), p...), 0)[:size]
		if b.Submit(r, 7, malformed[1], malformed[2:]) != nil {
			t.Fatal("unverified item request length accepted", size)
		}
	}
	if ack := b.Submit(r, 7, p[1], p[2:]); !bytes.Equal(ack, []byte{53, 5, 4, 2}) {
		t.Fatal("captured native item ignored", ack)
	}
	if f.Char.Bag[2].Count != 50 {
		t.Fatal("selection consumed item")
	}
}

func TestBattleItemRejectsInvalidChoicesAndFailedConsumption(t *testing.T) {
	for _, test := range []string{"truncated", "foreign actor", "bad target", "empty slot", "wrong ID", "locked", "dead", "joining", "failed save"} {
		t.Run(test, func(t *testing.T) {
			b, r, f := itemBattle(t)
			data := []byte{4, 2, 4, 2, 3, 125, 0, 0, 1, 0}
			switch test {
			case "truncated":
				data = data[:9]
			case "foreign actor":
				data[0] = 1
			case "bad target":
				data[2] = 2
			case "empty slot":
				data[8] = 2
			case "wrong ID":
				data[4] = 4
			case "locked":
				f.Char.Bag[0].Locked = true
			case "dead":
				f.HP = 0
			case "joining":
				f.Joining = true
			case "failed save":
				r.CommitItem = func(Action, *Fighter, int, int) bool { return false }
			}
			beforeHP, beforeSP, beforeBag := f.HP, f.SP, f.Char.Bag
			ack := b.Submit(r, 7, 2, data)
			if test == "failed save" {
				if ack == nil || len(b.useItem(r, b.Pending[f.key()])) != 0 {
					t.Fatal("failed save published result")
				}
			} else if ack != nil || len(b.Pending) != 0 {
				t.Fatal("invalid request consumed turn", ack)
			}
			if f.HP != beforeHP || f.SP != beforeSP || f.Char.Bag != beforeBag {
				t.Fatal("invalid item changed resources")
			}
		})
	}
}

func TestBattleItemRevivalCreatureMotionAndReservations(t *testing.T) {
	b, r, f := itemBattle(t)
	pet := &Fighter{Kind: Pet, ID: 14162, Owner: 7, Side: Attacker, X: 3, Y: 2, HP: 0, MaxHP: 100, MaxSP: 30, Pet: &game.Pet{ID: 14162}}
	b.Attackers = append(b.Attackers, pet)
	r.Items[32003] = assets.BattleItem{ID: 32003, Skill: 15138, HP: 80}
	r.Skills = map[uint16]assets.Skill{15138: {ID: 15138, Name: "Secular Feather"}}
	if b.Submit(r, 7, 2, []byte{4, 2, 3, 2, 3, 125, 0, 0, 1, 0}) == nil {
		t.Fatal("revive rejected")
	}
	if steps := b.useItem(r, b.Pending[f.key()]); len(steps) != 1 || pet.HP != 80 || f.SP != 5 {
		t.Fatal("revive costs SP or failed", pet, steps)
	}
	b.Pending = map[int]Action{}
	r.Items[32003] = assets.BattleItem{ID: 32003, HP: 80}
	pet.HP = 20
	f.Char.Bag[0].Count = 1
	if b.Submit(r, 7, 2, []byte{4, 2, 3, 2, 3, 125, 0, 0, 1, 0}) == nil {
		t.Fatal("pet recovery rejected")
	}
	if b.Submit(r, 7, 2, []byte{3, 2, 4, 2, 3, 125, 0, 0, 1, 0}) != nil {
		t.Fatal("two actors reserved last item")
	}
	steps := b.useItem(r, b.Pending[f.key()])
	if len(steps) != 1 || !bytes.Equal(steps[0].Packets[1][6:8], []byte{41, 39}) {
		t.Fatal("creature item motion", steps)
	}
}

func TestBattleItemPetTurnOrderingAndChangedTarget(t *testing.T) {
	b, r, f := itemBattle(t)
	p := &Fighter{Kind: Pet, ID: 14162, Owner: 7, Side: Attacker, X: 3, Y: 2, HP: 100, MaxHP: 100, Spd: 200, Pet: &game.Pet{ID: 14162}}
	b.Attackers = append(b.Attackers, p)
	if b.Submit(r, 7, 2, []byte{3, 2, 4, 2, 3, 125, 0, 0, 1, 0}) == nil {
		t.Fatal("pet item choice rejected")
	}
	groups := b.order(r, []Action{b.Pending[p.key()]})
	if len(groups) == 0 || groups[0][0].Actor != p {
		t.Fatal("item ignored actor speed")
	}
	f.HP = 0
	if steps := b.useItem(r, b.Pending[p.key()]); len(steps) != 0 || f.Char.Bag[0].Count != 2 {
		t.Fatal("food revived a target defeated before execution")
	}
	f.HP, f.Absent = 20, true
	if steps := b.useItem(r, b.Pending[p.key()]); len(steps) != 0 || f.Char.Bag[0].Count != 2 {
		t.Fatal("item used on disconnected target")
	}
}

func TestBattleItemThrowableCountsUnclampedRoundDamage(t *testing.T) {
	b, r, _ := itemBattle(t)
	enemy := b.Defenders[0]
	enemy.HP, enemy.MaxHP = 100, 100
	r.Items[32003] = assets.BattleItem{ID: 32003, Skill: 10013, HP: -151}
	r.Skills = map[uint16]assets.Skill{10013: {ID: 10013, Name: "Bomb"}}
	if b.Submit(r, 7, 2, []byte{4, 2, enemy.X, enemy.Y, 3, 125, 0, 0, 1, 0}) == nil {
		t.Fatal("throwable rejected")
	}
	steps := b.useItem(r, b.Pending[b.Attackers[0].key()])
	if len(steps) == 0 || !enemy.Overkilled || enemy.RoundDamage != 151 {
		t.Fatal("throwable lost overkill damage", enemy)
	}
}

func TestBattleItemFullTeammateCanTakeDamageBeforeRecovery(t *testing.T) {
	b, r, f := itemBattle(t)
	ally := &Fighter{Kind: Player, ID: 8, Side: Attacker, X: 4, Y: 3, HP: 100, MaxHP: 100, SP: 40, MaxSP: 40}
	b.Attackers = append(b.Attackers, ally)
	if b.Submit(r, 7, 2, []byte{4, 2, 4, 3, 3, 125, 0, 0, 1, 0}) == nil {
		t.Fatal("full teammate rejected at selection")
	}
	ally.hurt(90)
	steps := b.useItem(r, b.Pending[f.key()])
	if len(steps) != 1 || ally.HP != 90 || ally.SP != 40 || f.Char.Bag[0].Count != 1 {
		t.Fatal("teammate was not healed at execution", ally, steps)
	}
}
