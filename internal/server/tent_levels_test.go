package server

import (
	"bytes"
	"context"
	"testing"
	"time"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

// tentLevelFixture opens the owner's tent and puts a stairs item plus a wave
// floor covering inside, so level switches can be exercised end to end.
func tentLevelFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	s, players, wires := tentFixture(t)
	s.Assets.Items[39031] = game.ItemDefinition{ID: 39031, Name: "Stairs", Type: 30, CellWidth: 1, CellHeight: 1}
	s.Assets.Items[39506] = game.ItemDefinition{ID: 39506, Name: "Wave Floor", Type: 30, CellWidth: 1, CellHeight: 1}
	if s.Assets.TentSurfaces == nil {
		s.Assets.TentSurfaces = map[uint16]byte{}
	}
	s.Assets.TentSurfaces[39506] = assets.TentSurfaceFloor
	owner := players[0]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	enterTestTent(t, s, owner, owner.character.ID)
	next := owner.character.Clone()
	next.Bag[2] = game.Item{ID: 39031, Count: 1}
	next.Bag[3] = game.Item{ID: 39506, Count: 1}
	if err := s.commit(context.Background(), owner, next); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, owner, protocol.Builder{62, 1, 3}.U32(40).U32(40).U32(0).U8(0))
	tradeDo(t, s, owner, protocol.Builder{62, 1, 4}.U32(0).U32(0).U32(0).U8(1))
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func TestTentFloorClickSwitchesLevelAndDebounces(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner := players[0]
	tent, err := s.Store.Tent(context.Background(), owner.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tent.Items) != 2 {
		t.Fatalf("tent items %d", len(tent.Items))
	}
	stairs := uint16(len(tent.Items))
	tradeDo(t, s, owner, protocol.Builder{62, 14}.U16(stairs))
	if owner.tentFloor != 1 {
		t.Fatal("stairs did not switch the level")
	}
	packets := wires[0].packets(t)
	if !contains(packets, protocol.Builder{protocol.CommandPosition}.U32(owner.character.ID).U16(game.TentMapID).U16(460).U16(700).U8(1)) {
		t.Fatal("level missing from the position packet", packets)
	}
	if !hasPrefix(packets, []byte{62, 14}) {
		t.Fatal("floor surface missing")
	}
	// The client repeats the activation; a second press without moving is ignored.
	tradeDo(t, s, owner, protocol.Builder{62, 14}.U16(stairs))
	if owner.tentFloor != 1 {
		t.Fatal("repeated activation toggled the level")
	}
	// Stepping away and pressing again returns to the ground floor.
	next := owner.character.Clone()
	next.X += 100
	if err := s.commit(context.Background(), owner, next); err != nil {
		t.Fatal(err)
	}
	owner.tentFloorClickAt = owner.tentFloorClickAt.Add(-3 * time.Second)
	tradeDo(t, s, owner, protocol.Builder{62, 14}.U16(stairs))
	if owner.tentFloor != 0 {
		t.Fatal("second press did not return to the ground floor")
	}
}

func TestTentFloorClickIgnoresOtherFurniture(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner := players[0]
	tradeDo(t, s, owner, protocol.Builder{62, 14}.U16(1))
	if owner.tentFloor != 0 {
		t.Fatal("a workbench switched the level")
	}
}

func TestTentElevatorSwitchesLevel(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner := players[0]
	s.Assets.Items[39014] = game.ItemDefinition{ID: 39014, Name: "Elevator", Type: 29, CellWidth: 3, CellHeight: 5}
	next := owner.character.Clone()
	next.Bag[4] = game.Item{ID: 39014, Count: 1}
	if err := s.commit(context.Background(), owner, next); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, owner, protocol.Builder{62, 1, 5}.U32(29).U32(38).U32(1))
	tent, err := s.Store.Tent(context.Background(), owner.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	tradeDo(t, s, owner, protocol.Builder{62, 14}.U16(uint16(len(tent.Items))))
	if owner.tentFloor != 1 {
		t.Fatal("elevator did not switch the level")
	}
	if !contains(wires[0].packets(t), protocol.Builder{protocol.CommandPosition}.U32(owner.character.ID).U16(game.TentMapID).U16(460).U16(700).U8(1)) {
		t.Fatal("elevator position missing", wires[0].packets(t))
	}
}

