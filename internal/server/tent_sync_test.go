package server

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestTickManufacturingExchangesTentOccupants(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner, guest := players[0], players[1]
	enterTestTent(t, s, guest, owner.character.ID)
	owner.tentOccupantsSync = true
	owner.tentSyncAt = time.Now().Add(-time.Second)
	wires[0].Reset()
	wires[1].Reset()
	s.worldMu.Lock()
	s.tickManufacturing(context.Background(), time.Now())
	s.worldMu.Unlock()
	if owner.tentOccupantsSync {
		t.Fatal("occupant exchange flag was not cleared")
	}
	if !hasPrefix(wires[0].packets(t), []byte{protocol.CommandAppearance}) {
		t.Fatal("owner did not receive the guest appearance", wires[0].packets(t))
	}
	if !hasPrefix(wires[1].packets(t), []byte{protocol.CommandAppearance}) {
		t.Fatal("guest did not receive the owner appearance", wires[1].packets(t))
	}
}

func TestTickManufacturingRepeatsTentSign(t *testing.T) {
	s, players, wires := tentLevelFixture(t)
	owner, bystander := players[0], players[2]
	sign := tentSign(owner)
	bystander.tentSignSync = true
	wires[2].Reset()
	s.worldMu.Lock()
	s.tickManufacturing(context.Background(), time.Now())
	s.worldMu.Unlock()
	if bystander.tentSignSync {
		t.Fatal("tent sign resend flag was not cleared")
	}
	if !contains(wires[2].packets(t), sign) {
		t.Fatal("tent sign was not repeated", wires[2].packets(t))
	}
	wires[2].Reset()
	s.worldMu.Lock()
	s.tickManufacturing(context.Background(), time.Now())
	s.worldMu.Unlock()
	if len(wires[2].packets(t)) != 0 {
		t.Fatal("tent sign was repeated twice", wires[2].packets(t))
	}
}

func TestTentPeersIncludesLoadingSessionsInOrder(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner, guest, loading := players[0], players[1], players[2]
	enterTestTent(t, s, guest, owner.character.ID)
	next := loading.character.Clone()
	next.Map = game.TentMapID
	loading.character = &next
	loading.tentOwner = owner.character.ID
	loading.tentFloor = 0
	s.worldMu.Lock()
	delete(s.world, loading.info.ID)
	s.worldMu.Unlock()
	s.mu.Lock()
	s.sessions[loading.info.ID] = loading
	s.mu.Unlock()
	peers := s.tentPeers(owner)
	if len(peers) != 2 || peers[0] != guest || peers[1] != loading {
		t.Fatalf("loading session missing or order wrong: %v", peers)
	}
}

func TestTentFloorChatCommands(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner := players[0]
	owner.gmLevel.Store(1)
	say(t, s, owner, "/floorup")
	if owner.tentFloor != 1 || !owner.tentOccupantsSync {
		t.Fatal("floorup did not switch and defer the exchange")
	}
	say(t, s, owner, "/floordown")
	if owner.tentFloor != 0 {
		t.Fatal("floordown did not return to the ground floor")
	}
}

func TestTentExitFromUpperFloorGoesDownFirst(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner := players[0]
	owner.tentFloor = 1
	tradeDo(t, s, owner, []byte{65, 3, 0})
	if owner.tentFloor != 0 || owner.tentOwner == 0 || owner.character.Map != game.TentMapID {
		t.Fatalf("upper-floor exit skipped the ground floor: floor=%d owner=%d map=%d", owner.tentFloor, owner.tentOwner, owner.character.Map)
	}
	tradeDo(t, s, owner, []byte{65, 3, 0})
	if owner.tentOwner != 0 || owner.character.Map == game.TentMapID {
		t.Fatal("second exit did not leave the tent", owner.character.Map, owner.tentOwner)
	}
}

func TestTentPortalFromUpperFloorGoesDownFirst(t *testing.T) {
	s, players, _ := tentLevelFixture(t)
	owner := players[0]
	owner.tentFloor = 1
	tradeDo(t, s, owner, []byte{20, protocol.EventPortal, 1, 0})
	if owner.tentFloor != 0 || owner.tentOwner == 0 {
		t.Fatal("portal exit skipped the ground floor")
	}
	tradeDo(t, s, owner, []byte{20, protocol.EventPortal, 1, 0})
	if owner.tentOwner != 0 {
		t.Fatal("portal did not leave the tent")
	}
}

func TestTentUpperFloorPlacementUsesSessionLevel(t *testing.T) {
	s, players, wires, path := tentFixturePath(t)
	owner := players[0]
	tradeDo(t, s, owner, []byte{23, 96, 1})
	enterTestTent(t, s, owner, owner.character.ID)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("UPDATE tents SET type = 2 WHERE owner_id = ?", owner.character.ID); err != nil {
		t.Fatal(err)
	}
	owner.tentFloor = 1
	wires[0].Reset()
	tradeDo(t, s, owner, protocol.Builder{62, 1, 2}.U32(29).U32(38).U32(1))
	tent, err := s.Store.Tent(context.Background(), owner.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row := tent.Items[len(tent.Items)-1]; row.Floor != 1 {
		t.Fatalf("furniture stored on floor %d", row.Floor)
	}
	found := false
	for _, packet := range wires[0].packets(t) {
		if len(packet) == 2+protocol.TentTreeRecordBytes && packet[0] == protocol.CommandTentFurniture &&
			packet[1] == protocol.TentPlaceFurniture && packet[2+49] == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("confirmation lacks the upper level", wires[0].packets(t))
	}
}
