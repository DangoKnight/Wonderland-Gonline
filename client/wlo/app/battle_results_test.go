package app

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func resultsClient(t *testing.T) (*Client, *time.Time) {
	t.Helper()
	c, now, _ := enteredClient(t)
	if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
		useCompiledPetAssets(t, c, bundle)
	}
	c.mapReady = true
	c.Stats.EXP, c.Stats.Level, c.Stats.Gold = 6, 1, 10
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 14162, Name: []byte("Roca"), Stats: world.Stats{EXP: 6, Level: 1}, Amity: 100}
	c.items[300] = assets.NativeItem{Definition: game.ItemDefinition{ID: 300, Name: "Pure Iron Ore"}}
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	finishSceneFade(c, now)
	c.battleFrame(false)
	c.Input.X, c.Input.Y = -1, -1
	return c, now
}

// Independent AC23:5 fixture: one 31-byte native additive bag record.
func resultItemPacket(slot byte, id uint16, count byte) []byte {
	p := make([]byte, 33)
	copy(p, []byte{23, 5, slot, byte(id), byte(id >> 8), count, 0})
	return p
}

func closeResultBattle(c *Client) {
	c.dispatch([]byte{11, 12, 1})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
}

func showResultAfterFade(c *Client, now *time.Time) {
	*now = now.Add(sceneFadeSteps * sceneFadeFrame)
	c.frame(false)
}