func TestTentPeersSeparateLevels(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	enterTestTent(t, s, guest, owner.character.ID)
	if got := len(s.tentPeers(owner)); got != 1 {
		t.Fatalf("ground occupants %d", got)
	}
	guest.tentFloor = 1
	if got := len(s.tentPeers(owner)); got != 0 {
		t.Fatalf("upper guest listed on the ground floor: %d", got)
	}
	if sameScene(owner, guest) {
		t.Fatal("levels shared one scene")
	}
}

func TestTentFloorSwitchDespawnsPreviousLevel(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	enterTestTent(t, s, guest, owner.character.ID)
	tent, err := s.Store.Tent(context.Background(), owner.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	if err := s.sendFloorSwitch(guest, tent, 1); err != nil {
		t.Fatal(err)
	}
	if !guest.tentOccupantsSync {
		t.Fatal("occupant exchange was not deferred")
	}
	despawn := wires[0].packets(t)
	wanted := protocol.Builder{protocol.CommandMapAcknowledgment}.U32(guest.character.ID).U16(game.TentUpperMapID)
	found := false
	for _, packet := range despawn {
		if len(packet) == 18 && packet[0] == 12 && packet[2] == wanted[2] && packet[3] == wanted[3] {
			found = true
		}
	}
	if !found {
		t.Fatal("ground observer did not get a warp away", despawn)
	}
}

func TestTentSnapshotCarriesOnlyCurrentLevel(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner := players[0]
	owner.tentFloor = 1
	wires[0].Reset()
	if err := s.tentSnapshot(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	layout := []byte{protocol.CommandTent, protocol.TentInteriorLayout, 1, 1}
	if !contains(packets, layout) {
		t.Fatal("upper layout header missing", packets)
	}
	ground := []byte{protocol.CommandTent, protocol.TentInteriorLayout, 1, 0}
	if contains(packets, ground) {
		t.Fatal("ground layout header leaked into the upper snapshot")
	}
	if !hasPrefix(packets, []byte{62, 14}) {
		t.Fatal("surface missing")
	}
}

func TestEnterAnotherTentKeepsOwnTentOpen(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	tradeDo(t, s, guest, []byte{23, 96, 1})
	if guest.openTent == nil {
		t.Fatal("guest tent did not open")
	}
	enterTestTent(t, s, guest, owner.character.ID)
	if guest.openTent == nil {
		t.Fatal("entering another tent folded the visitor's own tent")
	}
}

func TestClosePlayerTentBroadcastsClosed(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	wires[1].Reset()
	tradeDo(t, s, owner, []byte{65, 2, 0})
	closed := protocol.Builder{protocol.CommandTent, protocol.TentClosed}.U32(owner.character.ID)
	if !contains(wires[1].packets(t), closed) {
		t.Fatal("tent close was not broadcast", wires[1].packets(t))
	}
	if guest.character.Map != 12000 {
		t.Fatal("bystander left their map")
	}
}

func TestServerRegistersTentMaps(t *testing.T) {
	s, _, _ := tentFixture(t)
	for _, id := range []uint16{game.TentMapID, game.TentUpperMapID} {
		if _, ok := s.World.Map(id); !ok {
			t.Fatalf("tent map %d missing", id)
		}
	}
	if _, ok := s.Assets.Maps[game.TentUpperMapID]; !ok {
		t.Fatal("upper tent map not registered in the catalog")
	}
}

func hasPrefix(packets [][]byte, prefix []byte) bool {
	for _, packet := range packets {
		if bytes.HasPrefix(packet, prefix) {
			return true
		}
	}
	return false
}
