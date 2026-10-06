package server

import (
	"context"
	"wonderland-gonline/internal/protocol"
)

// Account changes exclude in-flight authentication so an old credential cannot
// publish a session after the change and escape the disconnect.
func (s *Server) ResetAccountPassword(ctx context.Context, id uint32, password string) error {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if err := s.Store.ResetPassword(ctx, id, password); err != nil {
		return err
	}
	s.KickAccount(id)
	return nil
}

func (s *Server) DeleteAccount(ctx context.Context, id uint32) error {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	characters, err := s.Store.Characters(ctx, id)
	if err != nil {
		return err
	}
	type relation struct{ owner, friend uint32 }
	var relations []relation
	for _, c := range characters {
		friends, err := s.Store.Friends(ctx, c.ID)
		if err != nil {
			return err
		}
		for _, f := range friends {
			relations = append(relations, relation{c.ID, f.ID})
		}
	}

	if err := s.Store.DeleteAccount(ctx, id); err != nil {
		return err
	}
	s.KickAccount(id)
	for _, r := range relations {
		if peer := s.friendPlayer(r.friend); peer != nil {
			delete(peer.friendRequests, r.owner)
			s.sendOrClose(peer, protocol.Builder{protocol.CommandFriends, protocol.FriendsRemove}.U32(r.owner))
			if err := s.sendFriendList(context.Background(), peer); err != nil {
				s.Log.Error("friend deletion refresh failed", "error", err)
			}
		}
	}
	return nil
}
