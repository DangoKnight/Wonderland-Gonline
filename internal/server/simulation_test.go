package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestWorldSimulationMovementRejectionPreservesState(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: make([]byte, 10000)}}
	s.Assets.Terrains[c.character.Map].Cells[60*100+65] = 23
	s.World = world.New(s.Assets)
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	x, y := c.character.X, c.character.Y
	c.arrived = true
	request := protocol.Builder{6, 1, 2}.U16(1200).U16(1300)
	if err := s.worldCommand(ctx, c, request); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], c.character.PositionPacket()) || c.character.X != x || c.character.Y != y || !c.arrived {
		t.Fatal("rejection changed session", packets)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved[0].X != x || saved[0].Y != y {
		t.Fatal("rejected coordinates persisted")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1100).U16(1100)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1100 || c.character.Y != 1100 || c.arrived {
		t.Fatal("valid movement rejected")
	}
}

func TestWorldSimulationVisibilityAndInstanceIsolation(t *testing.T) {
	s, players, wires := worldFixture(t)
	mapID := players[0].character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 7, Template: 17001, Name: "Rabbit", Flags: 1, X: 100, Y: 100, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100, Delay: 2000}}}}}
	s.Assets.NPCs = map[uint16]assets.NPC{17001: {ID: 17001, Name: "Rabbit"}}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
		wires[i].Reset()
	}
	players[1].view.Hidden[7] = true
	now := time.Unix(1000, 0)
	s.simulateWorld(now)
	s.simulateWorld(now.Add(8 * time.Second))
	want := []byte{22, 2, 7, 0, 200, 0, 100, 0, 2}
	if got := wires[0].packets(t); len(got) != 1 || !bytes.Equal(got[0], want) {
		t.Fatal(got)
	}
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("hidden or foreign-map viewer received movement")
	}
	wires[0].Reset()
	players[0].event = &eventSession{click: 7}
	s.simulateWorld(now.Add(11 * time.Second))
	if wires[0].Len() != 0 {
		t.Fatal("scripted actor moved")
	}
	players[0].event = nil
	players[0].tentOwner = players[0].character.ID
	s.simulateWorld(now.Add(20 * time.Second))
	if wires[0].Len() != 0 {
		t.Fatal("world update leaked into tent")
	}
	players[0].tentOwner = 0
	s.simulateWorld(now.Add(30 * time.Second))
	if len(wires[0].packets(t)) != 1 {
		t.Fatal("actor did not resume")
	}
}

func TestWorldSimulationFailedRecipientIsIsolated(t *testing.T) {
	s, players, wires := worldFixture(t)
	mapID := players[0].character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 7, Template: 17001, Name: "Rabbit", Flags: 1, X: 100, Y: 100, WalkBehavior: 2, WalkSteps: []assets.WalkStep{{X: 200, Y: 100}}}}}
	s.Assets.NPCs = map[uint16]assets.NPC{17001: {ID: 17001, Name: "Rabbit"}}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
		wires[i].Reset()
	}
	failed := &failedWorldConn{}
	players[0].conn = failed
	now := time.Unix(1, 0)
	s.simulateWorld(now)
	s.simulateWorld(now.Add(8 * time.Second))
	if !failed.closed || len(wires[1].packets(t)) != 1 || !players[1].ready {
		t.Fatal("recipient failure stopped simulation")
	}
}

func TestWorldSimulationVehicleTravelKeepsSceneBounds(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	c.ready = true
	s.world[c.info.ID] = c
	c.character.Bag[0] = game.Item{ID: 48001, Count: 1}
	c.character.ActiveVehicle = 48001
	c.character.VehicleSlot = 1
	s.Assets.Items[48001] = game.ItemDefinition{ID: 48001, Type: game.VehicleType}
	cells := make([]byte, 10000)
	cells[60*100+65] = 23
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: cells}}
	s.World = world.New(s.Assets)
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1200).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 || c.character.Y != 1300 {
		t.Fatal("land mask blocked valid vehicle")
	}
	wires[0].Reset()
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(2000).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 || len(wires[0].packets(t)) != 1 {
		t.Fatal("vehicle escaped bounds")
	}
	c.character.ActiveVehicle = 48002
	// An unowned riding flag does not bypass the blocked destination.
	cells[61*100+65] = 23
	if err := s.worldCommand(ctx, c, protocol.Builder{6, 1, 2}.U16(1220).U16(1300)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 1200 {
		t.Fatal("stale vehicle bypassed collision")
	}
}
