package app

import "wonderland-gonline/internal/localization"

import (
	"encoding/binary"
	"strconv"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/protocol"
)

// Trading items. The toolbar's button 7 (handler 0x295820, case 0x295a25)
// shows the Business menu (hud.Business); its Trade Items (0x25bdf4) arms
// target mode 9 (FUN_0049bbcc) with cursor 2, and a click on a player
// (FUN_003c674c, TTradeManage at PTR_DAT_004ca8d0) sends 25/1 with their
// ID. The server opens both trade forms with 25/1 (FUN_003c6b18), relays
// each locked offer as 25/3 and ends the trade with 25/2.
const (
	tradeButton    = 7
	tradeMinLevel  = 25    // FUN_004166e4(…, 0x28), the level with rebirths
	mapNoTradeA    = 12064 // 0x2f20, "Can't trade here"
	mapNoTradeB    = 11073 // 0x2b41
	mapNoBusinessA = 10079 // 0x275f, FUN_004851a4
	mapNoBusinessB = 10095 // 0x276f

	noticeTradeHere      = "ui.app.can_t_trade_here_4570ba5e"
	noticeRequiresLevel  = "ui.app.requires_player_lv25_d0cc1c20"
	noticeOnlineTime     = "ui.app.not_enough_online_time_to_use_3c5ebb44"
	noticeTargetLevel    = "ui.app.player_is_below_lv25_b9ded6ee"
	tradeTargetNotice    = 1500 * time.Millisecond
	tradeHereNotice      = 1200 * time.Millisecond
	tradeBusinessNotice  = 2000 * time.Millisecond
	tradeUnavailableHere = 1200 * time.Millisecond
)

// noBusinessMap is FUN_004851a4 or FUN_00485160: 10079, 10095, 10096,
// 10097 and 10099 to 10103.
func noBusinessMap(m uint16) bool {
	return m == mapNoBusinessA || m == mapNoBusinessB || m-10096 < 2 || m-10099 < 5
}

func (c *Client) initTrade() {
	c.Business = hud.NewBusiness(c.Env)
	c.Business.Gold = func() uint32 { return c.Stats.Gold }
	c.Business.OnTradeItems = c.tradeItems
	c.UI.Add(c.Business)
	c.Trade = inventory.NewTradeForm(c.Env, c.Inventory)
	c.Trade.Send = func(p []byte) { c.Net.Send(p) }
	c.Trade.Notice = func(s string, d time.Duration) { c.Notices.Show([]byte(s), d, c.Now()) }
	c.Trade.Busy = func() bool { return c.sport != nil }
	c.Trade.PlayerGold = func() uint32 { return c.Stats.Gold }
	c.UI.Add(c.Trade)
	c.FuncButtons.Buttons[tradeButton].OnClick = c.toggleBusiness
}

// toggleBusiness is case 0x295a25.
func (c *Client) toggleBusiness() {
	if c.toolbarBlocked(tradeButton) {
		return
	}
	if c.World.Player.Map == mapNoMakeFriends {
		c.friendNotice(localization.UI(noticeNotHere), noticeFriendsHere)
		return
	}
	c.Business.Toggle()
}

// tradeItems is Trade Items (0x25bdf4).
func (c *Client) tradeItems() {
	if c.World == nil {
		return
	}
	m := c.World.Player.Map
	switch {
	case c.Stats.EffectiveLevel() < tradeMinLevel:
		c.friendNotice(localization.UI(noticeRequiresLevel), tradeBusinessNotice)
	case !c.tradeAllowed:
		c.friendNotice(localization.UI(noticeOnlineTime), tradeBusinessNotice)
	case m == mapNoTradeA || m == mapNoTradeB:
		c.friendNotice(localization.UI(noticeTradeHere), tradeBusinessNotice)
	case noBusinessMap(m):
		c.friendNotice(localization.UI(noticeNotHere), tradeUnavailableHere)
	case m == mapNoTeamOrFriends:
		c.friendNotice(localization.UI(noticeTemporarilyOff), tradeUnavailableHere)
	default:
		c.setJoinTeamTarget(false)
		c.setFriendTarget(false)
		c.World.StopWalk()
		c.resetGroundHold()
		c.setTradeTarget(true)
		c.Business.Hide()
	}
}

// resetTrade forgets the trade state at world entry and disconnection;
// the server ends any trade on its side.
func (c *Client) resetTrade() {
	c.setTradeTarget(false)
	c.tradeAllowed = false
	c.Business.Hide()
	c.Trade.Reset()
}

func (c *Client) setTradeTarget(active bool) {
	if active {
		c.setPKTarget(false)
	}
	c.tradeTarget = active
	c.syncTargetSelection()
}

// syncTargetSelection lights hovered players while a player target mode
// (PK, Join Team, Make Friends, Trade) is armed.
func (c *Client) syncTargetSelection() {
	if c.World != nil {
		c.World.JoinTeamSelection = c.pkTarget || c.joinTeamTarget || c.friendTarget || c.tradeTarget
		c.World.Hover(-1, -1)
	}
}

// tradeTargetAt is FUN_003c674c for a click in mode 9.
func (c *Client) tradeTargetAt(x, y int) bool {
	if !c.tradeTarget {
		return false
	}
	if c.World == nil {
		c.setTradeTarget(false)
		return true
	}
	id := c.World.TeamTargetAt(x, y)
	c.setTradeTarget(false)
	if id == 0 {
		return true
	}
	switch m := c.World.Player.Map; {
	case m == mapNoBusinessA || m == mapNoBusinessB:
		c.friendNotice(localization.UI(noticeTradeHere), tradeHereNotice)
	case c.sport != nil:
		c.Trade.Close()
	default:
		if p := c.World.Peers[id]; p != nil && p.EffectiveLevel() < tradeMinLevel {
			c.friendNotice(localization.UI(noticeTargetLevel), tradeTargetNotice)
			return true
		}
		c.Net.Send(protocol.Builder{protocol.CommandTrade, protocol.TradeRequest}.U32(id))
	}
	return true
}

// tradePacket is AC25's ported part (dispatcher cases 0x2e92e6..0x2e930e
// and 0x2c).
func (c *Client) tradePacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandTrade || c.Trade == nil {
		return false
	}
	switch p[1] {
	case protocol.TradeRequest:
		if len(p) != 6 {
			return false
		}
		// FUN_003c6b18: a minigame refuses; otherwise the form opens for a
		// player in view (FUN_00367d80).
		if c.sport != nil {
			c.Trade.Close()
			return true
		}
		id := binary.LittleEndian.Uint32(p[2:])
		if c.World != nil {
			if peer := c.World.Peers[id]; peer != nil {
				c.Trade.Open(id, peer.Name)
			}
		}
		return true
	case protocol.TradeEnded:
		if len(p) != 3 {
			return false
		}
		c.Trade.Ended(p[2])
		return true
	case protocol.TradePartnerOffer:
		if !c.Trade.PartnerOffer(p) {
			return false
		}
		for _, cell := range c.Trade.Theirs {
			if icon := c.items[cell.Item().ID].Icon; icon != 0 {
				c.loadPictures(strconv.Itoa(int(icon)))
			}
		}
		return true
	case protocol.TradeOnlineTime:
		if len(p) != 3 {
			return false
		}
		switch p[2] {
		case protocol.TradeOnlineTimeShort:
			c.tradeAllowed = false
		case protocol.TradeOnlineTimeEnough:
			c.tradeAllowed = true
		}
		return true
	}
	return false
}
