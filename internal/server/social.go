package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
)

const friendRequestTTL = time.Minute

// friendPlayer looks up exact character IDs across logged-in world characters. Caller holds worldMu.
func (s *Server) friendPlayer(id uint32) *Session {
	return s.friendSessions[id]
}

// friendCommand routes native AC14 text mail and friendship operations.
func (s *Server) friendCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.FriendsTextMail:
		return s.textMailCommand(ctx, c, p)
	case protocol.FriendsRequest, protocol.FriendsRemove:
		if len(p) != 2 && len(p) != 6 {
			return protocol.ErrMalformed
		}
		id := uint32(0)
		if len(p) == 6 {
			id = r.U32()
		}
		if id == 0 {
			return s.sendFriendList(ctx, c)
		}
		if p[1] == protocol.FriendsRequest {
			return s.requestFriend(ctx, c, id)
		}
		return s.removeFriend(ctx, c, id)
	case protocol.FriendsAccept:
		if len(p) != 6 && len(p) != 7 {
			return protocol.ErrMalformed
		}
		id := r.U32()
		// Native clients always send the answer byte; the short form is
		// retained as an acceptance for compatibility.
		answer := byte(protocol.FriendsAnswerAccept)
		if len(p) == 7 {
			answer = p[6]
		}
		return s.acceptFriend(ctx, c, id, answer)
	default:
		return ErrUnsupported
	}
}

func (s *Server) requestFriend(ctx context.Context, c *Session, id uint32) error {
	target := s.friendPlayer(id)
	if target == nil || target == c {
		return nil
	}
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil {
		return err
	}
	for _, f := range friends {
		if f.ID == id {
			return nil
		}
	}
	if len(friends) >= store.FriendLimit {
		return c.send(serverwire.HeadBanner("Your friend list is full."))
	}
	if target.friendRequests == nil {
		target.friendRequests = map[uint32]time.Time{}
	}
	if at, ok := target.friendRequests[c.character.ID]; ok && time.Since(at) < friendRequestTTL {
		return nil
	}
	target.friendRequests[c.character.ID] = time.Now()
	s.sendOrClose(target, protocol.Builder{protocol.CommandFriends, protocol.FriendsRequest}.U32(c.character.ID))
	return nil
}

