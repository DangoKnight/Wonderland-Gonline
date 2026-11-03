package app

import (
	"fmt"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/lottery"
	"wonderland-gonline/internal/protocol"
)

const (
	luckyDrawButton         = 10
	luckyDrawNoticeDuration = 3 * time.Second
	luckyDrawInventoryFull  = "Please free an inventory slot. Lucky Draw retained."
	luckyDrawUnavailable    = "Lucky Draw rewards are unavailable."
	luckyDrawDailyLimit     = "You have used all three Lucky Draws for today."
)

func (c *Client) initLuckyDraw() {
	f := lottery.NewForm(c.Env)
	c.LuckyDraw = f
	f.CanDraw, f.OnSpin = c.canLuckyDraw, c.startLuckyDraw
	f.ItemName = func(id uint16) string {
		if name := c.items[id].Definition.Name; name != "" {
			return name
		}
		return fmt.Sprintf("Item %d", id)
	}
	f.DrawItem = func(reward lottery.Reward, x, y int) {
		picture := "Icon_UnKnow"
		if icon := c.items[reward.ID].Icon; icon != 0 {
			picture = strconv.Itoa(int(icon))
		}
		c.Pics.Draw(c.Screen, c.Pics.Find(picture), x, y, true)
		if reward.Quantity > 1 {
			c.Env.Text.Draw(x, y+19, 0, false, true, c.Screen, []byte(strconv.Itoa(int(reward.Quantity))), 13, 32, 0, skinColor, 0)
		}
	}
	c.UI.Add(f)
	c.UI.Add(f.Preview)
	c.FuncButtons.Buttons[luckyDrawButton].OnClick = c.toggleLuckyDraw
	c.FuncButtons.Buttons[luckyDrawButton].Now = func() time.Time { return c.Now() }
}

func (c *Client) canLuckyDraw() bool {
	return c.World != nil && c.Inventory.CanAct() && c.UI.Modal == nil &&
		!c.battle.state.Active && !c.remote.battle.active && c.Trade.Partner == 0 && !c.Talk.Drawn()
}

func (c *Client) toggleLuckyDraw() {
	if c.LuckyDraw.Visible {
		c.LuckyDraw.Hide()
		return
	}
	if !c.canLuckyDraw() || c.toolbarBlocked(luckyDrawButton) {
		return
	}
	c.LuckyDraw.Show()
}

func (c *Client) startLuckyDraw() {
	if !c.LuckyDraw.Visible || !c.canLuckyDraw() || c.toolbarBlocked(luckyDrawButton) {
		return
	}
	c.LuckyDraw.State.Start(c.Now())
	c.LuckyDraw.Refresh()
}

// Animation and timeout progress even when this session's preview is not drawn.
func (c *Client) luckyDrawTick() {
	f := c.LuckyDraw
	if f == nil {
		return
	}
	if !c.canLuckyDraw() && (f.Visible || f.Preview.Visible || f.State.Rolling) {
		f.Hide()
	}
	send, timedOut := f.State.Advance(c.Now())
	if send {
		if err := c.Net.Send([]byte{protocol.CommandLuckyDraw, protocol.LuckyDrawSpin}); err != nil {
			f.State.Fail()
			c.Notices.Show([]byte(err.Error()), luckyDrawNoticeDuration, c.Now())
		}
	}
	if timedOut {
		c.Notices.Show([]byte("Lucky Draw result has not arrived. Check your inventory before trying again."), luckyDrawNoticeDuration, c.Now())
	}
	f.Refresh()
	c.FuncButtons.Buttons[luckyDrawButton].Animating = f.State.Ready && f.State.Remaining() > 0 && len(f.State.Rewards) > 0
}

func (c *Client) luckyDrawPacket(p []byte) {
	if c.World == nil || !c.LuckyDraw.State.Apply(p) {
		if c.Unhandled != nil {
			c.Unhandled(p)
		}
		return
	}
	for _, reward := range c.LuckyDraw.State.Rewards {
		if icon := c.items[reward.ID].Icon; icon != 0 {
			c.loadPictures(strconv.Itoa(int(icon)))
		}
	}
	if c.LuckyDraw.Preview.Visible {
		c.LuckyDraw.Preview.Open()
	}
	c.LuckyDraw.Refresh()
}

// AC104 has no failure result. These existing server head banners release a
// pending request without changing its allowance. Unrelated banners do not.
func (c *Client) luckyDrawNotice(text []byte) {
	if c.LuckyDraw == nil || !c.LuckyDraw.State.Pending {
		return
	}
	switch string(text) {
	case luckyDrawInventoryFull, luckyDrawUnavailable, luckyDrawDailyLimit:
		c.LuckyDraw.State.Fail()
		c.LuckyDraw.Refresh()
	}
}

func (c *Client) LuckyDrawKey(key uint16) bool {
	if key != escapeKey || c.LuckyDraw == nil || c.UI.Modal != nil {
		return false
	}
	if c.LuckyDraw.Preview.Visible {
		c.LuckyDraw.Preview.Hide()
		return true
	}
	if c.LuckyDraw.Visible {
		c.LuckyDraw.Hide()
		return true
	}
	return false
}
