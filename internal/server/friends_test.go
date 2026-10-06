package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func friendFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	s, players, wires := worldFixture(t)
	for _, c := range players {
		tradeDo(t, s, c, []byte{12, 1})
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestFriendDispatcherRequestAcceptListsAndRemoval(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2] // Different maps.
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("unsolicited accept succeeded")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	if !contains(w[2].packets(t), protocol.Builder{14, 2}.U32(a.character.ID)) {
		t.Fatal("cross-map friend request missing")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	if w[2].Len() != 0 {
		t.Fatal("duplicate prompt")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID).U8(2))
	for _, pair := range []struct {
		owner, other *Session
		wire         *captureConn
	}{{a, b, w[0]}, {b, a, w[2]}} {
		packets := pair.wire.packets(t)
		id := pair.other.character.ID
		if !contains(packets, protocol.Builder{14, 3}.U32(id).U8(2)) || !contains(packets, protocol.Builder{14, 9}.U32(id).U8(0)) || !contains(packets, protocol.Builder{10, 3}.U32(id).U8(255)) {
			t.Fatal("native accept packets missing", packets)
		}
		if !contains(packets, protocol.Builder{14, 11}.Bytes(friendEntry(*pair.other.character, true, true))) || !contains(packets, protocol.Builder{14, 5}.Bytes(friendEntry(*pair.other.character, false, true))) {
			t.Fatal("native lists wrong", packets)
		}
		friends, err := s.Store.Friends(context.Background(), pair.owner.character.ID)
		if err != nil || len(friends) != 1 || friends[0].ID != id {
			t.Fatal("friend not durable", friends, err)
		}
	}
	if w[1].Len() != 0 {
		t.Fatal("friend packets leaked to unrelated player")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("accept replayed")
	}
	tradeDo(t, s, a, protocol.Builder{14, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{14, 4}.U32(b.character.ID)) || !contains(w[2].packets(t), protocol.Builder{14, 4}.U32(a.character.ID)) {
		t.Fatal("remove not replicated")
	}
	for _, c := range []*Session{a, b} {
		f, err := s.Store.Friends(context.Background(), c.character.ID)
		if err != nil || len(f) != 0 {
			t.Fatal("removed friendship remained", err)
		}
	}
}

func TestFriendPresenceOfflineAndReconnect(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	if err := s.Store.AddFriend(context.Background(), a.character.ID, b.character.ID); err != nil {
		t.Fatal(err)
	}
	s.leaveWorld(b)
	packets := w[0].packets(t)
	if !contains(packets, protocol.Builder{14, 8}.U32(b.character.ID)) || !contains(packets, protocol.Builder{14, 5}.Bytes(friendEntry(*b.character, false, false))) {
		t.Fatal("offline presence wrong", packets)
	}
	tradeDo(t, s, b, []byte{12, 1})
	online, _ := protocol.Builder{14, 7}.U32(b.character.ID).String(b.character.Name)
	if !contains(w[0].packets(t), online) {
		t.Fatal("friend reconnect missing")
	}
	w[2].Reset()
	tradeDo(t, s, b, []byte{14, 2})
	if !contains(w[2].packets(t), protocol.Builder{14, 5}.Bytes(friendEntry(*a.character, false, true))) {
		t.Fatal("list query wrong")
	}
	// A map load is not logout, but disconnect during it still reports offline.
	s.depart(b, protocol.Builder{12}.U32(b.character.ID))
	w[0].Reset()
	tradeDo(t, s, a, []byte{14, 2})
	if !contains(w[0].packets(t), protocol.Builder{14, 5}.Bytes(friendEntry(*b.character, false, true))) {
		t.Fatal("map loading appeared offline")
	}
	s.leaveWorld(b)
	if !contains(w[0].packets(t), protocol.Builder{14, 8}.U32(b.character.ID)) {
		t.Fatal("disconnect during warp failed to report offline")
	}
}

func TestFriendExpiredRequestsAndDeparture(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, action := range []string{"expired", "departed"} {
		tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
		w[2].Reset()
		if action == "expired" {
			b.friendRequests[a.character.ID] = time.Now().Add(-2 * friendRequestTTL)
		} else {
			s.leaveWorld(a)
		}
		tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
		if w[2].Len() != 0 {
			t.Fatal("stale friend accepted")
		}
		f, err := s.Store.Friends(context.Background(), b.character.ID)
		if err != nil || len(f) != 0 {
			t.Fatal("stale friendship saved", err)
		}
	}
}

func TestFriendMalformedAndFailedSave(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	for _, packet := range [][]byte{{14}, {14, 2, 1}, {14, 2, 1, 2, 3}, {14, 3}, {14, 3, 1, 2, 3}, {14, 3, 1, 2, 3, 4, 5, 6}, {14, 4, 1}} {
		if err := s.dispatch(context.Background(), a, packet); err == nil {
			t.Fatal("malformed friend packet accepted", packet)
		}
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(a.character.ID))
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(999999))
	if w[0].Len() != 0 {
		t.Fatal("invalid target acknowledged")
	}
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	w[2].Reset()
	s.Store.Close()
	if err := s.dispatch(context.Background(), b, protocol.Builder{14, 3}.U32(a.character.ID)); err == nil {
		t.Fatal("failed save ignored")
	}
	if w[0].Len() != 0 || w[2].Len() != 0 {
		t.Fatal("failed save emitted success")
	}
}

func TestFriendEntryGoldenAppearanceWords(t *testing.T) {
	c := game.Character{ID: 0x01020304, Name: "Al", Level: 25, Element: 3, Body: 2, Head: 4, Color1: 0x33441122, Color2: 0x77885566}
	expected := []byte{4, 3, 2, 1, 2, 'A', 'l', 25, 0, 0, 3, 2, 4, 0x22, 0x11, 0x44, 0x33, 0x66, 0x55, 0x88, 0x77, 0}
	if !hasPacket([][]byte{friendEntry(c, true, true)}, expected) {
		t.Fatal("friend appearance words")
	}
	if !hasPacket([][]byte{friendEntry(c, false, true)}, append(expected, 0, 1)) {
		t.Fatal("friend online record")
	}
}

func TestFriendAccountDeletionRefreshesOnlinePeer(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[2]
	if err := s.Store.AddFriend(context.Background(), a.character.ID, b.character.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAccount(context.Background(), b.account.ID); err != nil {
		t.Fatal(err)
	}
	if !contains(w[0].packets(t), protocol.Builder{14, 4}.U32(b.character.ID)) {
		t.Fatal("deleted account retained friend UI")
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("deleted account retained relationship", err)
	}
}
