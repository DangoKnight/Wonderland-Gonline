package app

import (
	"encoding/binary"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/internal/protocol"
)

// Make Friends is the toolbar's button 6 (handler 0x295820, case
// 0x295e20): it arms target mode 5 (FUN_0049bbcc) with cursor 5; the next
// click on a player (FUN_004112f8 case 2 → FUN_003c1628) sends 14/2 with
// their ID, and any click on something ends the mode (FUN_00496ca4). The
// asked player answers through the question box (FUN_003bfb44): Yes
// (0x3bfc28) sends 14/3 result 1, No (FUN_003bfcb4) result 2 with
// wav0151, and the 10-second timeout (FUN_003c23e4) result 3. 14/3 back
// (dispatcher case 0xe/3) reports the answer; result 1 adds the friend
// from the player in view (FUN_003c1d44).
const (
	makeFriendsButton = 6
	// The toolbar handler's map checks.
	mapNoToolbarA      = 10085 // 0x2765
	mapNoToolbarB      = 12766 // 0x31de
	mapNoTeamOrFriends = 10701 // 0x29cd, Join Team and Make Friends
	mapNoMakeFriends   = 10072 // 0x2758
	friendListLimit    = 50    // PTR_DAT_004caa70, without the GM entry
	friendAskTimeout   = 10 * time.Second

	friendAnswerAccept = 1
	friendAnswerRefuse = 2
	friendAnswerIgnore = 3

	friendRefuseSound = `sound\wav0151.wav`

	noticeNotHere        = "Not available here"
	noticeTemporarilyOff = "Temporary unavailable"
	noticeAlreadyFriend  = "Already your friendW"
	noticeTooManyFriends = "Too many friends in list"
	friendAsks           = " wants to add you as a friend"
	friendNoticePrefix   = "Player "
	friendAccepted       = " friend accepted"
	friendRefused        = " refused to add"
	friendIgnored        = " ignored"
	friendBusy           = "Player busy"

	noticeHereDuration = 2000 * time.Millisecond
	noticeTempDuration = 1200 * time.Millisecond
	noticeFriendsHere  = 1000 * time.Millisecond
	noticeFriendAnswer = 2000 * time.Millisecond
)

type friendAsk struct {
	id      uint32
	until   time.Time
	dismiss func()
}

func (c *Client) friendNotice(text string, d time.Duration) {
	c.Notices.Show([]byte(text), d, c.Now())
}

// toolbarBlocked is the handler's map checks for button tag.
func (c *Client) toolbarBlocked(tag int) bool {
	if c.World == nil {
		return true
	}
	switch m := c.World.Player.Map; {
	case m == mapNoToolbarA || m == mapNoToolbarB:
		c.friendNotice(noticeNotHere, noticeHereDuration)
		return true
	case (tag == joinTeamButton || tag == makeFriendsButton) && m == mapNoTeamOrFriends:
		c.friendNotice(noticeTemporarilyOff, noticeTempDuration)
		return true
	}
	return false
}

// makeFriends is case 0x295e20.
func (c *Client) makeFriends() {
	if c.toolbarBlocked(makeFriendsButton) || !c.Inventory.CanAct() || c.UI.Modal != nil {
		return
	}
	if c.World.Player.Map == mapNoMakeFriends {
		c.friendNotice(noticeNotHere, noticeFriendsHere)
		return
	}
	if c.Social == nil || len(c.Social.Friends)-1 >= friendListLimit {
		return
	}
	c.setJoinTeamTarget(false)
	c.setTradeTarget(false)
	c.World.StopWalk()
	c.groundHeld = false
	c.setFriendTarget(true)
}

func (c *Client) setFriendTarget(active bool) {
	c.friendTarget = active
	c.syncTargetSelection()
}

