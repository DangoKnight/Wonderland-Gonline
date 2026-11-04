package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
)

func TestCapturedMonsterRecruitedAtLevelOne(t *testing.T) {
	s, c, w := battleFixture(t, 1000)
	run := c.battle
	run.timer.Stop()
	run.b.Captures = []battle.Capture{{Owner: c.character.ID, Template: 10500, Name: "Slime", Level: 50}}
	w.Reset()
	s.endBattle(run, battle.Victory)
	settle(t, s, c)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) == 0 || len(stored[0].Pets) != 1 {
		t.Fatal("capture not persisted", stored, err)
	}
	p := stored[0].Pets[0]
	if p.Level != 1 || p.Exp != 0 || p.HP != p.CalculatedMaxHP() || p.SP != p.CalculatedMaxSP() || p.HP != p.MaxHP || p.SP != p.MaxSP {
		t.Fatal("capture retained wild level or incorrect initial vitals", p)
	}
	if len(c.character.Pets) != 1 || c.character.Pets[0].Level != 1 {
		t.Fatal("session level differs from saved pet")
	}
	// The normal recruit/progression builders must announce exactly the stored
	// level-one companion to both legacy and Gonline clients.
	packets := w.packets(t)
	for _, packet := range p.ProgressionPackets(c.pets.slot(p.ID), s.Assets.Items) {
		if !contains(packets, packet) {
			t.Fatal("missing level-one progression", packet)
		}
	}
}

func TestCaptureNetUsesPersistAndSaveFailureDoesNotPublish(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s, c, w := battleFixture(t, 1000)
		run := c.battle
		run.timer.Stop()
		next := c.character.Clone()
		next.Equipment[game.CaptureNetEquipmentSlot-1] = game.Item{ID: game.MonsterNetItemID, Count: 1, Damage: 90}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		w.Reset()
		if fail {
			s.Store.Close()
		}
		allowance, ok := s.combatRules(run).PrepareCapture(battle.Action{Player: c.character.ID})
		if fail {
			if ok || c.character.Equipment[game.CaptureNetEquipmentSlot-1].Damage != 90 || w.Len() != 0 {
				t.Fatal("failed net use published")
			}
			continue
		}
		if !ok || allowance != 10 {
			t.Fatal("missing capture net allowance", allowance, ok)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || stored[0].Equipment[game.CaptureNetEquipmentSlot-1].Damage != 100 {
			t.Fatal("net use not saved", stored, err)
		}
		allowance, ok = s.combatRules(run).PrepareCapture(battle.Action{Player: c.character.ID})
		if !ok || allowance != 5 {
			t.Fatal("exhausted net still grants allowance")
		}
	}
}

func TestCaptureNetConsumedOnSuccessfulAndFailedRolls(t *testing.T) {
	for _, success := range []bool{false, true} {
		s, c, _ := battleFixture(t, 1000)
		run := c.battle
		run.timer.Stop()
		next := c.character.Clone()
		next.Equipment[game.CaptureNetEquipmentSlot-1] = game.Item{ID: game.PetNetItemID, Count: 1}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		target := run.b.Defenders[0]
		target.HP--
		r := s.combatRules(run)
		r.Float = func() float64 {
			if success {
				return 0
			}
			return .999
		}
		actor := run.member(c).self
		run.b.Submit(r, c.character.ID, 1, []byte{actor.X, actor.Y, target.X, target.Y, 0x18, 0x27})
		if !run.b.HasCommand(actor) {
			t.Fatal("capture command rejected")
		}
		run.b.Round(r)
		if (len(run.b.Captures) == 1) != success {
			t.Fatal("wrong capture outcome", run.b.Captures)
		}
		stored, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || stored[0].Equipment[game.CaptureNetEquipmentSlot-1].Damage != 10 {
			t.Fatal("attempt did not consume one net use", success, err)
		}
	}
}