// acceptFriend answers a pending request. Any answer but acceptance is
// reported to the asker with the same result byte, and no friendship forms.
func (s *Server) acceptFriend(ctx context.Context, c *Session, id uint32, answer byte) error {
	if answer < protocol.FriendsAnswerAccept || answer > protocol.FriendsAnswerIgnore {
		return protocol.ErrMalformed
	}
	at, pending := c.friendRequests[id]
	delete(c.friendRequests, id)
	other := s.friendPlayer(id)
	if !pending || time.Since(at) > friendRequestTTL || other == nil || other == c {
		return nil
	}
	if answer != protocol.FriendsAnswerAccept {
		s.sendOrClose(other, protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(c.character.ID).U8(answer))
		return nil
	}
	if err := s.Store.AddFriend(ctx, c.character.ID, id); err != nil {
		if errors.Is(err, store.ErrFriendLimit) {
			return c.send(serverwire.HeadBanner("A friend list is full."))
		}
		return err
	}
	// Publish only after the shared relationship is durable.
	for _, pair := range [][2]*Session{{other, c}, {c, other}} {
		owner, friend := pair[0], pair[1]
		online := protocol.Builder{protocol.CommandFriends, protocol.FriendsTabs}.Bytes(serverwire.FriendEntry(*friend.character, true, true))
		s.sendOrClose(owner, protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(friend.character.ID).U8(protocol.FriendsAnswerAccept), protocol.Builder{protocol.CommandFriends, protocol.FriendsPresence}.U32(friend.character.ID).U8(protocol.FriendsPresenceOnline), online, protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(friend.character.ID).U8(255))
		if err := s.sendFriendList(ctx, owner); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) removeFriend(ctx context.Context, c *Session, id uint32) error {
	removed, err := s.Store.RemoveFriend(ctx, c.character.ID, id)
	if err != nil {
		return err
	}
	delete(c.friendRequests, id)
	s.sendOrClose(c, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(id))
	if err := s.sendFriendList(ctx, c); err != nil {
		return err
	}
	if other := s.friendPlayer(id); removed && other != nil && other != c {
		delete(other.friendRequests, c.character.ID)
		s.sendOrClose(other, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(c.character.ID))
		if err := s.sendFriendList(ctx, other); err != nil {
			s.Log.Warn("friend removal refresh failed", "recipient", id, "error", err)
			other.conn.Close()
		}
	}
	return nil
}

func (s *Server) sendFriendList(ctx context.Context, c *Session) error {
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil {
		return err
	}
	return s.sendFriends(c, friends)
}
func (s *Server) sendFriends(c *Session, friends []game.Character) error {
	tab, list := protocol.Builder{protocol.CommandFriends, protocol.FriendsTabs}, protocol.Builder{protocol.CommandFriends, protocol.FriendsList}
	presence := protocol.Builder{protocol.CommandFriends, protocol.FriendsPresence}
	for _, f := range friends {
		peer := s.friendPlayer(f.ID)
		if peer != nil {
			f = peer.character.Clone()
		}
		tab = tab.Bytes(serverwire.FriendEntry(f, true, peer != nil))
		list = list.Bytes(serverwire.FriendEntry(f, false, peer != nil))
		status := byte(protocol.FriendsPresenceOffline)
		if peer != nil {
			status = protocol.FriendsPresenceOnline
		}
		presence = presence.U32(f.ID).U8(status)
	}
	packets := [][]byte{tab, list}
	if len(friends) > 0 {
		packets = append(packets, presence)
	}
	return s.sendAll(c, packets)
}

// friendPresence runs on initial map publication and disconnect, never on warps.
// Empty lists are returned on demand; no extra login packets are added for users
// with no friends. Caller holds worldMu.
func (s *Server) friendPresence(c *Session, online bool) {
	if c.character == nil {
		return
	}
	ctx := context.Background()
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil {
		s.Log.Error("friend presence failed", "error", err)
		return
	}
	if online && len(friends) > 0 {
		if err := s.sendFriends(c, friends); err != nil {
			c.conn.Close()
		}
	}
	for _, f := range friends {
		peer := s.friendPlayer(f.ID)
		if peer == nil {
			continue
		}
		status := byte(protocol.FriendsPresenceOffline)
		if online {
			status = protocol.FriendsPresenceOnline
		}
		p := protocol.Builder{protocol.CommandFriends, protocol.FriendsPresence}.U32(c.character.ID).U8(status)
		s.sendOrClose(peer, p)
		if err := s.sendFriendList(ctx, peer); err != nil {
			s.Log.Error("friend list refresh failed", "error", err)
		}
	}
}

func (s *Server) clearFriendRequests(c *Session) {
	c.friendRequests = nil
	if c.character == nil {
		return
	}
	for _, peer := range s.friendSessions {
		delete(peer.friendRequests, c.character.ID)
	}
}

const nativeSocialBloodTypeMax = 4

// socialCommand implements the native AC10 contact operations under worldMu.
// AC10 adds contacts immediately on the same map; AC14 keeps its invitation flow.
func (s *Server) socialCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	// WLRI AC10:1 is a length-prefixed nickname and AC10:2 carries a
	// social code, blood type and birthday (sender 0x2c3ddb/0x2c3ea0).
	// Preserve old contact adapters only for recognizable character-ID layouts.
	if p[1] == protocol.SocialNicknameUpdate && len(p) >= 3 && int(p[2]) == len(p)-3 {
		if len(p) != 6 || s.friendPlayer(binary.LittleEndian.Uint32(p[2:])) == nil {
			return s.updateSocialNickname(ctx, c, string(p[3:]))
		}
	}
	if p[1] == protocol.SocialProfileUpdate && len(p) == protocol.SocialProfileRequestBytes {
		id := binary.LittleEndian.Uint32(p[2:])
		if s.friendPlayer(id) == nil {
			return s.updateSocialProfile(ctx, c, p[2:])
		}
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.SocialAddFriend:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.addLocalContact(ctx, c, r.U32(), true)
	case protocol.SocialFriendReply:
		if len(p) != 7 {
			return protocol.ErrMalformed
		}
		id, reply := r.U32(), r.U8()
		switch reply {
		case protocol.SocialFriendDecline:
			return nil
		case protocol.SocialFriendAccept:
			return s.addLocalContact(ctx, c, id, false)
		default:
			return protocol.ErrMalformed
		}
	case protocol.SocialFriendList:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendFriendList(ctx, c)
	case protocol.SocialQueryStatus:
		// The reference refreshes the whole list; it never reads a target operand.
		// Accept a bare query or an optional uint32-shaped operand, which is ignored.
		if len(p) != 2 && len(p) != 6 {
			return protocol.ErrMalformed
		}
		return s.sendFriendList(ctx, c)
	case protocol.SocialRemoveFriend:
		if len(p) != 6 {
			return protocol.ErrMalformed
		}
		id := r.U32()
		if err := s.removeFriend(ctx, c, id); err != nil {
			return err
		}
		return c.send(protocol.Builder{protocol.CommandSocialRelations, protocol.SocialRemoveFriend}.U32(id))
	default:
		return ErrUnsupported
	}
}

