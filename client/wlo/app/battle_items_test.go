package app

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/cursor"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func clientItemFixture(t *testing.T) (*Client, func() [][]byte) {
	t.Helper()
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.items[32003] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32003, Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{180, 120}}}
	c.InventoryState.Bag[0] = game.Item{ID: 32003, Count: 2}
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 14162}
	f := c.battle.state.At(clientbattle.Cell{X: 4, Y: 2})
	f.HP, f.SP = 20, 0
	return c, sent
}

func TestBattleItemTwoVitalsRecoverAtContact(t *testing.T) {
	c, now, _ := enterBattle(t)
	cell := clientbattle.Cell{X: 4, Y: 2}
	c.dispatch([]byte{51, 1, 4, 2, 25, 20, 0, 0, 0, 4, 2, 26, 0, 0, 0, 0})
	c.battleFrame(false)
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10024: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
	start := *now
	// Independent native result with simultaneous HP +80 and SP +20.
	c.dispatch([]byte{50, 1, 23, 0, 4, 2, 40, 39, 0, 1, 4, 2, 1, 0, 2, 25, 80, 0, 0, 0, 3, 26, 20, 0, 0, 0, 3})
	c.dispatch([]byte{51, 1, 4, 2, 25, 100, 0, 0, 0, 4, 2, 26, 20, 0, 0, 0})
	c.battleFrame(false)
	*now = start.Add(149 * time.Millisecond)
	c.battleFrame(false)
	f := c.battle.displayed(*c.battle.state.At(cell))
	if f.HP != 20 || f.SP != 0 {
		t.Fatal("item recovery displayed before contact", f)
	}
	*now = start.Add(150 * time.Millisecond)
	c.battleFrame(false)
	f = c.battle.displayed(*c.battle.state.At(cell))
	if f.HP != 100 || f.SP != 20 {
		t.Fatal("item recovery missed contact", f)
	}
	if len(c.battle.floats) != 2 || c.battle.floats[0].font != "Num_YellowRed10_2" || c.battle.floats[1].font != "Num_Blue10_1" {
		t.Fatal("native HP/SP recovery number sheets", c.battle.floats)
	}
	if _, exists := c.battle.lastActions[cell]; exists {
		t.Fatal("item animation became a repeatable learned skill")
	}
}

func TestBattleItemCapturedLegacyRecoveryAtContact(t *testing.T) {
	c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	cell := clientbattle.Cell{X: 4, Y: 2}
	f := c.battle.state.At(cell)
	f.SP, f.MaxSP = 779, 1808
	c.battleFrame(false)
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10024: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
	start := *now
	// Captured native item result; mode 0 must remain a recovery on playback.
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 40, 39, 0, 1, 4, 2, 1, 0, 1, 26, 144, 1, 0, 0, 0})
	c.battleFrame(false)
	*now = start.Add(149 * time.Millisecond)
	c.battleFrame(false)
	if got := c.battle.displayed(*c.battle.state.At(cell)); got.SP != 779 {
		t.Fatal("item recovery shown before contact", got.SP)
	}
	*now = start.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if got := c.battle.displayed(*c.battle.state.At(cell)); got.SP != 1179 {
		t.Fatal("native mode-zero recovery missed contact", got.SP)
	}
	if len(c.battle.floats) != 1 || c.battle.floats[0].font != "Num_Blue10_1" {
		t.Fatal("native SP recovery digits", c.battle.floats)
	}
}

func TestBattleItemModeZeroHPRecoveryDigits(t *testing.T) {
	c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	cell := clientbattle.Cell{X: 3, Y: 2}
	c.battle.state.At(cell).HP = 20
	c.battleFrame(false)
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10025: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
	start := *now
	// Native creature recovery: HP +35, mode 0. The Nian Noodle screenshot
	// establishes the orange outlined number presentation on the recipient.
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 41, 39, 0, 1, 3, 2, 1, 0, 1, 25, 35, 0, 0, 0, 0})
	c.battleFrame(false)
	*now = start.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if len(c.battle.floats) != 1 || c.battle.floats[0].font != "Num_YellowRed10_2" || c.battle.floats[0].amount != 35 || c.battle.floats[0].cell != cell {
		t.Fatal("native HP recovery digits on recipient", c.battle.floats)
	}
}

