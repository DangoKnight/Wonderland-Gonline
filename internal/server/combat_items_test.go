package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func serverItemFixture(t *testing.T) (*Server, *Session, *captureConn) {
	t.Helper()
	s, c, w := battleFixture(t, 1000)
	t.Cleanup(func() { c.battle.timer.Stop() })
	next := c.character.Clone()
	next.Bag[0] = game.Item{ID: 32003, Count: 2}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Items[32003] = game.ItemDefinition{ID: 32003, Name: "Rice Flour", Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{180, 120}}
	f := c.battle.member(c).self
	f.HP, f.SP = 1, 0
	f.Spd = 10000
	w.Reset()
	return s, c, w
}

func TestBattleItemNegotiatedChoiceExecutesOnce(t *testing.T) {
	s, c, w := serverItemFixture(t)
	run := c.battle
	c.combatCapabilities = 1
	run.member(c).choiceToken = 9
	c.combatSequence = 9
	p := []byte{254, 2, 9, 0, 0, 0, 1, 50, 2, 4, 2, 4, 2, 3, 125, 220, 205, 1, 0}
	if err := s.worldCommand(context.Background(), c, p); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	if c.battle != run || c.character.Bag[0].Count != 1 {
		t.Fatal("item round not executed")
	}
	steps := decodeRoundPackets(t, w.packets(t))
	var removals int
	for _, step := range steps {
		for _, p := range step.Packets {
			if len(p) == 4 && p[0] == protocol.CommandInventory && p[1] == protocol.InventoryRemove {
				removals++
			}
		}
	}
	if removals != 1 {
		t.Fatal("item receipt count", removals)
	}
	if err := s.worldCommand(context.Background(), c, p); err != nil || c.character.Bag[0].Count != 1 || w.Len() != 0 {
		t.Fatal("choice token replay consumed item", err)
	}
}

func TestBattleItemPlayerAndPetFinishSelection(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		chain, wounded, legacy bool
	}{{"native/full", false, false, false}, {"native/wounded", false, true, false}, {"chain/full", true, false, false}, {"chain/wounded", true, true, false}, {"captured legacy", false, false, true}} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, _ := serverItemFixture(t)
			run := c.battle
			run.timer.Stop()
			next := c.character.Clone()
			slot := 0
			if tc.legacy {
				slot = 2
			}
			next.Bag[slot] = game.Item{ID: 32176, Count: 50}
			s.Assets.Items[32176] = game.ItemDefinition{ID: 32176, Name: "Spicy Hot Pot", Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{400, 400}}
			next.Pets = []game.Pet{{ID: 10501, Slot: 1, Amity: 60, HP: 100, MaxHP: 100, SP: 30, MaxSP: 30, Battle: true}}
			next.ActivePet = 10501
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			m := run.member(c)
			m.pet = battle.PetFighter(c.character.Pets[0], c.character.ID, s.Assets.Items, 0)
			if tc.wounded {
				m.pet.HP, m.pet.SP = 1, 0
			}
			run.b.Attackers = append(run.b.Attackers, m.pet)
			choices := [][]byte{{50, 2, 4, 2, 3, 2, 176, 125, 0, 0, 1, 0}, {50, 2, 3, 2, 3, 2, 176, 125, 0, 0, 1, 0}}
			if tc.legacy {
				// Unmodified requests captured from legacy aLogin on 2026-11-03.
				choices = [][]byte{
					{50, 2, 4, 2, 4, 2, 176, 125, 60, 153, 3, 0, 70, 86, 2, 1, 3, 0, 0, 0, 186, 156, 67, 116, 200, 9, 211, 225, 52, 93, 168, 53, 193, 159, 223, 121, 77, 50, 211, 134, 97, 101, 4, 8, 135, 42, 168, 240, 93, 52, 168, 222},
					{50, 2, 3, 2, 4, 2, 176, 125, 140, 165, 3, 0, 70, 86, 2, 1, 4, 0, 0, 0, 243, 15, 90, 84, 123, 86, 243, 116, 115, 168, 76, 251, 37, 95, 32, 7, 123, 207, 39, 207, 22, 74, 212, 177, 30, 205, 97, 73, 38, 191, 162, 13},
				}
			}
			if tc.chain {
				c.combatCapabilities, m.choiceToken, c.combatSequence = 1, 9, 9
				p, err := protocol.EncodeCombatChoices(9, choices)
				if err != nil {
					t.Fatal(err)
				}
				choices = [][]byte{p}
			}
			for _, p := range choices {
				if err := s.worldCommand(context.Background(), c, p); err != nil {
					t.Fatal("item submission", err)
				}
			}
			settle(t, s, c)
			if run.turn != 1 || c.character.Bag[slot].Count != 48 {
				t.Fatal("both item choices did not execute", run.turn, c.character.Bag[slot], run.b.Pending)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || stored[0].Bag[slot].Count != 48 {
				t.Fatal("two item uses not saved", stored, err)
			}
		})
	}
}