func socialStatusPacket(character *Session) ([]byte, error) {
	return protocol.Builder{protocol.CommandSocialRelations, protocol.SocialFriendStatus}.U32(character.character.ID).U8(protocol.SocialFriendOnline).String(character.character.Name)
}

func (s *Server) addLocalContact(ctx context.Context, c *Session, id uint32, notifyStatus bool) error {
	other := s.friendPlayer(id)
	if other == nil || other == c || !other.ready || !s.samePlayerScene(other, c) {
		return nil
	}
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil {
		return err
	}
	existing := false
	for _, friend := range friends {
		if friend.ID == id {
			existing = true
			break
		}
	}
	if err := s.Store.AddFriend(ctx, c.character.ID, id); err != nil {
		if errors.Is(err, store.ErrFriendLimit) {
			return c.send(serverwire.HeadBanner("A friend list is full."))
		}
		return err
	}
	// Clear obsolete AC14 invitations only once the relationship is durable.
	delete(c.friendRequests, id)
	delete(other.friendRequests, c.character.ID)
	for _, pair := range [][2]*Session{{c, other}, {other, c}} {
		owner, friend := pair[0], pair[1]
		var packets [][]byte
		if !existing {
			online := protocol.Builder{protocol.CommandFriends, protocol.FriendsTabs}.Bytes(serverwire.FriendEntry(*friend.character, true, true))
			packets = append(packets, protocol.Builder{protocol.CommandFriends, protocol.FriendsPresence}.U32(friend.character.ID).U8(protocol.FriendsPresenceOnline), online)
		}
		if notifyStatus {
			status, err := socialStatusPacket(friend)
			if err != nil {
				return err
			}
			packets = append(packets, status)
		}
		s.sendOrClose(owner, packets...)
	}
	return nil
}

func (s *Server) updateSocialNickname(ctx context.Context, c *Session, nickname string) error {
	if len(nickname) > protocol.SocialNicknameMaxBytes {
		return c.send(serverwire.HeadBanner("Social title must fit in 14 bytes."))
	}
	for _, b := range []byte(nickname) {
		if b < ' ' || b == 127 {
			return c.send(serverwire.HeadBanner("Social title contains invalid characters."))
		}
	}
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error { stored.Nickname = nickname; return nil }); err != nil {
		return err
	}
	c.character.Nickname = nickname
	if c.autosaveBaseline != nil {
		c.autosaveBaseline.Nickname = nickname
	}
	packet, err := protocol.Builder{protocol.CommandSocialRelations, protocol.SocialNicknameUpdate}.U32(c.character.ID).String(nickname)
	if err != nil {
		return err
	}
	s.broadcastWorld(c, packet)
	return nil // Native owner has already updated its local nickname.
}

