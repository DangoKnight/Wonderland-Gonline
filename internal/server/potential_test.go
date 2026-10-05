package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"testing"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func potentialFixture(t *testing.T, itemID uint16, level uint16, target byte) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := rebirthFixture(t)
	for _, id := range []uint16{34269, 34289, 34350} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: 25}
	}
	next := c.character.Clone()
	next.Bag[49] = game.Item{ID: itemID, Count: 1}
	if target == 0 {
		next.Potential = level
	} else {
		next.Pets[1].Potential = level
		next.Pets[1].Normalize(s.Assets.Items, false)
	}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	return s, c, wires
}

func TestPotentialPillPlayerAndPetSQLAndNativeReply(t *testing.T) {
	for _, target := range []byte{0, 2} {
		t.Run(string(rune('0'+target)), func(t *testing.T) {
			s, c, wires := potentialFixture(t, 34350, 11, target)
			before := c.character.Clone()
			// Walking is pending. Consuming a resource must not save stale session state.
			c.character.X += 20
			walkedX := c.character.X
			if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, target}); err != nil {
				t.Fatal(err)
			}
			packets := wires[0].packets(t)
			if len(packets) < 3 || !bytes.Equal(packets[0], []byte{23, 9, 50, 1}) || !bytes.Equal(packets[1], []byte{23, 213, 1, target, 12}) {
				t.Fatal("native pill replies", packets)
			}
			if !c.character.Bag[49].Empty() || c.character.X != walkedX || c.character.Base != before.Base {
				t.Fatal("wrong resource or walking state")
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || len(saved) != 1 || saved[0].X != before.X || saved[0].Bag != c.character.Bag {
				t.Fatal("SQL mutation", err, saved)
			}
			if target == 0 {
				if c.character.Potential != 12 || saved[0].Potential != 12 || c.character.HP != before.HP || c.character.SP != before.SP {
					t.Fatal("player potential/vitals")
				}
			} else {
				if c.character.Pets[1].Potential != 12 || c.character.Pets[1].Slot != 3 || saved[0].Pets[1].Potential != 12 || c.character.Pets[1].Base != before.Pets[1].Base || c.character.Pets[1].HP != before.Pets[1].HP || !reflect.DeepEqual(c.character.Pets[0], before.Pets[0]) {
					t.Fatal("wrong persistent pet", c.character.Pets)
				}
				if !contains(packets, protocol.Builder{8, 2, 4, 2, 0, 28, 1}.U32(88).U32(0)) {
					t.Fatal("pet bonus refresh", packets)
				}
			}
			if wires[1].Len() != 0 {
				t.Fatal("pill result leaked to observer")
			}
			state := c.character.Clone()
			if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, target}); err != nil || !reflect.DeepEqual(state, *c.character) {
				t.Fatal("replay recreated pill", err)
			}
			if contains(wires[0].packets(t), []byte{23, 213, 1, target, 12}) {
				t.Fatal("repeat succeeded")
			}
		})
	}
}

func TestPotentialPillRejectedAttemptsPreserveInventory(t *testing.T) {
	for _, tc := range []struct {
		name         string
		id, level    uint16
		slot, target byte
		locked       bool
	}{
		{"golden below minimum", 34289, 9, 50, 0, false},
		{"cap", 34350, 12, 50, 0, false},
		{"pet cap", 34350, 12, 50, 2, false}, {"missing pet", 34350, 0, 50, 4, false},
		{"invalid target", 34350, 0, 50, 255, false}, {"invalid slot", 34350, 0, 0, 0, false},
		{"reserved pill", 34350, 0, 50, 0, true}, {"wrong item", 34269, 0, 1, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, wires := potentialFixture(t, tc.id, tc.level, tc.target)
			c.character.Bag[49].Locked = tc.locked
			before := c.character.Clone()
			if err := s.dispatch(context.Background(), c, []byte{23, 126, tc.slot, tc.target}); err != nil {
				t.Fatal(err)
			}
			packets := wires[0].packets(t)
			if len(packets) == 0 || !bytes.Equal(packets[0], []byte{23, 213, 0, tc.target, 0}) || !reflect.DeepEqual(before, *c.character) {
				t.Fatal("rejected request mutated or acknowledged", packets)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || saved[0].Bag[49].ID != tc.id || saved[0].Bag[49].Count != 1 {
				t.Fatal("rejection consumed pill", err)
			}
		})
	}
}

