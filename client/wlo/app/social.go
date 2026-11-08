package app

import (
	"encoding/binary"
	"image"
	"time"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/internal/protocol"
)

// The Social window (hud.Social): the main menu's Social button, the
// friend table from 14/5 and the 14/4 removal.
const (
	mainSocialButton = 5
	// socialNotice is the notice board's duration for the window's
	// prompts (0x4b0 ms).
	socialNotice = 1200 * time.Millisecond
	// friendPortraitAction is the portrait's action (+0x121 = 1).
	friendPortraitAction = 1
	// gmFace is the face sprite FUN_002586c8 draws for a body-0 entry
	// such as the GM's (0x1f94 in the 008 archive).
	gmFace = 0x1f94
	// neutralColor is the colour value whose digits are all the neutral
	// 4; the capture shows the GM's face unrecoloured.
	neutralColor = 444444444
)

func (c *Client) newSocial(formula *login.Formula) {
	s := hud.NewSocial(c.Env)
	level := login.NewStatus(c.Env, formula)
	s.DrawLevel = func(v byte, x, y int) { level.Level(v, image.Pt(x, y), true) }
	s.DrawPortrait = c.drawFriendPortrait
	s.Presence = c.friendPresence
	s.Notice = func(text string) { c.Notices.Show([]byte(text), socialNotice, c.Now()) }
	s.Confirm = c.confirm
	s.Remove = func(id uint32) {
		c.Net.Send(protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(id))
	}
	c.wireEmotes(s)
	c.wireSetUp(s)
	c.Social = s
	c.UI.Add(s)
	c.newMailSend()
	c.newMailbox()
	c.MainButtons.Buttons[mainSocialButton].OnClick = func() {
		if s.Visible || (c.canOpenModule() && c.UI.Modal == nil) {
			s.Toggle()
		}
	}
}

// drawFriendPortrait is FUN_002586c8 for a row: a player's face, or the
// 008 face for a body-0 entry.
func (c *Client) drawFriendPortrait(f hud.Friend, x, y int) {
	if c.lib == nil {
		return
	}
	if f.Body == 0 {
		if c.gmFace == nil {
			c.gmFace = role.NewNPC(c.lib, 0, [4]uint32{neutralColor, neutralColor, neutralColor, neutralColor})
		}
		c.gmFace.DrawFace(c.Screen, x, y, gmFace, false)
		return
	}
	if c.friendFaces == nil {
		c.friendFaces = map[uint32]*role.Human{}
	}
	h := c.friendFaces[f.ID]
	if h == nil {
		h = role.NewHuman(c.lib, c.items)
		h.Now = func() time.Time { return c.Now() }
		c.friendFaces[f.ID] = h
	}
	h.SetCharacter(&login.CharacterSlot{Body: f.Body, Head: f.Head, Color1: f.Color1, Color2: f.Color2})
	h.DrawFace(c.Screen, x, y, friendPortraitAction, false)
}

// friendPresence is FUN_0027b5e8's presence for a row: a known player's
// status byte (registry +0x94), otherwise offline.
func (c *Client) friendPresence(id uint32) byte {
	if status, ok := c.friendStatuses[id]; ok {
		return status
	}
	if c.World != nil {
		if p := c.World.Peers[id]; p != nil && p.Presence <= hud.PresenceOffline {
			return p.Presence
		}
	}
	return hud.PresenceOffline
}

// receiveFriendList is 14/5 (FUN_002dde1c case 0xe/5): records of ID,
// name, six bytes (level, job, +0x17, element, body, head), two colours,
// nickname, guild and the VIP byte. IDs 100 and 101 restart the count,
// so they become the GM entry, which the client fills itself.
func decodeFriendRecords(data []byte, tabs bool) ([]hud.Friend, bool) {
	var list []hud.Friend
	r := protocol.NewReader(data)
	for r.Remaining() > 0 {
		var f hud.Friend
		f.ID = r.U32()
		f.Name = append([]byte(nil), r.Bytes(int(r.U8()))...)
		b := r.Bytes(6)
		f.Color1, f.Color2 = r.U32(), r.U32()
		f.Nickname = append([]byte(nil), r.Bytes(int(r.U8()))...)
		if !tabs {
			f.Guild = append([]byte(nil), r.Bytes(int(r.U8()))...)
			f.VIP = r.U8()
		}
		if r.Err() != nil {
			return nil, false
		}
		f.Level, f.Job, f.Field17, f.Element, f.Body, f.Head = b[0], b[1], b[2], b[3], b[4], b[5]
		if f.ID == hud.GMFriendID || f.ID == hud.GMFriendID+1 {
			list = list[:0]
			continue
		}
		list = append(list, f)
	}
	return list, true
}

