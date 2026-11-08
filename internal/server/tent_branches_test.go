package server

import (
	"context"
	"errors"
	"testing"

	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func TestTentChatLockCommands(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/tentlock")...)); err != nil {
		t.Fatal(err)
	}
	tent, err := s.Store.Tent(ctx, c.character.ID)
	if err != nil || !tent.Locked {
		t.Fatal("tent was not locked", err)
	}
	if err := s.worldCommand(ctx, c, append([]byte{2, 2}, []byte("/tentunlock")...)); err != nil {
		t.Fatal(err)
	}
	tent, err = s.Store.Tent(ctx, c.character.ID)
	if err != nil || tent.Locked {
		t.Fatal("tent was not unlocked", err)
	}
}

func TestTentSnapshotReportsLockedStatus(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner := players[0]
	if err := s.Store.SetTentLocked(context.Background(), store.CharacterRef{Account: owner.account.ID, ID: owner.character.ID}, true); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.tentSnapshot(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	if !contains(wires[0].packets(t), []byte{protocol.CommandTent, protocol.TentInteriorStatus, 1, 0}) {
		t.Fatal("locked status missing", wires[0].packets(t))
	}
}

func TestSameSceneAndPeerGuards(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	if sameScene(nil, owner) || sameScene(owner, nil) {
		t.Fatal("nil sessions share a scene")
	}
	if sameScene(&Session{}, &Session{}) {
		t.Fatal("empty sessions share a scene")
	}
	enterTestTent(t, s, guest, owner.character.ID)
	other := guest.character.Clone()
	other.Map = 12000
	guest.character = &other
	if got := len(s.tentPeers(owner)); got != 0 {
		t.Fatalf("peer on another map listed: %d", got)
	}
	guest.character = nil
	if got := len(s.tentPeers(owner)); got != 0 {
		t.Fatalf("peer without a character listed: %d", got)
	}
}

func TestTentCommandRejectsUnknownAndTruncated(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{65, 99}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown tent subcommand accepted", err)
	}
	if err := s.dispatch(ctx, c, []byte{65, 1, 0, 0}); err == nil {
		t.Fatal("truncated tent entry accepted")
	}
	c.openTent = nil
	if err := s.dispatch(ctx, c, []byte{65, 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{62, 99}); !errors.Is(err, ErrUnsupported) {
		t.Fatal("unknown furniture subcommand accepted", err)
	}
	if err := s.dispatch(ctx, c, []byte{62, 1}); err == nil {
		t.Fatal("truncated placement accepted")
	}
}