func TestPotentialPillSQLAuthorityAndFailedSave(t *testing.T) {
	s, c, wires := potentialFixture(t, 34350, 0, 0)
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	if _, err := s.Store.MutateOwnedCharacter(context.Background(), ref, func(next *game.Character) error { return next.Bag.Remove(50, 1) }); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); err != nil || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("stale session spent missing SQL pill", err)
	}
	if contains(wires[0].packets(t), []byte{23, 213, 1, 0, 1}) {
		t.Fatal("stale session acknowledged")
	}
	s, c, wires = potentialFixture(t, 34350, 0, 0)
	before = c.character.Clone()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.dispatch(ctx, c, []byte{23, 126, 50, 0}); err == nil || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed save published mutation", err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Potential != 0 || saved[0].Bag[49].Count != 1 {
		t.Fatal("failed save consumed pill", err)
	}
}

func TestPotentialPillSocketFailureKeepsCommittedConsumption(t *testing.T) {
	s, c, wires := potentialFixture(t, 34350, 0, 0)
	c.conn = &failedWorldConn{}
	if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); err == nil {
		t.Fatal("socket error missing")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Potential != 1 || !saved[0].Bag[49].Empty() || c.character.Potential != 1 {
		t.Fatal("committed pill reverted", err)
	}
	c.conn = wires[0]
	if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); err != nil || c.character.Potential != 1 {
		t.Fatal("retry granted another level", err)
	}
}

func TestPotentialPillMalformedAndWorldGates(t *testing.T) {
	s, c, wires := potentialFixture(t, 34350, 0, 0)
	before := c.character.Clone()
	for _, p := range [][]byte{{23, 126}, {23, 126, 50}, {23, 126, 50, 0, 1}} {
		if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) || wires[0].Len() != 0 {
			t.Fatal("malformed pill", p, err)
		}
	}
	for _, set := range []func(){func() { c.battle = &battleRun{} }, func() { c.trade = &tradeSession{} }, func() { c.event = &eventSession{} }, func() { c.storm = true }, func() { c.beach = &beachRun{} }} {
		set()
		if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); err != nil || !reflect.DeepEqual(before, *c.character) {
			t.Fatal("locked pill", err)
		}
		for _, packet := range wires[0].packets(t) {
			if bytes.Equal(packet, []byte{23, 213, 1, 0, 1}) {
				t.Fatal("locked pill acknowledged")
			}
		}
		c.battle, c.trade, c.event, c.beach = nil, nil, nil, nil
		c.storm = false
	}
	c.ready = false
	if err := s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); !errors.Is(err, protocol.ErrMalformed) || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("loading pill", err)
	}
}

func TestPotentialPillRollbackAfterSQLStateUpdate(t *testing.T) {
	s, c, wire, path := mallFixture(t)
	var logs bytes.Buffer
	s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
	s.Assets.Items[34350] = game.ItemDefinition{ID: 34350, Type: 25}
	next := c.character.Clone()
	next.Bag[49] = game.Item{ID: 34350, Count: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	// saveCharacter updates potential before rewriting item rows. Fail the latter
	// to prove both the increased potential and inventory debit are rolled back.
	if _, err = raw.Exec("CREATE TRIGGER reject_pill BEFORE INSERT ON character_items BEGIN SELECT RAISE(ABORT,'reject pill inventory save'); END"); err != nil {
		t.Fatal(err)
	}
	before := c.character.Clone()
	if err = s.dispatch(context.Background(), c, []byte{23, 126, 50, 0}); err == nil || wire.Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("failed SQL published potential", err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Potential != 0 || saved[0].Bag[49].Count != 2 {
		t.Fatal("SQL rollback lost pill or retained potential", err, saved)
	}
	var entry map[string]any
	if err = json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err, logs.String())
	}
	if entry["msg"] != "potential pill attempt not committed" || entry["committed"] != false || entry["potential_after"] != float64(1) || entry["error"] == nil {
		t.Fatal("rollback logged as committed", entry)
	}
}

