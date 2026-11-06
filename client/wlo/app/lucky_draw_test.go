package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/lottery"
	"wonderland-gonline/client/wlo/seui"
)

func TestLuckyDrawToolbarSpinAndResult(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.LuckyDraw
	c.dispatch([]byte{104, 1, 1, 0, 2, 176, 125, 2, 216, 132, 3})
	if f.Visible || !f.State.Ready || f.State.Remaining() != 3 {
		t.Fatal("catalog opened a window or lost allowance")
	}
	c.FuncButtons.Buttons[10].Click()
	if !f.Visible || len(sent()) != 0 {
		t.Fatal("toolbar did not open locally")
	}
	f.RewardIcon.Click()
	if !f.Preview.Visible {
		t.Fatal("reward preview missing")
	}
	if !c.LuckyDrawKey(27) || f.Preview.Visible || !f.Visible {
		t.Fatal("escape must close preview first")
	}
	f.Spin.Click()
	f.Spin.Click()
	*now = now.Add(2799 * time.Millisecond)
	c.luckyDrawTick()
	if len(sent()) != 0 || f.State.Used != 0 {
		t.Fatal("draw sent before native animation ended")
	}
	*now = now.Add(time.Millisecond)
	c.frame(false)
	if packets := sent(); len(packets) != 1 || !bytes.Equal(packets[0], []byte{104, 1}) || !f.State.Pending {
		t.Fatal(packets)
	}
	f.Close.Click()
	c.FuncButtons.Buttons[10].Click()
	f.Spin.Click()
	if len(sent()) != 1 || !f.State.Pending {
		t.Fatal("reopening duplicated a pending draw")
	}
	bag := c.InventoryState.Bag
	c.dispatch([]byte{104, 1, 2, 2, 1})
	if f.State.Slot != 2 || f.State.Remaining() != 2 || f.State.Busy() || c.InventoryState.Bag != bag {
		t.Fatal("result must update presentation only")
	}
	addition := append([]byte{23, 5, 1, 216, 132, 3, 0}, make([]byte, 26)...)
	c.dispatch(addition)
	if it := c.InventoryState.Bag[0]; it.ID != 34008 || it.Count != 3 {
		t.Fatal("ordinary inventory receipt not applied", it)
	}
	if dir := os.Getenv("SNAPSHOT_DIR"); dir != "" {
		c.Frame()
		save(t, c, filepath.Join(dir, "lucky_draw_result.png"))
		catalog := []byte{104, 1, 1, 1, 20}
		for i := 0; i < 20; i++ {
			catalog = append(catalog, 216, 132, byte(i+1))
		}
		c.dispatch(catalog)
		f.RewardIcon.Click()
		c.Frame()
		save(t, c, filepath.Join(dir, "lucky_draw_rewards.png"))
	}
	if !c.LuckyDrawKey(27) {
		t.Fatal("draw escape not handled")
	}
}

func TestLuckyDrawRefusalTimeoutAndReset(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	f := c.LuckyDraw
	c.dispatch([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	c.toggleLuckyDraw()
	c.startLuckyDraw()
	*now = now.Add(lottery.SpinDuration)
	c.luckyDrawTick()
	c.dispatch(append([]byte{2, 16, 0, 0, 0, 0}, []byte("Unrelated world notice")...))
	if !f.State.Pending {
		t.Fatal("unrelated banner released pending draw")
	}
	c.dispatch(append([]byte{2, 16, 0, 0, 0, 0}, []byte("Please free an inventory slot. Lucky Draw retained.")...))
	if f.State.Busy() || f.State.Used != 0 || !f.Spin.Enabled {
		t.Fatal("refused draw consumed allowance or remained stuck")
	}
	c.startLuckyDraw()
	*now = now.Add(lottery.SpinDuration)
	c.luckyDrawTick()
	*now = now.Add(lottery.ReplyTimeout)
	c.frame(false)
	if len(sent()) != 2 || f.State.Busy() || f.State.Used != 0 {
		t.Fatal("missing response retried or consumed allowance")
	}
	c.dispatch([]byte{104, 1, 1, 3, 1, 176, 125, 2})
	if f.Spin.Enabled || f.State.Remaining() != 0 {
		t.Fatal("daily limit ignored")
	}
	c.dispatch([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	if !f.Spin.Enabled {
		t.Fatal("server midnight refresh did not enable draw")
	}
	c.startLuckyDraw()
	c.disconnected()
	if f.Visible || f.Preview.Visible || f.State.Ready || f.State.Busy() {
		t.Fatal("disconnect leaked draw state")
	}
}

func TestLuckyDrawInteractionGates(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.dispatch([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	for _, mode := range []string{"loading", "held", "event", "battle", "remote", "trade", "modal", "movie", "sport", "frozen"} {
		t.Run(mode, func(t *testing.T) {
			c.LuckyDraw.Show()
			switch mode {
			case "loading":
				c.mapReady = false
			case "held":
				c.held = true
			case "event":
				c.event.active = true
			case "battle":
				c.battle.state.Active = true
			case "remote":
				c.remote.battle.active = true
			case "trade":
				c.Trade.Partner = 100
			case "modal":
				c.UI.Modal = seui.NewForm(c.Env)
			case "movie":
				c.movie = &moviePlay{}
			case "sport":
				c.sport = &sportPlay{}
			case "frozen":
				c.fade.step = 1
			}
			c.startLuckyDraw()
			c.luckyDrawTick()
			if c.LuckyDraw.State.Busy() || len(sent()) != 0 {
				t.Fatal("blocked draw started")
			}
			c.mapReady, c.held, c.event.active = true, false, false
			c.battle.state.Active, c.remote.battle.active, c.Trade.Partner = false, false, 0
			c.UI.Modal, c.movie, c.sport, c.fade.step = nil, nil, nil, 0
		})
	}
	c.LuckyDraw.Show()
	c.startLuckyDraw()
	c.event.active = true
	*now = now.Add(3 * time.Second)
	c.luckyDrawTick()
	if c.LuckyDraw.State.Busy() || len(sent()) != 0 {
		t.Fatal("animation sent after an interaction began")
	}
}

func TestLuckyDrawSendFailureAndMalformedDispatch(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.mapReady = true
	var rejected int
	c.Unhandled = func([]byte) { rejected++ }
	c.dispatch([]byte{104, 1, 1, 0, 1, 176, 125, 2})
	c.dispatch([]byte{104, 1, 2, 0, 1})
	if rejected != 1 || c.LuckyDraw.State.Used != 0 {
		t.Fatal("invalid result changed usage")
	}
	c.toggleLuckyDraw()
	c.startLuckyDraw()
	*now = now.Add(lottery.SpinDuration)
	c.luckyDrawTick() // The fixture has no connected socket.
	if c.LuckyDraw.State.Busy() || c.LuckyDraw.State.Used != 0 {
		t.Fatal("send failure consumed a draw or remained pending")
	}
	c.resetInventory(c.World.Player)
	if c.LuckyDraw.State.Ready || c.LuckyDraw.Visible {
		t.Fatal("character entry retained old catalog")
	}
}
