package app

import (
	"bytes"
	"maps"
	"testing"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

const autoTestAttackSkill = 11007
const autoTestPetAttackSkill = 11008
const autoTestSupportLayer = 4

func autoTestDefinition(c *Client, id uint16, layer byte, cost uint16) {
	// Definitions are shared by normal client sessions; isolate these authored
	// test skills rather than editing the resource cache.
	catalog := *c.SkillState.Catalog
	catalog.Definitions = maps.Clone(catalog.Definitions)
	catalog.Definitions[id] = skills.Definition{Skill: assets.Skill{ID: id, EffectLayer: layer, SP: cost}}
	c.SkillState.Catalog = &catalog
}

func autoTestAcknowledge(c *Client, cell clientbattle.Cell, id uint16) {
	for _, actor := range c.remote.battle.fighters {
		if actor.x == cell.X && actor.y == cell.Y {
			c.rememberBattleSubmission(actor, id)
			c.dispatch([]byte{53, 5, cell.X, cell.Y})
			return
		}
	}
}

func TestBattleAutoRepeatsAttackAndFallsBack(t *testing.T) {
	fixture := newClientFixture(t)
	for _, tc := range []struct {
		name               string
		layer              byte
		sp                 byte
		unlearned, missing bool
		repeat             bool
	}{
		{name: "physical", layer: skills.Physical, sp: 100, repeat: true},
		{name: "magical", layer: skills.Magical, sp: 100, repeat: true},
		{name: "exact_SP", layer: skills.Magical, sp: 6, repeat: true},
		{name: "insufficient_SP", layer: skills.Physical, sp: 5},
		{name: "support", layer: autoTestSupportLayer, sp: 100},
		{name: "revive", layer: nativeReviveLayer, sp: 100},
		{name: "control", layer: nativeControlLayer, sp: 100},
		{name: "debuff", layer: nativeDebuffLayer, sp: 100},
		{name: "alternate_debuff", layer: nativeDebuffAlternateLayer, sp: 100},
		{name: "forgotten", layer: skills.Physical, sp: 100, unlearned: true},
		{name: "unknown", layer: skills.Physical, sp: 100, missing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _, sent := enterBattleWith(t, fixture)
			autoTestDefinition(c, autoTestAttackSkill, tc.layer, 6)
			c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
			autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, autoTestAttackSkill)
			if tc.unlearned {
				delete(c.SkillState.Learned, autoTestAttackSkill)
			}
			if tc.missing {
				delete(c.SkillState.Catalog.Definitions, autoTestAttackSkill)
			}
			// Authoritative SP update before the next choice period.
			c.dispatch([]byte{51, 1, 4, 2, 26, tc.sp, 0, 0, 0})
			c.dispatch([]byte{50, 6, 4, 2, 0})
			c.dispatch([]byte{52, 1})
			c.chooseBattleCommand(battleCommandAuto)
			want := []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}
			if tc.repeat {
				want[6], want[7] = 0xff, 0x2a
			}
			if p := sent(); len(p) != 1 || !bytes.Equal(p[0], want) {
				t.Fatalf("AUTO choice %v, want %v", p, want)
			}
			c.battleAutoTick()
			if len(sent()) != 1 {
				t.Fatal("AUTO submitted twice in one turn")
			}
		})
	}
}