func TestPotentialPillRandomOutcomesConsumeAndPersist(t *testing.T) {
	for _, tc := range []struct {
		name            string
		item, old, want uint16
		roll            int
		target          byte
	}{
		{"normal success", 34269, 10, 11, 17, 0}, {"normal failure", 34269, 10, 9, 18, 0},
		{"golden success", 34289, 10, 11, 17, 0}, {"golden failure", 34289, 10, 10, 18, 0},
		{"pet normal failure", 34269, 10, 9, 99, 2}, {"pet golden failure", 34289, 10, 10, 99, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, wires := potentialFixture(t, tc.item, tc.old, tc.target)
			if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, tc.target}, func() (int, error) { return tc.roll, nil }); err != nil {
				t.Fatal(err)
			}
			got := wires[0].packets(t)
			if !contains(got, []byte{23, 213, 1, tc.target, byte(tc.want)}) || !contains(got, []byte{23, 9, 50, 1}) {
				t.Fatal("attempt reply", got)
			}
			saved, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || !saved[0].Bag[49].Empty() {
				t.Fatal("pill debit", err)
			}
			potential := saved[0].Potential
			if tc.target != 0 {
				potential = saved[0].Pets[1].Potential
			}
			if potential != tc.want {
				t.Fatal("potential", potential, tc.want)
			}
		})
	}
}

func TestPotentialPillGuaranteedInitialLevelsAndRandomFailure(t *testing.T) {
	for level := uint16(0); level < 3; level++ {
		s, c, wires := potentialFixture(t, 34269, level, 0)
		if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, 0}, func() (int, error) { t.Fatal("guaranteed attempt rolled"); return 0, nil }); err != nil {
			t.Fatal(err)
		}
		if !contains(wires[0].packets(t), []byte{23, 213, 1, 0, byte(level + 1)}) {
			t.Fatal("guaranteed normal failed")
		}
	}
	s, c, wires := potentialFixture(t, 34269, 4, 0)
	before := c.character.Clone()
	failure := errors.New("random source failed")
	if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, 0}, func() (int, error) { return 0, failure }); !errors.Is(err, failure) || wires[0].Len() != 0 || !reflect.DeepEqual(before, *c.character) {
		t.Fatal("random failure consumed pill", err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Potential != 4 || saved[0].Bag[49].Count != 1 {
		t.Fatal("random failure changed SQL", err)
	}
}

func TestPotentialPillNormalFailureForgetsSkillAndClampsVitals(t *testing.T) {
	s, c, wires := potentialFixture(t, 34269, 4, 0)
	s.Assets.Skills[11001] = assets.Skill{ID: 11001, TableOrder: 100}
	s.Assets.Skills[15091] = assets.Skill{ID: 15091, TableOrder: 101}
	next := c.character.Clone()
	next.Body, next.Head, next.Element = 1, 0, game.Water
	next.Base = game.Attributes{Strength: 9, Constitution: 10, Wisdom: 10}
	next.Skills = []game.LearnedSkill{{ID: 11001, Grade: 5, EXP: 17}, {ID: 15091, Grade: 1}}
	next.RecalculateVitals(s.Assets.Items)
	next.HP, next.SP = next.MaxHP, next.MaxSP
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, 0}, func() (int, error) { return 99, nil }); err != nil {
		t.Fatal(err)
	}
	if c.character.Potential != 3 || len(c.character.Skills) != 1 || c.character.Skills[0].ID != 15091 || c.character.HP != c.character.MaxHP || c.character.SP != c.character.MaxSP || c.character.HP >= next.HP || c.character.SP >= next.SP {
		t.Fatal("failed pill did not clamp or forget", c.character)
	}
	packets := wires[0].packets(t)
	if !contains(packets, []byte{23, 213, 1, 0, 3}) || !contains(packets, []byte{5, 4}) {
		t.Fatal("skill removal not refreshed", packets)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(saved[0].Skills, c.character.Skills) || saved[0].HP != c.character.HP {
		t.Fatal("skill loss not saved", err)
	}
}