// friendTargetAt is FUN_003c1628 for a click in mode 5.
func (c *Client) friendTargetAt(x, y int) bool {
	if !c.friendTarget {
		return false
	}
	if c.World == nil {
		c.setFriendTarget(false)
		return true
	}
	// An empty click keeps the mode, as Join Team's does (the original
	// ends it only on a click on something, FUN_00496ca4).
	id := c.World.TeamTargetAt(x, y)
	if id == 0 {
		return true
	}
	c.setFriendTarget(false)
	if _, ok := c.friend(id); ok {
		c.friendNotice(noticeAlreadyFriend, noticeFriendAnswer)
		return true
	}
	c.Net.Send(protocol.Builder{protocol.CommandFriends, protocol.FriendsRequest}.U32(id))
	return true
}

// askFriend is 14/2 (FUN_003bfb44): the question names the player.
func (c *Client) askFriend(p []byte) bool {
	if len(p) != 5 || c.Team == nil {
		return false
	}
	id := binary.LittleEndian.Uint32(p[1:])
	if c.sport != nil {
		// A minigame (PTR_DAT_004c9994 +4) refuses at once.
		c.answerFriend(id, friendAnswerRefuse)
		return true
	}
	if c.friendAsk != nil && c.friendAsk.dismiss != nil {
		c.friendAsk.dismiss()
	}
	name := c.peerName(id)
	ask := &friendAsk{id: id, until: c.Now().Add(friendAskTimeout)}
	c.friendAsk = ask
	ask.dismiss = c.Team.Confirm(string(name)+friendAsks, 0, func() {
		c.friendAsk = nil
		if c.Social != nil && len(c.Social.Friends)-1 >= friendListLimit {
			c.friendNotice(noticeTooManyFriends, noticeFriendAnswer)
			c.answerFriend(id, friendAnswerRefuse)
			return
		}
		c.answerFriend(id, friendAnswerAccept)
	}, func() {
		c.friendAsk = nil
		c.answerFriend(id, friendAnswerRefuse)
	})
	return true
}

func (c *Client) answerFriend(id uint32, answer byte) {
	if answer == friendAnswerRefuse {
		c.playSound(friendRefuseSound)
	}
	c.Net.Send(protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(id).U8(answer))
}

// friendAskTick is the question box's timeout.
func (c *Client) friendAskTick(now time.Time) {
	a := c.friendAsk
	if a == nil || now.Before(a.until) {
		return
	}
	c.friendAsk = nil
	if a.dismiss != nil {
		a.dismiss()
	}
	c.answerFriend(a.id, friendAnswerIgnore)
}

// friendAnswered is 14/3 (dispatcher case 0xe/3).
func (c *Client) friendAnswered(p []byte) bool {
	if len(p) != 6 {
		return false
	}
	id := binary.LittleEndian.Uint32(p[1:])
	name := string(c.peerName(id))
	switch p[5] {
	case friendAnswerAccept:
		c.addFriend(id)
		c.friendNotice(friendNoticePrefix+name+friendAccepted, noticeFriendAnswer)
	case friendAnswerRefuse:
		c.friendNotice(friendNoticePrefix+name+friendRefused, noticeFriendAnswer)
	case friendAnswerIgnore:
		c.friendNotice(friendNoticePrefix+name+friendIgnored, noticeFriendAnswer)
	default:
		c.friendNotice(friendNoticePrefix+name+friendBusy, noticeFriendAnswer)
	}
	return true
}

// addFriend is FUN_003c1d44: the record comes from the player in view.
func (c *Client) addFriend(id uint32) {
	if c.Social == nil || id == mailSystemSenderAlt {
		return
	}
	if _, ok := c.friend(id); ok {
		return
	}
	f := hud.Friend{ID: id}
	if c.World != nil {
		if p := c.World.Peers[id]; p != nil {
			f.Name, f.Nickname = append([]byte(nil), p.Name...), append([]byte(nil), p.Nickname...)
			f.Level, f.Element = p.Level, p.Element
			f.Body, f.Head, f.Color1, f.Color2 = byte(p.Body), byte(p.Head), p.Color1, p.Color2
		}
	}
	c.Social.SetFriends(append(append([]hud.Friend(nil), c.Social.Friends[1:]...), f))
}

func (c *Client) peerName(id uint32) []byte {
	if c.World == nil {
		return nil
	}
	return c.World.PeerName(id)
}
