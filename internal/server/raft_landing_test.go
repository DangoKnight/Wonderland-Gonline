package server

import (
	"context"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestNewbieRaftPortalArrivalPreservesMountedRaft(t *testing.T) {
	// Newbie Island's authored warp has click ID one. Edge area three, as
	// captured on Legacy, resolves through the single-destination fallback.
	for _, portal := range []uint16{1, 3} {
		t.Run(string(rune('0'+portal)), func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			vehicleDo(t, s, c, 7, 2, 48016)
			s.Assets.Maps[10003] = assets.Map{ID: 10003, Warps: []assets.Warp{{ClickID: 1, MapID: 11016, X: 134, Y: 1126}}}
			s.Assets.Maps[11016] = assets.Map{ID: 11016}
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Map, next.X, next.Y = 10003, 4500, 3000
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			c.arrived = false
			wires[0].Reset()
			if err := s.enterPortalStep(context.Background(), c, portal, true); err != nil {
				t.Fatal(err)
			}
			if c.character.Map != 11016 || c.character.ActiveVehicle != 48016 || c.character.Bag[1].Count != 1 {
				t.Fatal("arrival consumed or dismounted raft")
			}
			if contains(wires[0].packets(t), protocol.Builder{15, 15}.U32(c.character.ID).U16(48016)) {
				t.Fatal("arrival triggered break")
			}
			assertTravelSaved(t, s, c, 11016, 134, 1126)
		})
	}
}

func TestStarterBeachRaftPortalAndLandingFailure(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	next := c.character.Clone()
	next.Map, next.X, next.Y = 11016, 350, 980
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Maps[11016] = assets.Map{ID: 11016, Warps: []assets.Warp{{ClickID: 2, MapID: 60000, X: 1857, Y: 2602}}}
	s.Assets.Maps[60000] = assets.Map{ID: 60000}
	s.World = world.New(s.Assets)
	c.arrived = false
	wires[0].Reset()
	if err := s.usePortal(context.Background(), c, []byte{20, 8, 2, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.Map != 11016 || c.character.ActiveVehicle != 48016 || !contains(wires[0].packets(t), []byte{20, 8}) {
		t.Fatal("water exit escaped Starter Beach")
	}
	// A failed SQL landing must retain the owned raft and publish no break.
	s.Assets.Terrains = map[uint16]assets.Terrain{11016: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: make([]byte, 10000)}}
	wires[0].Reset()
	s.Store.Close()
	if err := s.vehicleCommand(context.Background(), c, vehicleRequest(10, 2, 48016)); err == nil {
		t.Fatal("expected failed landing commit")
	}
	if c.character.ActiveVehicle != game.DisposableRaftItemID || c.character.Bag[1].Count != 1 || wires[0].Len() != 0 {
		t.Fatal("failed landing consumed or published raft break")
	}
}