func (c *Client) receiveFriendList(s []byte) bool {
	list, ok := decodeFriendRecords(s[1:], false)
	if !ok {
		return false
	}
	keep := map[uint32]bool{}
	for i := range list {
		keep[list[i].ID] = true
		if old, exists := c.friend(list[i].ID); exists {
			list[i].Mail = old.Mail
		}
	}
	for id := range c.friendStatuses {
		if !keep[id] {
			delete(c.friendStatuses, id)
		}
	}
	c.Social.SetFriends(list)
	return true
}

// Native AC14:8 refreshes an existing profile; AC14:11 upserts profiles.
func (c *Client) receiveFriendProfiles(p []byte) bool {
	list, ok := decodeFriendRecords(p[2:], p[1] == protocol.FriendsTabs)
	if !ok {
		return false
	}
	friends := append([]hud.Friend(nil), c.Social.Friends[1:]...)
	for _, f := range list {
		found := false
		for i, old := range friends {
			if old.ID == f.ID {
				f.Mail, f.Ticked = old.Mail, old.Ticked
				if p[1] == protocol.FriendsTabs {
					f.Guild, f.VIP = old.Guild, old.VIP
				}
				friends[i] = f
				found = true
				break
			}
		}
		if !found && p[1] == protocol.FriendsTabs {
			friends = append(friends, f)
		}
		c.rememberPlayer(f.ID, f.Name)
	}
	c.Social.SetFriends(friends)
	return true
}

func (c *Client) removeReceivedFriend(id uint32) {
	if id == hud.GMFriendID || id == mailSystemSenderAlt {
		return
	}
	var friends []hud.Friend
	for _, f := range c.Social.Friends[1:] {
		if f.ID != id {
			friends = append(friends, f)
		}
	}
	c.Social.SetFriends(friends)
	delete(c.friendStatuses, id)
	delete(c.friendFaces, id)
}

func (c *Client) receiveFriendPresence(p []byte) bool {
	const presenceRecordBytes = 5 // Native U32 character followed by status byte.
	if (len(p)-2)%presenceRecordBytes != 0 {
		return false
	}
	for at := 2; at < len(p); at += presenceRecordBytes {
		if p[at+4] > hud.PresenceOffline {
			return false
		}
	}
	if c.friendStatuses == nil {
		c.friendStatuses = map[uint32]byte{}
	}
	for at := 2; at < len(p); at += presenceRecordBytes {
		id := binary.LittleEndian.Uint32(p[at:])
		if _, ok := c.friend(id); ok {
			c.friendStatuses[id] = p[at+4]
		}
	}
	c.Social.Refresh()
	return true
}

// confirm asks OK or Cancel in the talk window (FUN_0034d150) and runs
// yes on OK.
func (c *Client) confirm(text []byte, yes func()) {
	c.talkConfirm = yes
	c.Talk.Ask([][]byte{text}, hud.Speaker{}, nil, [][]byte{nil, nil}, true)
}

// pickConfirm is a click while a confirm shows; it reports whether one did.
func (c *Client) pickConfirm(x, y int) bool {
	if c.talkConfirm == nil {
		return false
	}
	row, cancel := c.Talk.Pick(x, y)
	if row < 0 && !cancel {
		return true
	}
	yes := c.talkConfirm
	c.talkConfirm = nil
	c.Talk.Hide()
	if !cancel && row == 0 {
		yes()
	}
	return true
}