func (s *Server) updateSocialProfile(ctx context.Context, c *Session, values []byte) error {
	code, blood, year, month, day := values[0], values[1], values[2], values[3], values[4]
	// Native birth year is the byte offset from 1900; zero fields are unfilled.
	const nativeBirthYearBase = 1900
	birthday := time.Date(nativeBirthYearBase+int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	if blood == 0 || blood > nativeSocialBloodTypeMax || year == 0 || month == 0 || day == 0 || birthday.Month() != time.Month(month) || birthday.Day() != int(day) {
		return c.send(serverwire.HeadBanner("Fill in a valid blood type and birthday before saving your social profile."))
	}
	if err := s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(stored *game.Character) error {
		stored.SocialProfileCode, stored.BloodType, stored.BirthYearOffset, stored.BirthMonth, stored.BirthDay = code, blood, year, month, day
		return nil
	}); err != nil {
		return err
	}
	for _, ch := range []*game.Character{c.character, c.autosaveBaseline} {
		if ch != nil {
			ch.SocialProfileCode, ch.BloodType, ch.BirthYearOffset, ch.BirthMonth, ch.BirthDay = code, blood, year, month, day
		}
	}
	// Native AC10:2 receive consumes character ID and the social code only.
	s.broadcastWorld(c, protocol.Builder{protocol.CommandSocialRelations, protocol.SocialProfileUpdate}.U32(c.character.ID).U8(code))
	return nil
}

const guildInvitationTTL = time.Minute

type guildInvitation struct {
	From    *Session
	GuildID uint32
	At      time.Time
}

