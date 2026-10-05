package server

import (
	"context"
	"encoding/binary"
	"errors"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

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
	if other == nil || other == c || !other.ready || !sameScene(other, c) {
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
			return c.send(headBanner("A friend list is full."))
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
			online, err := protocol.Builder{protocol.CommandFriends, protocol.FriendsAccepted}.U32(friend.character.ID).String(friend.character.Name)
			if err != nil {
				return err
			}
			packets = append(packets, protocol.Builder{protocol.CommandFriends, protocol.FriendsAcceptanceReply}.U32(friend.character.ID).U8(protocol.FriendsAcceptanceSuccess), online)
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
		return c.send(headBanner("Social title must fit in 14 bytes."))
	}
	for _, b := range []byte(nickname) {
		if b < ' ' || b == 127 {
			return c.send(headBanner("Social title contains invalid characters."))
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
		return c.send(headBanner("Fill in a valid blood type and birthday before saving your social profile."))
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
