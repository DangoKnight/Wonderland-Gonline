package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func TestSyncTentOccupantsExchangesAppearances(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	enterTestTent(t, s, guest, owner.character.ID)
	wires[0].Reset()
	wires[1].Reset()
	s.syncTentOccupants(context.Background(), owner)
	toOwner := wires[0].packets(t)
	toGuest := wires[1].packets(t)
	if !hasPrefix(toOwner, []byte{protocol.CommandAppearance}) {
		t.Fatal("owner did not receive the guest appearance", toOwner)
	}
	if !hasPrefix(toGuest, []byte{protocol.CommandAppearance}) {
		t.Fatal("guest did not receive the owner appearance", toGuest)
	}
	// The level travels with the position so the client files the actor correctly.
	wantPosition := owner.character.PositionPacketFloor(owner.tentFloor)
	if !contains(toGuest, wantPosition) {
		t.Fatal("guest appearance missing the level position", toGuest)
	}
}

func TestTentFloorCommands(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner := players[0]
	ctx := context.Background()
	if err := s.floorUp(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if owner.tentFloor != 1 || !owner.tentOccupantsSync {
		t.Fatal("floorUp did not switch and defer the exchange")
	}
	if !contains(wires[0].packets(t), protocol.Builder{protocol.CommandPosition}.U32(owner.character.ID).U16(63507).U16(460).U16(700).U8(1)) {
		t.Fatal("floorUp position", wires[0].packets(t))
	}
	wires[0].Reset()
	if err := s.floorDown(ctx, owner); err != nil {
		t.Fatal(err)
	}
	if owner.tentFloor != 0 {
		t.Fatal("floorDown did not return to the ground floor")
	}
	if !contains(wires[0].packets(t), protocol.Builder{protocol.CommandPosition}.U32(owner.character.ID).U16(63507).U16(460).U16(700).U8(0)) {
		t.Fatal("floorDown position", wires[0].packets(t))
	}
}

func TestTentFloorCommandsRequireATent(t *testing.T) {
	s, players, wires := tentFixture(t)
	c := players[0]
	c.tentOwner = 0
	wires[0].Reset()
	if err := s.floorUp(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if err := s.floorDown(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.tentFloor != 0 {
		t.Fatal("commands changed the level outside a tent")
	}
	if len(wires[0].packets(t)) == 0 {
		t.Fatal("no guidance sent")
	}
}

func TestLockedTentRejectsGuests(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	if err := s.Store.SetTentLocked(context.Background(), store.CharacterRef{Account: owner.account.ID, ID: owner.character.ID}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.enterTent(context.Background(), guest, owner.character.ID); err != nil {
		t.Fatal(err)
	}
	if guest.tentOwner != 0 || guest.character.Map != 12000 {
		t.Fatal("guest entered a locked tent")
	}
}

func TestClosePlayerTentEvictsGuests(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	enterTestTent(t, s, guest, owner.character.ID)
	if err := s.closePlayerTent(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	if guest.tentOwner != 0 || guest.character.Map == 63507 {
		t.Fatal("guest stayed inside a folded tent")
	}
	if owner.openTent != nil {
		t.Fatal("owner tent still open")
	}
}