func TestBattleResultsConfirmedProgressAndItemsAfterMapFade(t *testing.T) {
	c, now := resultsClient(t)
	sent := wire(t, c)
	exp := []byte{8, 1, 36, 1, 81, 0, 0, 0, 0, 0, 0, 0}
	c.dispatch(exp)
	c.dispatch(exp) // Repeated absolute synchronization must not double EXP.
	c.dispatch([]byte{8, 1, 35, 1, 3, 0, 0, 0})
	c.dispatch([]byte{26, 4, 50, 0, 0, 0})
	c.dispatch(game.PetStat(1, 36, 81))
	c.dispatch(game.PetStat(1, 35, 3))
	c.dispatch(game.PetStat(1, 64, 99))
	c.dispatch(resultItemPacket(1, 300, 2))
	c.dispatch(resultItemPacket(1, 300, 3))
	c.Notices.Show([]byte("Obtain Pure Iron Ore"), battleResultsDuration, *now)
	closeResultBattle(c)
	if c.battleResults.form != nil && c.battleResults.form.Visible {
		t.Fatal("results interrupted battle exit")
	}
	// Final post-close stat synchronization belongs to this battle too.
	c.dispatch(exp)
	showResultAfterFade(c, now)
	want := []string{"Dango EXP gained: 75", "Dango level up: 3", "Roca EXP gained: 75", "Roca level up: 3", "Roca Amity lost: 1", "Gold gained: 40", "Items obtained:", "Pure Iron Ore x5"}
	if !reflect.DeepEqual(c.battleResults.lines, want) || c.battleResults.form == nil || !c.battleResults.form.Visible {
		t.Fatal("results", c.battleResults.lines)
	}
	if c.InventoryState.Bag[0].Count != 5 || c.Stats.EXP != 81 || len(sent()) != 0 || c.UI.Modal != nil {
		t.Fatal("summary changed gameplay/sent requests")
	}
	if c.Notices.shown || len(c.Notices.items) != 0 {
		t.Fatal("earlier loot notice covers the battle results")
	}
	c.frame(false)
	if !reflect.DeepEqual(c.battleResults.lines, want) {
		t.Fatal("summary replayed")
	}
	if path := os.Getenv("BATTLE_RESULTS_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
}

func TestBattleResultsCompactEnglishReferenceNotice(t *testing.T) {
	c, now := resultsClient(t)
	c.dispatch([]byte{8, 1, 36, 1, 36, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch(game.PetStat(1, 36, 36))
	c.dispatch(game.PetStat(1, 35, 3))
	closeResultBattle(c)
	showResultAfterFade(c, now)
	want := []string{"Dango EXP gained: 30", "Roca EXP gained: 30", "Roca level up: 3"}
	f := c.battleResults.form
	if !reflect.DeepEqual(c.battleResults.lines, want) || f == nil || !f.Visible {
		t.Fatal("English reference result", c.battleResults.lines)
	}
	if f.Width >= 360 || f.Height > 120 || f.Left+f.Width/2 != ScreenWidth/2 || f.Top < 32 || f.Top > 40 || f.scroll.Visible || f.Draggable {
		t.Fatal("short notice is not compact and centered below the toolbar", f.Rect())
	}
	for _, child := range f.Children {
		if _, button := child.(*seui.FixedButton); button {
			t.Fatal("passive result notice has dialog buttons")
		}
	}
	if path := os.Getenv("BATTLE_RESULTS_REFERENCE_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
}

func TestBattleResultsIncludeFinalPostCloseReceipts(t *testing.T) {
	c, now := resultsClient(t)
	closeResultBattle(c)
	c.dispatch([]byte{8, 1, 36, 1, 16, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{26, 4, 15, 0, 0, 0})
	c.dispatch(resultItemPacket(1, 300, 1))
	showResultAfterFade(c, now)
	if !reflect.DeepEqual(c.battleResults.lines, []string{"Dango EXP gained: 10", "Gold gained: 5", "Items obtained:", "Pure Iron Ore x1"}) {
		t.Fatal("late settlement lost", c.battleResults.lines)
	}
	// Unrelated later rewards must not modify the immutable displayed summary.
	c.dispatch([]byte{8, 1, 36, 1, 26, 0, 0, 0, 0, 0, 0, 0})
	if c.battleResults.lines[0] != "Dango EXP gained: 10" {
		t.Fatal("later reward entered old summary")
	}
}

func TestBattleResultsReceiptsExcludeInvalidItemsAndConsumedStock(t *testing.T) {
	c, now := resultsClient(t)
	c.InventoryState.Bag[0] = game.Item{ID: 300, Count: 2}
	c.dispatch([]byte{23, 9, 1, 2})
	c.dispatch(resultItemPacket(1, 300, 2))
	c.dispatch(resultItemPacket(0, 300, 40)) // Malformed slot changes neither bag nor rewards.
	closeResultBattle(c)
	showResultAfterFade(c, now)
	if !reflect.DeepEqual(c.battleResults.lines, []string{"Items obtained:", "Pure Iron Ore x2"}) || c.InventoryState.Bag[0].Count != 2 {
		t.Fatal("predicted/incorrect items", c.battleResults.lines)
	}
}

func TestBattleResultsNoEmptyPopupAndLosses(t *testing.T) {
	for _, loss := range []bool{false, true} {
		t.Run(fmt.Sprint(loss), func(t *testing.T) {
			c, now := resultsClient(t)
			if loss {
				c.dispatch([]byte{8, 1, 36, 1, 1, 0, 0, 0, 0, 0, 0, 0})
				c.dispatch(game.PetStat(1, 64, 99))
			}
			c.dispatch([]byte{11, 12, 2})
			c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
			showResultAfterFade(c, now)
			if !loss && (len(c.battleResults.lines) != 0 || c.battleResults.form != nil) {
				t.Fatal("empty battle created reward popup")
			}
			if loss && !reflect.DeepEqual(c.battleResults.lines, []string{"Dango EXP lost: 5", "Roca Amity lost: 1"}) {
				t.Fatal("loss summary", c.battleResults.lines)
			}
		})
	}
}

func TestBattleResultsDesertionAndReusedPetSlot(t *testing.T) {
	c, now := resultsClient(t)
	c.dispatch(game.PetStat(1, 64, 99))
	c.dispatch(protocol.Builder{15, 2}.U32(10001).U8(1))
	if c.npcTemplates == nil {
		c.npcTemplates = map[uint32]world.NPCTemplate{}
	}
	c.npcTemplates[10501] = world.NPCTemplate{Name: "Slime"}
	pet := game.NewPet(10501, "Slime", 1, game.PetTemplate{}, nil)
	c.dispatch(pet.RecruitPacket(10001, game.PetTemplate{}))
	if len(c.Notices.items) == 0 || string(c.Notices.items[len(c.Notices.items)-1].text) != "Slime joins the party" {
		t.Fatal("missing immediate capture notification")
	}
	c.dispatch(game.PetStat(1, 36, 1000))
	closeResultBattle(c)
	showResultAfterFade(c, now)
	if !reflect.DeepEqual(c.battleResults.lines, []string{"Roca Amity lost: 1"}) {
		t.Fatal("replacement EXP assigned to departed pet", c.battleResults.lines)
	}
}

func TestBattleResultsLongListScrollAndExpiry(t *testing.T) {
	c, now := resultsClient(t)
	for i := 0; i < 20; i++ {
		id := uint16(300 + i)
		c.items[id] = assets.NativeItem{Definition: game.ItemDefinition{ID: id, Name: fmt.Sprintf("Material %02d", i)}}
		c.dispatch(resultItemPacket(byte(i+1), id, 1))
	}
	closeResultBattle(c)
	showResultAfterFade(c, now)
	f := c.battleResults.form
	if f == nil || !f.Visible || !f.scroll.Visible || len(f.list.Items) != 21 || f.Bottom() > ScreenHeight {
		t.Fatal("large list truncated or off-screen")
	}
	at := f.list.Abs()
	c.Input.X, c.Input.Y = at.X+8, at.Y+8
	c.Frame()
	if !c.UI.Wheel(false) || f.list.TopIndex != 1 {
		t.Fatal("centered notice lost wheel scrolling")
	}
	f.list.SetTop(len(f.list.Items) - f.list.RowsShown)
	if string(f.list.Items[len(f.list.Items)-1]) != "Material 19 x1" || f.list.TopIndex == 0 {
		t.Fatal("last reward inaccessible")
	}
	at = f.Abs()
	c.Input.X, c.Input.Y = at.X+40, at.Y+50
	*now = c.battleResults.until.Add(-time.Millisecond)
	c.frame(false)
	if !f.Visible {
		t.Fatal("results expired before the native six-second deadline")
	}
	*now = now.Add(time.Millisecond)
	c.frame(false)
	if f.Visible {
		t.Fatal("hover prolonged results beyond the native six-second deadline")
	}
}

func TestBattleResultsClicksDismissNoticeExceptScrollbar(t *testing.T) {
	for _, target := range []string{"panel", "text", "scrollbar"} {
		t.Run(target, func(t *testing.T) {
			c, now := resultsClient(t)
			for i := 0; i < 8; i++ {
				c.dispatch(resultItemPacket(byte(i+1), uint16(300+i), 1))
			}
			closeResultBattle(c)
			showResultAfterFade(c, now)
			f := c.battleResults.form
			at := f.Abs()
			x, y := at.X+f.Width/2, at.Y+12
			switch target {
			case "text":
				at = f.list.Abs()
				x, y = at.X+8, at.Y+8
			case "scrollbar":
				at = f.scroll.Abs()
				x, y = at.X+f.scroll.Width/2, at.Y+f.scroll.Height-2
			}
			c.Input.X, c.Input.Y = x, y
			c.Frame()
			if !c.UI.MouseDown(seui.ButtonLeft, 0, x, y) {
				t.Fatal("results click passed through to the world")
			}
			c.UI.MouseUp(seui.ButtonLeft, 0, x, y)
			if f.Visible != (target == "scrollbar") {
				t.Fatal("click did not dismiss results, or scrollbar dismissed them")
			}
			c.Frame()
			if f.Visible != (target == "scrollbar") {
				t.Fatal("result visibility changed after click")
			}
		})
	}
}

func TestBattleResultsDisconnectAndCharacterReset(t *testing.T) {
	t.Run("disconnect", func(t *testing.T) {
		c, now := resultsClient(t)
		c.dispatch(resultItemPacket(1, 300, 1))
		closeResultBattle(c)
		showResultAfterFade(c, now)
		f := c.battleResults.form
		c.disconnected()
		if c.battleResults.active || f.Visible || len(c.battleResults.lines) != 0 {
			t.Fatal("results survived disconnect")
		}
	})
	t.Run("character", func(t *testing.T) {
		c, now := resultsClient(t)
		c.dispatch(resultItemPacket(1, 300, 1))
		c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
		showResultAfterFade(c, now)
		if c.battleResults.active || len(c.battleResults.lines) != 0 {
			t.Fatal("results leaked into new character")
		}
	})
}

func TestGonlineCombatResultsWaitForQueuedSettlement(t *testing.T) {
	c, now := resultsClient(t)
	c.combat.enabled = true
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10001: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
	attack := []byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}
	deliverCombatRound(t, c, 2, []protocol.CombatStep{{Packets: [][]byte{attack}, Delay: time.Second}})
	c.dispatch([]byte{8, 1, 36, 1, 81, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch(resultItemPacket(1, 300, 3))
	closeResultBattle(c)
	c.frame(false)
	if c.battleResults.after.player.exp != 6 || c.battleResults.form != nil {
		t.Fatal("unplayed rewards shown early")
	}
	for i := 0; i < 60 && (c.battleResults.form == nil || !c.battleResults.form.Visible); i++ {
		*now = now.Add(100 * time.Millisecond)
		c.frame(false)
	}
	if c.battleResults.form == nil || !c.battleResults.form.Visible || strings.Join(c.battleResults.lines, ";") != "Dango EXP gained: 75;Items obtained:;Pure Iron Ore x3" {
		t.Fatal("queued results", c.battleResults.lines)
	}
}