func TestBattleItemInventorySelectionAndNativeRequest(t *testing.T) {
	c, sent := clientItemFixture(t)
	c.chooseBattleCommand(battleCommandItems)
	if !c.Inventory.Visible || c.Inventory.CanAct() {
		t.Fatal("inventory browsing/action gate")
	}
	c.Input.Hovered = c.Inventory.Slots[0]
	c.UI.DblClick()
	if c.battle.item == nil || c.Inventory.Visible || len(sent()) != 0 || c.InventoryState.Bag[0].Count != 2 {
		t.Fatal("selection sent/consumed item")
	}
	if got := c.battleCursor(); got != cursor.ShapeMagic {
		t.Fatal("item targeting cursor", got)
	}
	c.battleMenu()
	if c.battle.menu != nil && c.battle.menu.Visible {
		t.Fatal("menu obscures item target")
	}
	at := (clientbattle.Cell{X: 4, Y: 2}).Position()
	c.GroundClick(at.X, at.Y-30)
	want := []byte{50, 2, 4, 2, 4, 2, 3, 125, 0, 0, 1, 0}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], want) || c.battle.item != nil {
		t.Fatal("native item choice", got)
	}
	c.GroundClick(at.X, at.Y-30)
	if len(sent()) != 1 || c.InventoryState.Bag[0].Count != 2 {
		t.Fatal("duplicate or predicted consumption")
	}
	c.dispatch([]byte{23, 9, 1, 1})
	if c.InventoryState.Bag[0].Count != 1 {
		t.Fatal("server consumption ignored")
	}
}

func TestBattleItemClickAnyTeammateAtFullVitals(t *testing.T) {
	// All four allied owners and their pets, including fighters we cannot command.
	for y := byte(1); y <= 4; y++ {
		for _, x := range []byte{3, 4} {
			cell := clientbattle.Cell{X: x, Y: y}
			t.Run(fmt.Sprintf("%d,%d", x, y), func(t *testing.T) {
				var participants [][]byte
				if x != 4 || y != 2 {
					kind, id, owner := byte(clientbattle.Player), uint32(10002), uint32(0)
					if x == 3 {
						kind, id, owner = clientbattle.Pet, 14162, 10002
					}
					participants = append(participants, remoteRecord(5, kind, x, y, id, owner))
				}
				c, _, sent := enterBattle(t, participants...)
				c.items[32003] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32003, Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{180, 120}}}
				c.InventoryState.Bag[0] = game.Item{ID: 32003, Count: 2}
				c.chooseBattleCommand(battleCommandItems)
				c.Inventory.Use(1, false)
				at := cell.Position()
				c.GroundClick(at.X, at.Y-30)
				want := []byte{50, 2, 4, 2, x, y, 3, 125, 0, 0, 1, 0}
				if got := sent(); len(got) != 1 || !bytes.Equal(got[0], want) {
					t.Fatal("teammate click did not submit item", got)
				}
			})
		}
	}
}

func TestBattleItemBombTargetsEnemy(t *testing.T) {
	c, _, sent := enterBattle(t)
	// Native Bomb (33012): HP value 40 encodes -60, linked motion 10013.
	it := assets.NativeItem{Definition: game.ItemDefinition{ID: 33012, Type: 24, EquipSlot: 8, Status: [2]uint16{25, 0}, Values: [2]int32{40, 100}}}
	it.Record[134], it.Record[135] = 29, 39
	c.items[33012] = it
	c.SkillState.Catalog.Definitions[10013] = skills.Definition{Skill: assets.Skill{ID: 10013}, TargetKnown: true, NativeTarget: skills.TargetEnemy}
	c.InventoryState.Bag[0] = game.Item{ID: 33012, Count: 1}
	c.chooseBattleCommand(battleCommandItems)
	c.Inventory.Use(1, false)
	ally := (clientbattle.Cell{X: 4, Y: 2}).Position()
	c.GroundClick(ally.X, ally.Y-30)
	if len(sent()) != 0 || c.battle.item != nil {
		t.Fatal("invalid bomb target did not cancel selection")
	}
	c.chooseBattleCommand(battleCommandItems)
	c.Inventory.Use(1, false)
	enemy := (clientbattle.Cell{X: 2, Y: 2}).Position()
	c.GroundClick(enemy.X, enemy.Y-30)
	want := []byte{50, 2, 4, 2, 2, 2, 244, 128, 0, 0, 1, 0}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatal("bomb enemy choice", got)
	}
}

func TestBattleItemCompiledNativeMotions(t *testing.T) {
	bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if bundle == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE for native recovery motions")
	}
	c, _, _ := enterBattle(t)
	useCompiledPetAssets(t, c, bundle)
	loaded, err := clientbattle.Load(c.Assets)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint16{10024, 10025, 15138} {
		m, ok := loaded.Motions[id]
		if !ok || len(m.Frames) == 0 || m.Duration() <= 0 {
			t.Fatal("missing native item motion", id)
		}
	}
}

