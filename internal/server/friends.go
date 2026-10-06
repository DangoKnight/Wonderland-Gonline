package server

import (
	"context"
	"errors"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
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
		group := byte(1)
		if len(p) == 7 {
			group = p[6]
		}
		return s.acceptFriend(ctx, c, id, group)
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
		return c.send(headBanner("Your friend list is full."))
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

func (s *Server) acceptFriend(ctx context.Context, c *Session, id uint32, group byte) error {
	at, pending := c.friendRequests[id]
	delete(c.friendRequests, id)
	other := s.friendPlayer(id)
	if !pending || time.Since(at) > friendRequestTTL || other == nil || other == c {
		return nil
	}
	if err := s.Store.AddFriend(ctx, c.character.ID, id); err != nil {
		if errors.Is(err, store.ErrFriendLimit) {
			return c.send(headBanner("A friend list is full."))
		}
		return err
	}
	// Publish only after the shared relationship is durable.
	for _, pair := range [][2]*Session{{other, c}, {c, other}} {
		owner, friend := pair[0], pair[1]
		online, _ := protocol.Builder{protocol.CommandFriends, protocol.FriendsAccepted}.U32(friend.character.ID).String(friend.character.Name)
		s.sendOrClose(owner, protocol.Builder{protocol.CommandFriends, protocol.FriendsAccept}.U32(friend.character.ID).U8(group), protocol.Builder{protocol.CommandFriends, protocol.FriendsAcceptanceReply}.U32(friend.character.ID).U8(0), online, protocol.Builder{protocol.CommandPresence, protocol.PresenceOnline}.U32(friend.character.ID).U8(255))
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

// friendEntry is AC14.PackFriendEntry, including saved class and nickname.
// Guild presentation retains its existing empty defaults. Colors retain their words.
func friendEntry(c game.Character, tab bool, online bool) []byte {
	p, _ := protocol.Builder{}.U32(c.ID).String(c.Name)
	p = p.U8(c.Level).U8(c.RebornByte()).U8(c.Job).U8(c.Element).U8(byte(c.Body)).U8(byte(c.Head)).U16(uint16(c.Color1)).U16(uint16(c.Color1 >> 16)).U16(uint16(c.Color2)).U16(uint16(c.Color2 >> 16))
	p, _ = p.String(c.Nickname)
	if !tab {
		p = p.U8(0)
		flag := byte(0)
		if online {
			flag = 1
		}
		p = p.U8(flag)
	}
	return p
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
	for _, f := range friends {
		peer := s.friendPlayer(f.ID)
		if peer != nil {
			f = peer.character.Clone()
		}
		tab = tab.Bytes(friendEntry(f, true, peer != nil))
		list = list.Bytes(friendEntry(f, false, peer != nil))
	}
	return s.sendAll(c, [][]byte{tab, list})
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
		p := protocol.Builder{protocol.CommandFriends, protocol.FriendsWireCode8}.U32(c.character.ID)
		if online {
			p, _ = protocol.Builder{protocol.CommandFriends, protocol.FriendsAccepted}.U32(c.character.ID).String(c.character.Name)
		}
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
