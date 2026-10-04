package server

import (
	"context"
	"errors"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

// socialCommand implements the native AC10 contact operations under worldMu.
// AC10 adds contacts immediately on the same map; AC14 keeps its invitation flow.
func (s *Server) socialCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
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