func TestBattleItemMixedRoundChoices(t *testing.T) {
	c, sent := clientItemFixture(t)
	c.combat.enabled, c.combat.choiceToken = true, 9
	c.chooseBattleCommand(battleCommandItems)
	c.Inventory.Use(1, false)
	for _, f := range c.remote.battle.fighters {
		if f.x == 4 {
			c.submitBattleItem(f)
		}
	}
	if len(sent()) != 0 || len(c.combat.choices) != 1 {
		t.Fatal("item choice not buffered")
	}
	c.chooseBattleCommand(battleCommandDefend)
	want := [][]byte{{50, 2, 4, 2, 4, 2, 3, 125, 0, 0, 1, 0}, {50, 4, 3, 2, 3, 2, 117, 234}}
	got := sent()
	if len(got) != 1 {
		t.Fatal("expected one choice batch", got)
	}
	token, choices, err := protocol.DecodeCombatChoices(got[0])
	if err != nil || token != 9 || !reflect.DeepEqual(choices, want) {
		t.Fatal("mixed item/defense batch", choices, err)
	}
}

func TestBattleItemPlayerAndPetSubmitBothChoices(t *testing.T) {
	for _, chain := range []bool{false, true} {
		t.Run(fmt.Sprint(chain), func(t *testing.T) {
			c, sent := clientItemFixture(t)
			c.combat.enabled, c.combat.choiceToken = chain, 9
			c.items[32176] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32176, Name: "Spicy Hot Pot", Type: 23, EquipSlot: 8, Status: [2]uint16{25, 26}, Values: [2]int32{400, 400}}}
			c.InventoryState.Bag[0] = game.Item{ID: 32176, Count: 50}
			for _, cell := range []clientbattle.Cell{{X: 4, Y: 2}, {X: 3, Y: 2}} {
				c.chooseBattleCommand(battleCommandItems)
				c.Inventory.Use(1, false)
				if c.battle.item == nil || c.battle.item.actor != cell {
					t.Fatal("item actor", cell, c.battle.item)
				}
				at := (clientbattle.Cell{X: 3, Y: 2}).Position()
				c.GroundClick(at.X, at.Y-30)
				if !chain {
					// Native server receipts arrive between the two selections.
					c.dispatch([]byte{53, 5, cell.X, cell.Y})
				}
			}
			want := [][]byte{{50, 2, 4, 2, 3, 2, 176, 125, 0, 0, 1, 0}, {50, 2, 3, 2, 3, 2, 176, 125, 0, 0, 1, 0}}
			got := sent()
			if chain {
				if len(got) != 1 {
					t.Fatal("two item choices not sent as a batch", got)
				}
				_, got, _ = protocol.DecodeCombatChoices(got[0])
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatal("two item choices", got)
			}
		})
	}
}

func TestBattleItemCancelStaleAndUnsupported(t *testing.T) {
	for _, test := range []string{"escape", "changed item", "locked", "equipment", "animation", "timeout", "hotbar"} {
		t.Run(test, func(t *testing.T) {
			c, sent := clientItemFixture(t)
			c.Inventory.Show()
			switch test {
			case "locked":
				c.InventoryState.Bag[0].Locked = true
			case "equipment":
				c.items[32003] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32003, EquipSlot: 6}}
			case "animation":
				c.battle.state.Actions = []clientbattle.Action{{Actor: clientbattle.Cell{X: 4, Y: 2}}}
			}
			if test == "hotbar" {
				c.useBinding(hud.Binding{Kind: protocol.HotbarItem, ID: 32003})
			} else {
				c.Inventory.Use(1, false)
			}
			switch test {
			case "locked", "equipment", "animation":
				if c.battle.item != nil {
					t.Fatal("unusable item selected")
				}
			case "escape":
				if !c.HotbarKey(27, 0) || c.battle.item != nil {
					t.Fatal("item target not cancelled")
				}
			case "changed item":
				c.InventoryState.Bag[0].Count--
				for _, f := range c.remote.battle.fighters {
					if f.x == 4 {
						c.submitBattleItem(f)
					}
				}
				if c.battle.item != nil {
					t.Fatal("stale target retained")
				}
			case "timeout":
				c.dispatch([]byte{50, 6, 4, 2, 0})
				c.dispatch([]byte{52, 1})
				if c.battle.item != nil {
					t.Fatal("item selection survived turn")
				}
			case "hotbar":
				if c.battle.item == nil {
					t.Fatal("item shortcut ignored in battle")
				}
			}
			if len(sent()) != 0 {
				t.Fatal("cancelled/unusable selection sent packets", sent())
			}
		})
	}
}