func TestPotentialPillCalculationLogs(t *testing.T) {
	for _, tc := range []struct {
		name                string
		item, before, after uint16
		chance, roll        int
		target              byte
		random              bool
	}{
		{"guaranteed normal", 34269, 0, 1, 100, 99, 0, false},
		{"normal success boundary", 34269, 3, 4, 70, 69, 0, true},
		{"normal failure boundary", 34269, 3, 2, 70, 70, 0, true},
		{"golden failure", 34289, 10, 10, 18, 18, 2, true},
		{"guaranteed super", 34350, 11, 12, 100, 99, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := potentialFixture(t, tc.item, tc.before, tc.target)
			var logs bytes.Buffer
			s.Log = slog.New(slog.NewJSONHandler(&logs, nil))
			rolls := 0
			if err := s.potentialPillWithRoll(context.Background(), c, []byte{23, 126, 50, tc.target}, func() (int, error) { rolls++; return tc.roll, nil }); err != nil {
				t.Fatal(err)
			}
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err, logs.String())
			}
			want := map[string]any{
				"msg": "potential pill attempt", "level": "INFO", "committed": true,
				"character": float64(c.character.ID), "bag_slot": float64(50), "item_id": float64(tc.item), "target_slot": float64(tc.target),
				"potential_before": float64(tc.before), "attempted_potential": float64(tc.before + 1), "potential_after": float64(tc.after),
				"success_chance_percent": float64(tc.chance), "roll_max_exclusive": float64(100), "comparison": "roll < success_chance_percent",
				"random_roll_used": tc.random, "success": tc.after > tc.before,
				"bonus_before": float64(game.PotentialBonus(tc.before)), "bonus_after": float64(game.PotentialBonus(tc.after)),
				"bonus_delta_per_attribute": float64(int(game.PotentialBonus(tc.after)) - int(game.PotentialBonus(tc.before))),
			}
			if tc.random {
				want["roll"] = float64(tc.roll)
				if rolls != 1 {
					t.Fatal("unexpected random draws", rolls)
				}
			} else {
				want["roll"] = nil
				if rolls != 0 {
					t.Fatal("guaranteed attempt rolled", rolls)
				}
			}
			for key, value := range want {
				if got, ok := entry[key]; !ok || got != value {
					t.Fatal("wrong calculation field", key, got, value, entry)
				}
			}
		})
	}
}

func TestPotentialPetRosterRestoresBeforeFirstAttempt(t *testing.T) {
	s, c, _ := potentialFixture(t, 34269, 4, 2)
	next := c.character.Clone()
	next.Pets[0].Potential, next.Pets[0].Job = 7, 2
	next.Pets[1].Job = 6
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	roster := newPetRoster()
	packets := s.rosterPackets(&saved[0], roster)
	if len(packets) == 0 || !bytes.Equal(packets[0][:2], []byte{15, 8}) {
		t.Fatal("initial roster absent", packets)
	}
	packet := packets[0]
	// Native record: 30-byte header including name length; name; 3 skills
	// (15 bytes); six 21-byte equipment records; 11-byte footer.
	at := 2
	for _, want := range []struct{ slot, potential, job byte }{{1, 7, 2}, {2, 4, 6}} {
		length := 182 + int(packet[at+29])
		footer := packet[at+length-11 : at+length]
		if packet[at] != want.slot || footer[3] != want.potential || footer[4] != want.job {
			t.Fatal("incorrect initial native potential/job", want, footer)
		}
		at += length
	}
	if at != len(packet) {
		t.Fatal("native roster record framing", at, len(packet))
	}
	for _, packet := range packets[1:] {
		if len(packet) >= 6 && packet[0] == 8 && packet[1] == 2 && packet[5] == 37 {
			t.Fatal("wrong potential stat overwrote unrelated field", packet)
		}
		if len(packet) >= 2 && packet[0] == 23 && packet[1] == 213 {
			t.Fatal("login fabricated pill result", packet)
		}
	}
	if saved[0].Pets[1].Slot != 3 || roster.slot(saved[0].Pets[1].ID) != 2 {
		t.Fatal("test did not cover sparse saved slots")
	}
}