func itemRules(s *Server, run *battleRun) battle.Rules {
	return s.combatRules(run)
}

func TestBattleItemDurableConsumptionNativeAndRoundChain(t *testing.T) {
	for _, chain := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[chain], func(t *testing.T) {
			s, c, w := serverItemFixture(t)
			run := c.battle
			r := itemRules(s, run)
			f := run.member(c).self
			if chain {
				c.combatCapabilities = 1
			}
			// Request bytes independently authored, including the opaque native trailer.
			if ack := run.b.Submit(r, c.character.ID, 2, []byte{4, 2, 4, 2, 3, 125, 220, 205, 1, 0}); ack == nil {
				t.Fatal("recovery choice rejected")
			}
			if c.character.Bag[0].Count != 2 {
				t.Fatal("selection consumed item")
			}
			run.b.Processing = true
			steps, _ := run.b.Round(r)
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || c.character.Bag[0].Count != 1 || stored[0].Bag[0].Count != 1 || stored[0].HP != 81 || stored[0].SP != min(20, c.character.MaxSP) {
				t.Fatal("item not atomically committed", stored, err)
			}
			if f.SP != min(20, f.MaxSP) {
				t.Fatal("item spent SP", f.SP)
			}
			if !s.play(run, steps) {
				t.Fatal("playback stopped")
			}
			packets := w.packets(t)
			if chain {
				chainPackets := packets
				packets = nil
				for _, step := range decodeRoundPackets(t, chainPackets) {
					packets = append(packets, step.Packets...)
				}
			}
			if !contains(packets, []byte{23, 9, 1, 1}) {
				t.Fatal("missing item removal receipt", packets)
			}
			if run.b.Submit(r, c.character.ID, 2, []byte{4, 2, 4, 2, 3, 125, 0, 0, 1, 0}) != nil {
				t.Fatal("replayed choice accepted before next turn")
			}
		})
	}
}

func TestBattleItemSQLFailureOrStaleBagGrantsNothing(t *testing.T) {
	for _, failure := range []string{"closed database", "stale bag"} {
		t.Run(failure, func(t *testing.T) {
			s, c, w := serverItemFixture(t)
			run := c.battle
			r := itemRules(s, run)
			f := run.member(c).self
			run.b.Submit(r, c.character.ID, 2, []byte{4, 2, 4, 2, 3, 125, 0, 0, 1, 0})
			if failure == "closed database" {
				s.Store.Close()
			} else {
				if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(c *game.Character) error { c.Bag[0] = game.Item{}; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			// Keep enemies from changing vitals while testing failed consumption.
			run.b.Defenders[0].HP = 0
			steps, _ := run.b.Round(r)
			if c.character.Bag[0].Count != 2 || f.HP != 1 || f.SP != 0 || len(steps) != 0 || w.Len() != 0 {
				t.Fatal("failed transaction published/applied item", steps, f)
			}
		})
	}
}

func TestBattleItemTeammatePetTransactionAndRollback(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "missing pet rollback"}[missing], func(t *testing.T) {
			s, players, _ := combatSessions(t)
			c, other := players[0], players[1]
			next := c.character.Clone()
			next.Bag[0] = game.Item{ID: 32003, Count: 2}
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			next = other.character.Clone()
			next.Pets = []game.Pet{{ID: 14162, Slot: 1, Name: "Roca", HP: 1, MaxHP: 100, SP: 0, MaxSP: 40}}
			if err := s.commit(context.Background(), other, next); err != nil {
				t.Fatal(err)
			}
			self := battle.PlayerFighter(c.character, s.Assets.Items, 0)
			pet := battle.PetFighter(other.character.Pets[0], other.character.ID, s.Assets.Items, 1)
			run := &battleRun{members: []*battleMember{{c: c, self: self}, {c: other, self: battle.PlayerFighter(other.character, s.Assets.Items, 1), pet: pet}}}
			c.battle, other.battle = run, run
			if missing {
				if err := s.Store.UpdateCharacter(context.Background(), other.account.ID, other.character.ID, func(c *game.Character) error { c.Pets = nil; return nil }); err != nil {
					t.Fatal(err)
				}
			}
			a := battle.Action{Actor: self, Player: c.character.ID, ItemSlot: 1, Item: c.character.Bag[0], Kind: "item"}
			if ok := s.commitBattleItem(run, a, pet, 81, 20); ok == missing {
				t.Fatal("transaction result", ok)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			want := byte(1)
			if missing {
				want = 2
			}
			if err != nil || c.character.Bag[0].Count != want || stored[0].Bag[0].Count != want {
				t.Fatal("consumption rollback", stored, err)
			}
			if !missing {
				stored, err = s.Store.Characters(context.Background(), other.account.ID)
				if err != nil || stored[0].Pets[0].HP != 81 || other.character.Pets[0].SP != 20 {
					t.Fatal("teammate pet recovery", stored, err)
				}
			}
		})
	}
}
