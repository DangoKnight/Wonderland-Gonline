package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

func TestSocialNativeStatusGolden(t *testing.T) {
	packet, err := socialStatusPacket(&Session{character: &game.Character{ID: 0x12345678, Name: "Alice"}})
	expected := []byte{10, 1, 0x78, 0x56, 0x34, 0x12, 1, 5, 'A', 'l', 'i', 'c', 'e'}
	if err != nil || !bytes.Equal(packet, expected) {
		t.Fatal(packet, err)
	}
}

func TestSocialAddReplyListAndRemoval(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	ctx := context.Background()
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	for _, pair := range []struct {
		owner, friend *Session
		wire          *captureConn
	}{{a, b, w[0]}, {b, a, w[1]}} {
		packets := pair.wire.packets(t)
		online, _ := protocol.Builder{14, 7}.U32(pair.friend.character.ID).String(pair.friend.character.Name)
		status, _ := protocol.Builder{10, 1}.U32(pair.friend.character.ID).U8(1).String(pair.friend.character.Name)
		if len(packets) != 3 || !contains(packets, protocol.Builder{14, 9}.U32(pair.friend.character.ID).U8(0)) || !contains(packets, online) || !contains(packets, status) {
			t.Fatal(packets)
		}
		friends, err := s.Store.Friends(ctx, pair.owner.character.ID)
		if err != nil || len(friends) != 1 || friends[0].ID != pair.friend.character.ID {
			t.Fatal(friends, err)
		}
	}
	if w[2].Len() != 0 {
		t.Fatal("social packets leaked")
	}
	// A repeat emits status only and does not duplicate the durable relationship.
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	for _, wire := range w[:2] {
		packets := wire.packets(t)
		if len(packets) != 1 || packets[0][0] != 10 || packets[0][1] != 1 {
			t.Fatal(packets)
		}
	}
	for _, request := range [][]byte{{10, 3}, {10, 6}, protocol.Builder{10, 6}.U32(b.character.ID)} {
		tradeDo(t, s, a, request)
		packets := w[0].packets(t)
		if len(packets) != 2 || !contains(packets, protocol.Builder{14, 11}.Bytes(friendEntry(*b.character, true, true))) || !contains(packets, protocol.Builder{14, 5}.Bytes(friendEntry(*b.character, false, true))) {
			t.Fatal(packets)
		}
	}
	tradeDo(t, s, a, protocol.Builder{10, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{10, 4}.U32(b.character.ID)) || !contains(w[1].packets(t), protocol.Builder{14, 4}.U32(a.character.ID)) {
		t.Fatal("removal receipt missing")
	}
	// Legacy AC10:2 acceptance independently adds a same-map contact.
	tradeDo(t, s, b, protocol.Builder{10, 2}.U32(a.character.ID).U8(1))
	for _, wire := range w[:2] {
		packets := wire.packets(t)
		if len(packets) != 2 || packets[0][0] != 14 || packets[0][1] != 9 || packets[1][0] != 14 || packets[1][1] != 7 {
			t.Fatal(packets)
		}
	}
}

func TestSocialRejectsRemoteLoadingSelfAndDeclinedContacts(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b, remote := p[0], p[1], p[2]
	for _, packet := range [][]byte{protocol.Builder{10, 1}.U32(remote.character.ID), protocol.Builder{10, 2}.U32(remote.character.ID).U8(1), protocol.Builder{10, 1}.U32(a.character.ID), protocol.Builder{10, 1}.U32(999999), protocol.Builder{10, 2}.U32(b.character.ID).U8(0)} {
		tradeDo(t, s, a, packet)
	}
	b.ready = false
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal(friends, err)
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("rejected contact published packets")
		}
	}
}