func TestBattleAutoUsesSeparatePlayerAndPetAttacks(t *testing.T) {
	c, now, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	autoTestDefinition(c, autoTestAttackSkill, skills.Physical, 6)
	autoTestDefinition(c, autoTestPetAttackSkill, skills.Magical, 9)
	c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 14162, Skills: [3]game.PetSkill{{ID: autoTestPetAttackSkill, Grade: 1}}}
	autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, autoTestAttackSkill)
	autoTestAcknowledge(c, clientbattle.Cell{X: 3, Y: 2}, autoTestPetAttackSkill)
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.chooseBattleCommand(battleCommandAuto)
	want := [][]byte{{50, 1, 4, 2, 2, 2, 0xff, 0x2a}, {50, 1, 3, 2, 2, 2, 0, 0x2b}}
	p := sent()
	if len(p) != len(want) {
		t.Fatal("missing player or pet choice", p)
	}
	for i := range want {
		if !bytes.Equal(p[i], want[i]) {
			t.Fatalf("choice %d = %v, want %v", i, p[i], want[i])
		}
	}
	c.dispatch([]byte{53, 5, 4, 2})
	c.dispatch([]byte{53, 5, 3, 2})
	autoTestNextBattle(t, c, now, 14162)
	c.chooseBattleCommand(battleCommandAuto)
	p = sent()
	want = [][]byte{{50, 1, 4, 3, 2, 2, 0xff, 0x2a}, {50, 1, 3, 3, 2, 2, 0, 0x2b}}
	if len(p) != 4 {
		t.Fatal("next battle lost player or pet history", p)
	}
	for i := range want {
		if !bytes.Equal(p[i+2], want[i]) {
			t.Fatal("actor histories mixed across battles", p)
		}
	}
}

func TestBattleAutoItemUseDiscardsOlderAttack(t *testing.T) {
	c, sent := clientItemFixture(t)
	autoTestDefinition(c, autoTestAttackSkill, skills.Physical, 6)
	c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
	autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, autoTestAttackSkill)
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.selectBattleItem(1, false)
	for _, actor := range c.remote.battle.fighters {
		if actor.x == 4 && actor.y == 2 {
			c.submitBattleItem(actor)
			break
		}
	}
	if p := sent(); len(p) != 1 || p[0][1] != 2 {
		t.Fatal("item request missing", p)
	}
	c.dispatch([]byte{53, 5, 4, 2})
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.chooseBattleCommand(battleCommandAuto)
	p := sent()
	if len(p) != 3 || !bytes.Equal(p[1], []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}) {
		t.Fatal("item use replayed the older attack", p)
	}
}

func autoTestNextBattle(t *testing.T, c *Client, now *time.Time, pet uint32) {
	t.Helper()
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.battle.auto {
		t.Fatal("AUTO remained enabled after battle exit")
	}
	// The next formation uses different cells for the same owned actors.
	c.dispatch(remoteRecord(250, 2, 4, 3, 10001, 0))
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	c.dispatch(enemy)
	if pet != 0 {
		c.dispatch(remoteRecord(5, 4, 3, 3, pet, 10001))
	}
	c.dispatch([]byte{50, 6, 4, 3, 0})
	c.dispatch([]byte{52, 1})
	finishSceneFade(c, now)
	c.battleFrame(false)
	if c.battle.auto {
		t.Fatal("AUTO enabled itself in the next battle")
	}
}

func TestBattleAutoRemembersManualAndAutoAttackBetweenBattles(t *testing.T) {
	fixture := newClientFixture(t)
	for _, automatic := range []bool{false, true} {
		t.Run(map[bool]string{false: "manual", true: "AUTO"}[automatic], func(t *testing.T) {
			c, now, sent := enterBattleWith(t, fixture)
			autoTestDefinition(c, autoTestAttackSkill, skills.Physical, 6)
			c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
			autoTestAcknowledge(c, clientbattle.Cell{X: 4, Y: 2}, autoTestAttackSkill)
			if automatic {
				c.dispatch([]byte{50, 6, 4, 2, 0})
				c.dispatch([]byte{52, 1})
				c.chooseBattleCommand(battleCommandAuto)
				c.dispatch([]byte{53, 5, 4, 2})
			}
			before := len(sent())
			autoTestNextBattle(t, c, now, 0)
			if len(sent()) != before {
				t.Fatal("next battle submitted without enabling AUTO", sent())
			}
			c.chooseBattleCommand(battleCommandAuto)
			p := sent()
			if len(p) != before+1 || !bytes.Equal(p[before], []byte{50, 1, 4, 3, 2, 2, 0xff, 0x2a}) {
				t.Fatal("last attack not retained across battles", p)
			}
		})
	}
}