func (s *Server) guildRosterPacket(guild *store.GuildInfo) ([]byte, error) {
	leader := ""
	for _, member := range guild.Roster {
		if member.Character.ID == guild.LeaderID {
			leader = member.Character.Name
		}
	}
	p, err := protocol.Builder{protocol.CommandGuild, protocol.GuildRoster}.String(guild.Name)
	if err != nil {
		return nil, err
	}
	p = p.U32(guild.Icon)
	p, err = p.String(leader)
	if err != nil {
		return nil, err
	}
	if len(guild.Roster) > store.GuildMemberLimit {
		return nil, fmt.Errorf("guild exceeds native member limit")
	}
	p = p.U8(byte(len(guild.Roster)))
	for _, member := range guild.Roster {
		char := member.Character
		p = p.U32(char.ID)
		p, err = p.String(char.Name)
		if err != nil {
			return nil, err
		}
		online := byte(0)
		if s.onlineByID(char.ID) != nil {
			online = 1
		}
		p = p.U8(byte(char.Level)).U8(char.Job).U8(byte(char.Element)).U8(member.Rank).U8(online)
	}
	return p.String(guild.Notice)
}
func (s *Server) syncGuild(ctx context.Context, c *Session) error {
	guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
	if err != nil {
		return err
	}
	if guild == nil {
		return nil
	}
	roster, err := s.guildRosterPacket(guild)
	if err != nil {
		return err
	}
	if err = s.sendAll(c, [][]byte{{protocol.CommandGuild, protocol.GuildJournal, 0}, roster}); err != nil {
		return err
	}
	badge := serverwire.GuildBadge(c.character.ID, guild)
	s.broadcastWorld(c, badge)
	return c.send(badge)
}
func (s *Server) refreshGuilds(ctx context.Context) error {
	// Membership remains SQL-authoritative, including edits through Admin.
	for _, peer := range s.world {
		if !peer.ready {
			continue
		}
		if err := s.syncGuild(ctx, peer); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) guildChat(ctx context.Context, c *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Guild messages must contain 1–60 bytes.")
	}
	if time.Now().Before(c.character.MutedUntil) {
		return s.chatFeedback(c, "Your chat privileges are temporarily muted.")
	}
	guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
	if err != nil {
		return err
	}
	if guild == nil {
		return s.chatFeedback(c, "You are not in a guild.")
	}
	var recipients []*Session
	for _, id := range guild.Members {
		if peer := s.onlineByID(id); peer != nil {
			recipients = append(recipients, peer)
		}
	}
	s.deliverChat(c, recipients, game.ChatChannelGuild, chatPacket(protocol.ChatGuildMessage, c, []byte(text)), true)
	return nil
}
func (s *Server) guildCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.GuildHeaderRequestBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	switch p[1] {
	case protocol.GuildJournal:
		if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		return s.sendAll(c, s.World.Journal(c.character, c.view))
	case protocol.GuildInvite:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		target := s.mapPlayer(c, r.U32())
		if target == nil || target == c || !tradeNear(c, target) {
			return nil
		}
		guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			return s.chatFeedback(c, "You are not in a guild.")
		}
		permitted := guild.LeaderID == c.character.ID
		for _, member := range guild.Roster {
			if member.Character.ID == c.character.ID && member.Rank == store.GuildRankViceLeader {
				permitted = true
			}
		}
		if !permitted {
			return s.chatFeedback(c, store.ErrGuildPermission.Error())
		}
		own, err := s.Store.GuildForCharacter(ctx, target.character.ID)
		if err != nil {
			return err
		}
		if own != nil {
			return s.chatFeedback(c, "That character already belongs to a guild.")
		}
		if target.guildInvitation != nil && time.Since(target.guildInvitation.At) < guildInvitationTTL {
			return s.chatFeedback(c, "That character already has a pending guild invitation.")
		}
		target.guildInvitation = &guildInvitation{From: c, GuildID: guild.ID, At: time.Now()}
		return target.send(protocol.Builder{protocol.CommandGuild, protocol.GuildAccept}.U32(c.character.ID))
	case protocol.GuildAccept:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		id := r.U32()
		request := c.guildInvitation
		if request == nil || request.From.character.ID != id {
			return nil
		}
		c.guildInvitation = nil
		from := request.From
		if time.Since(request.At) > guildInvitationTTL || s.world[from.info.ID] != from || !from.ready || !tradeNear(c, from) {
			return s.chatFeedback(c, "Guild invitation expired.")
		}
		guild, err := s.Store.GuildForCharacter(ctx, from.character.ID)
		if err != nil {
			return err
		}
		if guild == nil || guild.ID != request.GuildID {
			return s.chatFeedback(c, "Guild invitation is no longer valid.")
		}
		if err = s.Store.JoinGuild(ctx, id, c.character.ID); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildLeave, protocol.GuildDismiss:
		target := c.character.ID
		if p[1] == protocol.GuildDismiss {
			if len(p) != protocol.GuildTargetRequestBytes {
				return protocol.ErrMalformed
			}
			target = r.U32()
		} else if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		if err := s.Store.LeaveGuild(ctx, c.character.ID, target); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		if peer := s.onlineByID(target); peer != nil {
			badge := serverwire.GuildBadge(target, nil)
			s.broadcastWorld(peer, badge)
			s.sendOrClose(peer, []byte{protocol.CommandGuild, protocol.GuildJournal, 0}, badge)
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildMessage:
		text := r.String()
		if r.Err() != nil || r.Remaining() != 0 {
			return protocol.ErrMalformed
		}
		return s.guildChat(ctx, c, text)
	case protocol.GuildNotice:
		// Original UnpackStringN consumes the remaining bytes without a length prefix.
		text := string(r.Rest())
		if !validChatText(text, store.GuildNoticeMaxBytes) && text != "" {
			return protocol.ErrMalformed
		}
		if err := s.Store.EditGuild(ctx, c.character.ID, &text, nil, 0, nil); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildDemote, protocol.GuildPromote, protocol.GuildPermission:
		expected := protocol.GuildTargetRequestBytes
		if p[1] == protocol.GuildPermission {
			expected = protocol.GuildPermissionRequestBytes
		}
		if len(p) != expected {
			return protocol.ErrMalformed
		}
		target := r.U32()
		rank := store.GuildRankMember
		if p[1] == protocol.GuildPromote {
			rank = store.GuildRankViceLeader
		}
		if p[1] == protocol.GuildPermission {
			rank = r.U8()
		}
		if err := s.Store.EditGuild(ctx, c.character.ID, nil, nil, target, &rank); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case protocol.GuildMemberList:
		if len(p) != protocol.GuildHeaderRequestBytes {
			return protocol.ErrMalformed
		}
		guild, err := s.Store.GuildForCharacter(ctx, c.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			return c.send([]byte{protocol.CommandGuild, protocol.GuildMemberList, 0})
		}
		return s.syncGuild(ctx, c)
	case protocol.GuildInsignia:
		if len(p) != protocol.GuildTargetRequestBytes {
			return protocol.ErrMalformed
		}
		icon := r.U32()
		if err := s.Store.EditGuild(ctx, c.character.ID, nil, &icon, 0, nil); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	default:
		return ErrUnsupported
	}
}