func TestSocialValidatesBeforeMutationAndUsesWorldGates(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	for _, packet := range [][]byte{{10}, {10, 1}, {10, 1, 1, 2, 3}, protocol.Builder{10, 1}.U32(b.character.ID).U8(0), protocol.Builder{10, 2}.U32(b.character.ID), protocol.Builder{10, 2}.U32(b.character.ID).U8(2), {10, 3, 0}, {10, 4}, {10, 6, 1}} {
		if err := s.dispatch(context.Background(), a, packet); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(packet, err)
		}
	}
	if err := s.dispatch(context.Background(), a, []byte{10, 99}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("invalid request emitted packets")
		}
	}
	for _, gate := range []string{"loading", "battle", "minigame"} {
		a.ready = gate != "loading"
		a.battle = nil
		a.event = nil
		if gate == "battle" {
			a.battle = &battleRun{}
		}
		if gate == "minigame" {
			a.event = &eventSession{onMinigame: func(byte) error { t.Fatal("social consumed event"); return nil }}
		}
		err := s.dispatch(context.Background(), a, protocol.Builder{10, 1}.U32(b.character.ID))
		if gate == "loading" && !errors.Is(err, protocol.ErrMalformed) {
			t.Fatal(err)
		}
		if gate != "loading" && err != nil {
			t.Fatal(err)
		}
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("blocked request added friend", friends, err)
	}
}

func TestSocialSharesAC14StateAndClearsInvitations(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	tradeDo(t, s, a, protocol.Builder{14, 2}.U32(b.character.ID))
	w[1].Reset()
	a.friendRequests = map[uint32]time.Time{b.character.ID: time.Now()}
	tradeDo(t, s, b, protocol.Builder{10, 2}.U32(a.character.ID).U8(1))
	w[0].Reset()
	w[1].Reset()
	if len(a.friendRequests) != 0 || len(b.friendRequests) != 0 {
		t.Fatal("obsolete invitations retained")
	}
	tradeDo(t, s, b, protocol.Builder{14, 3}.U32(a.character.ID))
	if w[0].Len() != 0 || w[1].Len() != 0 {
		t.Fatal("stale invitation acceptance replayed")
	}
	tradeDo(t, s, a, []byte{14, 2})
	if !contains(w[0].packets(t), protocol.Builder{14, 5}.Bytes(friendEntry(*b.character, false, true))) {
		t.Fatal("AC10 and AC14 lists diverged")
	}
	tradeDo(t, s, b, protocol.Builder{14, 4}.U32(a.character.ID))
	w[1].Reset()
	tradeDo(t, s, b, []byte{10, 3})
	if !contains(w[1].packets(t), []byte{14, 5}) {
		t.Fatal("AC14 removal retained AC10 contact")
	}
}

func TestSocialPersistenceFailureDoesNotPublishSuccess(t *testing.T) {
	s, p, w := friendFixture(t)
	s.Store.Close()
	for _, packet := range [][]byte{protocol.Builder{10, 1}.U32(p[1].character.ID), protocol.Builder{10, 2}.U32(p[1].character.ID).U8(1), protocol.Builder{10, 4}.U32(p[1].character.ID)} {
		if err := s.dispatch(context.Background(), p[0], packet); err == nil {
			t.Fatal("failed persistence accepted")
		}
	}
	for _, wire := range w {
		if wire.Len() != 0 {
			t.Fatal("failed commit published success")
		}
	}
}

func TestSocialFailedRecipientDoesNotFailActor(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	failed := &failedWorldConn{}
	b.conn = failed
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	if !failed.closed || w[0].Len() == 0 {
		t.Fatal("recipient failure isolation failed")
	}
	w[0].Reset()
	tradeDo(t, s, a, protocol.Builder{10, 4}.U32(b.character.ID))
	if !contains(w[0].packets(t), protocol.Builder{10, 4}.U32(b.character.ID)) {
		t.Fatal("failed recipient prevented removal receipt")
	}
	friends, err := s.Store.Friends(context.Background(), a.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal(friends, err)
	}
}