// friendsPacket is AC14's ported part.
func (c *Client) friendsPacket(p []byte) bool {
	if len(p) < 2 || p[0] != protocol.CommandFriends || c.Social == nil {
		return false
	}
	switch p[1] {
	case protocol.FriendsGuildUpdate:
		r := protocol.NewReader(p[2:])
		id, guild := r.U32(), r.String()
		if r.Err() != nil || r.Remaining() != 0 {
			return false
		}
		for i := range c.Social.Friends {
			if c.Social.Friends[i].ID == id {
				c.Social.Friends[i].Guild = []byte(guild)
			}
		}
		c.Social.Refresh()
		return true
	case protocol.FriendsRemove:
		if len(p) != 6 {
			return false
		}
		c.removeReceivedFriend(binary.LittleEndian.Uint32(p[2:]))
		return true
	case protocol.FriendsPresence:
		return c.receiveFriendPresence(p)
	case protocol.FriendsProfileUpdate, protocol.FriendsTabs:
		return c.receiveFriendProfiles(p)
	case protocol.FriendsList:
		return c.receiveFriendList(p[1:])
	case protocol.FriendsTextMail:
		return c.receiveLetter(p[1:])
	case protocol.FriendsRequest:
		return c.askFriend(p[1:])
	case protocol.FriendsAccept:
		return c.friendAnswered(p[1:])
	}
	return false
}

// Set Up tab sends (0x27a73c, FUN_0027b188).
const (
	socialStatusOnline = 0 // +0x3ca9
	socialStatusBusy   = 1
	// socialCodeRange is the social code's modulus (FUN_0027a304).
	socialCodeRange = 10
	birthYearBase   = 1900
)

func (c *Client) wireSetUp(s *hud.Social) {
	s.Now = c.Now
	s.SaveNickname = func(nick []byte) {
		if c.World != nil {
			c.World.Player.Nickname = append([]byte(nil), nick...)
		}
		p := append([]byte{protocol.CommandSocialRelations, protocol.SocialNicknameUpdate, byte(len(nick))}, nick...)
		c.Net.Send(p)
	}
	s.SaveProfile = c.saveSocialProfile
	s.RequestFriends = func() {
		c.Net.Send([]byte{protocol.CommandSocialRelations, protocol.SocialFriendList})
	}
	s.SetStatus = func(busy bool) {
		status := byte(socialStatusOnline)
		if busy {
			status = socialStatusBusy
		}
		if c.World != nil {
			c.World.Player.Presence = status
		}
		c.Net.Send([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefreshRequest})
	}
}

// saveSocialProfile is AC10:2: the social code, then blood type, birth
// year offset, month and day. The code comes from the profile stored
// before this save (FUN_0027a304 reads the player's +0x20aa.. before
// updating them): ((year / (month + day + 2)) × sign × blood + ID mod 10 +
// the name's length) mod 10.
func (c *Client) saveSocialProfile(sign, blood, year, month, day byte) {
	if c.World == nil {
		return
	}
	p := &c.World.Player
	code := ((birthYearBase+int(p.BirthYear))/(int(p.BirthMonth)+int(p.BirthDay)+2)*int(sign)*int(p.Blood) + int(p.ID%10) + len(p.Name)) % socialCodeRange
	p.Blood, p.BirthYear, p.BirthMonth, p.BirthDay = blood, year, month, day
	c.Net.Send([]byte{protocol.CommandSocialRelations, protocol.SocialProfileUpdate, byte(code), blood, year, month, day})
}

// mailTextKind is the 14/1 kind byte for a friend mail (the history's
// kind 1, 0x3c03c4).
const mailTextKind = 1

func (c *Client) newMailSend() {
	m := hud.NewMailSend(c.Env)
	m.DrawPortrait = c.drawMailPortrait
	m.Sound = c.playSound
	m.SendMail = func(to uint32, text []byte) {
		p := protocol.Builder{protocol.CommandFriends, protocol.FriendsTextMail, mailTextKind}.U32(to)
		c.Net.Send(append(p, text...))
	}
	c.MailSend = m
	c.UI.Add(m)
	c.Social.OpenMail = m.Open
}

// drawMailPortrait is FUN_003c4b4c's face: action 2 for a player, the
// GM's 008 face otherwise.
func (c *Client) drawMailPortrait(f hud.Friend, x, y int) {
	if c.lib == nil {
		return
	}
	if f.Body == 0 {
		c.drawFriendPortrait(f, x, y)
		return
	}
	if c.friendFaces == nil {
		c.friendFaces = map[uint32]*role.Human{}
	}
	h := c.friendFaces[f.ID]
	if h == nil {
		h = role.NewHuman(c.lib, c.items)
		h.Now = func() time.Time { return c.Now() }
		c.friendFaces[f.ID] = h
	}
	h.SetCharacter(&login.CharacterSlot{Body: f.Body, Head: f.Head, Color1: f.Color1, Color2: f.Color2})
	h.DrawFace(c.Screen, x, y, mailPortraitAction, false)
}

const mailPortraitAction = 2