// Administration follows the same world ownership as native guild commands.
// Successful database edits refresh badges/rosters of all affected online players.
func (s *Server) AdminSaveGuild(ctx context.Context, guild store.AdminGuild) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := s.Store.SaveAdminGuild(ctx, guild); err != nil {
		return err
	}
	return s.refreshGuildAdministration(ctx)
}
func (s *Server) AdminDeleteGuild(ctx context.Context, id uint32) error {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if err := s.Store.DeleteAdminGuild(ctx, id); err != nil {
		return err
	}
	return s.refreshGuildAdministration(ctx)
}
func (s *Server) refreshGuildAdministration(ctx context.Context) error {
	for _, peer := range s.world {
		if !peer.ready {
			continue
		}
		guild, err := s.Store.GuildForCharacter(ctx, peer.character.ID)
		if err != nil {
			return err
		}
		if guild == nil {
			badge := serverwire.GuildBadge(peer.character.ID, nil)
			s.sendOrClose(peer, []byte{protocol.CommandGuild, protocol.GuildJournal, 0}, badge)
			s.broadcastWorld(peer, badge)
		} else if err = s.syncGuild(ctx, peer); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) guildPresence(ctx context.Context, id uint32) error {
	guild, err := s.Store.GuildForCharacter(ctx, id)
	if err != nil || guild == nil {
		return err
	}
	p, err := s.guildRosterPacket(guild)
	if err != nil {
		return err
	}
	for _, member := range guild.Members {
		if peer := s.onlineByID(member); peer != nil {
			s.sendOrClose(peer, p)
		}
	}
	return nil
}

const (
	textMailDeliveryBatchSize = 32
)

// textMailCommand runs under worldMu. The reference ignores the type byte;
// retain it for provenance without assigning unverified attachment semantics.
func (s *Server) textMailCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < protocol.TextMailRequestHeaderBytes {
		return protocol.ErrMalformed
	}
	r := protocol.NewReader(p[2:])
	kind, recipient := r.U8(), r.U32()
	content := bytes.TrimRight(r.Rest(), "\x00") // Native accepts trailing NUL terminators.
	if r.Err() != nil || store.ValidateMailContent(content) != nil {
		return protocol.ErrMalformed
	}
	message, err := s.Store.SendTextMail(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, recipient, kind, content, time.Now())
	if errors.Is(err, sql.ErrNoRows) {
		return c.send(serverwire.HeadBanner("That character does not exist."))
	}
	if err != nil {
		return err
	}
	// The native AC14:9 updates presence; its final byte is not a mail
	// receipt result. Keep offline delivery from marking a friend online.
	status := byte(protocol.FriendsPresenceOffline)
	if s.friendPlayer(recipient) != nil {
		status = protocol.FriendsPresenceOnline
	}
	if err := c.send(protocol.Builder{protocol.CommandFriends, protocol.FriendsPresence}.U32(recipient).U8(status)); err != nil {
		return err
	}
	if target := s.friendPlayer(message.ReceiverID); target != nil && target.ready {
		if err := s.deliverPendingTextMail(ctx, target); err != nil {
			s.Log.Warn("mail delivery deferred", "recipient", message.ReceiverID, "error", err)
			target.conn.Close()
		}
	}
	return nil
}

// Delivery is at least once: the native protocol has no receipt acknowledgment.
// A crash between a successful write and the durable marker can repeat a message.
func (s *Server) deliverPendingTextMail(ctx context.Context, c *Session) error {
	for {
		messages, err := s.Store.PendingTextMail(ctx, c.character.ID, textMailDeliveryBatchSize)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			c.textMailPending = false
			return nil
		}
		c.textMailPending = true
		for _, message := range messages {
			key := deliveryKey{deliveryTextMail, c.character.ID, message.ID}
			if c.pendingMail[key] != nil {
				continue
			}
			packet, err := serverwire.TextMailPacket(message)
			if err != nil {
				return err
			}
			if err := s.sendMailDelivery(ctx, c, key, [][]byte{packet}); err != nil {
				return err
			}
		}
		if len(c.pendingMail) > 0 {
			return nil
		}
	}
}