func TestSocialFullRecipientDoesNotCreateOneSidedContact(t *testing.T) {
	s, p, w := friendFixture(t)
	a, b := p[0], p[1]
	ctx := context.Background()
	for i := 0; i < store.FriendLimit; i++ {
		name := fmt.Sprintf("Contact%d", i)
		account, err := s.Store.Register(ctx, name, "password", "")
		if err != nil {
			t.Fatal(err)
		}
		character := b.character.Clone()
		character.ID = account.CharacterID(1)
		character.Name = name
		if err := s.Store.CreateCharacter(ctx, account, character); err != nil {
			t.Fatal(err)
		}
		if err := s.Store.AddFriend(ctx, b.character.ID, character.ID); err != nil {
			t.Fatal(err)
		}
	}
	tradeDo(t, s, a, protocol.Builder{10, 1}.U32(b.character.ID))
	actor, err := s.Store.Friends(ctx, a.character.ID)
	if err != nil || len(actor) != 0 {
		t.Fatal("full recipient left one-sided relationship", actor, err)
	}
	recipient, err := s.Store.Friends(ctx, b.character.ID)
	if err != nil || len(recipient) != store.FriendLimit {
		t.Fatal(len(recipient), err)
	}
	packets := w[0].packets(t)
	if len(packets) != 1 || packets[0][0] != 2 || packets[0][1] != 16 || w[1].Len() != 0 {
		t.Fatal("full list published success", packets)
	}
}

func TestNativeSocialProfileSavesAndPreservesPendingWalking(t *testing.T) {
	s, players, wires := friendFixture(t)
	c := players[0]
	ctx := context.Background()
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
	c.character.X++
	walkedX := c.character.X
	// Actual native nickname layout, including a three-character six-byte frame.
	for _, packet := range [][]byte{{10, 1, 3, 'A', 'B', 'C'}, {10, 1, 5, 'H', 'e', 'l', 'l', 'o'}, {10, 2, 9, 2, 100, 2, 29}} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal(packet, err)
		}
	}
	if c.character.Nickname != "Hello" || c.character.BloodType != 2 || c.character.BirthYearOffset != 100 || c.character.BirthMonth != 2 || c.character.BirthDay != 29 || c.character.SocialProfileCode != 9 || c.character.X != walkedX {
		t.Fatal("native profile or walking lost", c.character)
	}
	if c.autosaveBaseline.X != baseline.X || c.autosaveBaseline.Nickname != "Hello" {
		t.Fatal("checkpoint baseline overwritten")
	}
	friends, err := s.Store.Friends(ctx, c.character.ID)
	if err != nil || len(friends) != 0 {
		t.Fatal("profile created friendship", friends, err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Nickname != "Hello" || chars[0].BloodType != 2 || chars[0].BirthDay != 29 || chars[0].X != baseline.X {
		t.Fatal("profile not durable or walking prematurely saved", chars, err)
	}
	appearance, err := chars[0].AppearancePacket(false)
	if err != nil || !bytes.Equal(appearance[len(appearance)-4:], []byte{2, 100, 2, 29}) {
		t.Fatal("profile reconnect bytes", appearance, err)
	}
	peerPackets := wires[1].packets(t)
	want := protocol.Builder{10, 1}.U32(c.character.ID).U8(5).Bytes([]byte("Hello"))
	if !contains(peerPackets, want) || !contains(peerPackets, protocol.Builder{10, 2}.U32(c.character.ID).U8(9)) {
		t.Fatal("native profile peer update", peerPackets)
	}
	if wires[2].Len() != 0 {
		t.Fatal("profile leaked across maps")
	}
	s.autosaveCharacters(ctx)
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Nickname != "Hello" || chars[0].X != walkedX {
		t.Fatal("walking checkpoint clobbered profile", chars, err)
	}
}

func TestNativeSocialMissingFieldsStayConnectedAndDoNotSave(t *testing.T) {
	s, players, wires := friendFixture(t)
	c := players[0]
	ctx := context.Background()
	for _, packet := range [][]byte{{10, 2, 9, 0, 100, 2, 29}, {10, 2, 9, 2, 100, 0, 29}, {10, 2, 9, 2, 101, 2, 29}, {10, 2, 9, 2, 100, 2, 0}} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal("incomplete form rejected connection", packet, err)
		}
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].BloodType != 0 || c.character.BloodType != 0 {
		t.Fatal("invalid profile saved", chars, err)
	}
	for _, packet := range wires[0].packets(t) {
		if packet[0] != 2 || packet[1] != 16 {
			t.Fatal("invalid profile success reply", packet)
		}
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("invalid profile broadcast")
	}
}
