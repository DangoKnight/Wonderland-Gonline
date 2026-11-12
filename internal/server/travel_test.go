package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func travelPacket(text string) []byte { return append([]byte{2, 2}, text...) }
func addTravelMaps(s *Server, ids ...uint16) {
	for _, id := range ids {
		s.Assets.Maps[id] = assets.Map{ID: id}
	}
	s.World = world.New(s.Assets)
}
func acknowledgeTravel(t *testing.T, s *Server, c *Session) {
	t.Helper()
	if err := s.dispatch(context.Background(), c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
}
func assertTravelSaved(t *testing.T, s *Server, c *Session, mapID, x, y uint16) {
	t.Helper()
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if chars[0].Map != mapID || chars[0].X != x || chars[0].Y != y || c.character.Map != mapID || c.character.X != x || c.character.Y != y {
		t.Fatalf("destination not persisted/adopted: %+v", chars[0])
	}
}

func TestPublicCarnieCommandAndReturn(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	addTravelMaps(s, 11094)
	if err := s.dispatch(context.Background(), c, travelPacket(":carnie")); err != nil {
		t.Fatal(err)
	}
	assertTravelSaved(t, s, c, 11094, 1180, 875)
	if c.gmLevel.Load() != 0 || c.ready || c.carnieReturn == nil || *c.carnieReturn != (world.Destination{Map: 10017, X: 1042, Y: 1075}) {
		t.Fatal("public travel or return state incorrect")
	}
	// Independent wire layout: AC12 owner, destination, portal zero, flags zero.
	want := protocol.Builder{12}.U32(c.character.ID).Bytes([]byte{86, 43, 156, 4, 107, 3, 0, 0, 0})
	if !contains(wires[0].packets(t), want) {
		t.Fatal("missing native Carnie warp")
	}
	acknowledgeTravel(t, s, c)
	if err := s.dispatch(context.Background(), c, travelPacket("/carnie")); err != nil {
		t.Fatal(err)
	}
	if c.carnieReturn.Map != 10017 {
		t.Fatal("repeated visit overwrote return point")
	}
	acknowledgeTravel(t, s, c)
	c.arrived = false
	c.character.X, c.character.Y = 600, 600
	if err := s.dispatch(context.Background(), c, []byte{20, 8, 1, 0}); err != nil {
		t.Fatal(err)
	}
	assertTravelSaved(t, s, c, 10017, 1042, 1075)
}

func TestPublicCarnieSaveFailureAndMissingDestination(t *testing.T) {
	for _, menu := range []bool{false, true} {
		s, players, wires := chatFixture(t)
		c := players[0]
		memo := &world.Destination{Map: 20000, X: 70, Y: 80}
		c.carnieReturn = memo
		// Missing destination must not change a previous return point.
		packet := travelPacket(":carnie")
		if menu {
			packet = []byte{5, 17, 3}
		}
		if err := s.dispatch(context.Background(), c, packet); err != nil || wires[0].Len() != 0 || c.carnieReturn != memo {
			t.Fatal("missing destination mutated state", err)
		}
		addTravelMaps(s, 11094)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := s.dispatch(ctx, c, packet); !errors.Is(err, context.Canceled) {
			t.Fatal("save failure not returned", err)
		}
		assertTravelSaved(t, s, c, 10017, 1042, 1075)
		if c.carnieReturn != memo || !c.ready {
			t.Fatal("failed save changed runtime ownership")
		}
		for _, p := range wires[0].packets(t) {
			if p[0] == 12 {
				t.Fatal("failed save emitted warp success")
			}
		}
	}
}

func TestPublicVehicleDismountAliases(t *testing.T) {
	for _, alias := range []string{":unride", "/unride", ":dismount", "/dismount"} {
		t.Run(alias, func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			vehicleDo(t, s, c, 7, 2, 48016)
			before := c.character.Bag
			for _, w := range wires {
				w.Reset()
			}
			if err := s.dispatch(context.Background(), c, travelPacket(alias)); err != nil {
				t.Fatal(err)
			}
			expected := protocol.Builder{15, 11, 2}.U32(c.character.ID)
			for _, w := range wires[:2] {
				if got := w.packets(t); len(got) != 1 || !bytes.Equal(got[0], expected) {
					t.Fatal("dismount not replicated", got)
				}
			}
			if c.character.ActiveVehicle != 0 || c.character.VehicleSlot != 0 || c.character.Bag != before {
				t.Fatal("dismount consumed or changed the raft")
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].ActiveVehicle != 0 || chars[0].Bag != before {
				t.Fatal("dismount not durable", err)
			}
		})
	}
}

func TestPublicDismountShoreAndCompanion(t *testing.T) {
	s, c, wires := mountFixture(t)
	next := c.character.Clone()
	next.ActiveMount = 12178
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.dispatch(context.Background(), c, travelPacket(":unride")); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveMount != 12178 || wires[0].Len() != 0 {
		t.Fatal("vehicle command rested a pet")
	}
	addTravelMaps(s, 10036)
	if err := s.commandTeleport(context.Background(), c, world.Destination{Map: 10036, X: 100, Y: 200}); err != nil {
		t.Fatal(err)
	}
	acknowledgeTravel(t, s, c)
	if err := s.dispatch(context.Background(), c, travelPacket("/dismount")); err != nil {
		t.Fatal(err)
	}
	assertTravelSaved(t, s, c, 10036, 1038, 2235)
	if c.ready {
		t.Fatal("shore relocation skipped map load")
	}
}

func TestTravelCommandsRespectInteractionOwnership(t *testing.T) {
	for _, gate := range []string{"loading", "battle", "trade", "event", "minigame", "storm", "beach"} {
		t.Run(gate, func(t *testing.T) {
			s, players, wires := chatFixture(t)
			c := players[0]
			addTravelMaps(s, 11094)
			next := c.character.Clone()
			s.Assets.Items[48016] = game.ItemDefinition{ID: 48016, Type: game.VehicleType}
			next.Bag[1] = game.Item{ID: 48016, Count: 1}
			next.ActiveVehicle, next.VehicleSlot = 48016, 2
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			switch gate {
			case "loading":
				c.ready = false
				c.warped = true
			case "battle":
				c.battle = &battleRun{}
			case "trade":
				c.trade = &tradeSession{}
			case "event":
				c.event = &eventSession{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { return nil }}
			case "storm":
				c.storm = true
			case "beach":
				c.beach = &beachRun{}
			}
			for _, w := range wires {
				w.Reset()
			}
			for _, cmd := range []string{":carnie", ":unride"} {
				if err := s.dispatch(context.Background(), c, travelPacket(cmd)); err != nil {
					t.Fatal(err)
				}
			}
			if gate != "trade" {
				s.SetGMLevel(c.account.ID, 1)
				for _, cmd := range []string{":town welling", ":warp 11094", ":summon Bobby", ":summonall"} {
					if err := s.dispatch(context.Background(), c, travelPacket(cmd)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if c.character.Map != 10017 || c.character.ActiveVehicle != 48016 || c.carnieReturn != nil {
				t.Fatal("command bypassed interaction")
			}
			for _, w := range wires {
				if w.Len() != 0 {
					t.Fatal("blocked travel produced packets")
				}
			}
		})
	}
}

func TestGMTownDestinationsAndPermissions(t *testing.T) {
	s, players, _ := chatFixture(t)
	c := players[0]
	// Independent authored coordinate fixtures include both Chang'an aliases.
	cases := []struct {
		key         string
		mapID, x, y uint16
	}{
		{"welling", 10001, 800, 750}, {"kelan", 10011, 1000, 1000}, {"holy", 10016, 1200, 950},
		{"kyoto", 10041, 1300, 1100}, {"changan", 10051, 1400, 1200}, {"chang_an", 10051, 1400, 1200},
		{"rome", 10061, 1100, 1000}, {"maya", 10071, 900, 900}, {"inca", 10081, 800, 850},
		{"bangkok", 10091, 1200, 1100}, {"southpole", 10021, 1000, 1000}, {"ghostisle", 10026, 800, 800},
		{"carnie", 11094, 1180, 875}, {"pirate", 10036, 1038, 2235}, {"kaohsiung", 10003, 1000, 1000}, {"jail", 10000, 600, 600},
	}
	for _, tc := range cases {
		addTravelMaps(s, tc.mapID)
	}
	if err := s.dispatch(context.Background(), c, travelPacket(":town welling")); err != nil {
		t.Fatal(err)
	}
	if c.character.Map != 10017 {
		t.Fatal("non-GM town warp")
	}
	s.SetGMLevel(c.account.ID, 1)
	for _, tc := range cases {
		if err := s.dispatch(context.Background(), c, travelPacket("/town "+tc.key)); err != nil {
			t.Fatal(err)
		}
		assertTravelSaved(t, s, c, tc.mapID, tc.x, tc.y)
		acknowledgeTravel(t, s, c)
	}
	for _, text := range []string{":town", ":town unknown"} {
		if err := s.dispatch(context.Background(), c, travelPacket(text)); err != nil {
			t.Fatal(err)
		}
		assertTravelSaved(t, s, c, 10000, 600, 600)
	}
}

func TestGMSummonAllSnapshotAndTargetGates(t *testing.T) {
	s, players, _ := chatFixture(t)
	gm, a, b := players[0], players[1], players[2]
	if err := s.dispatch(context.Background(), gm, travelPacket(":summonall")); err != nil {
		t.Fatal(err)
	}
	if b.character.Map != 20000 {
		t.Fatal("non-GM summoned players")
	}
	s.SetGMLevel(gm.account.ID, 1)
	a.battle = &battleRun{}
	if err := s.dispatch(context.Background(), gm, travelPacket("/summonall")); err != nil {
		t.Fatal(err)
	}
	if !a.ready || a.battle == nil {
		t.Fatal("summon bypassed battle ownership")
	}
	assertTravelSaved(t, s, b, 10017, 1042, 1075)
	if b.ready || !gm.ready {
		t.Fatal("wrong session entered map loading")
	}
	acknowledgeTravel(t, s, b)
	a.battle = nil
	if err := s.dispatch(context.Background(), gm, travelPacket(":summonall")); err != nil {
		t.Fatal(err)
	}
	if a.ready || b.ready || !gm.ready {
		t.Fatal("iteration lost a target while deleting published sessions")
	}
	assertTravelSaved(t, s, a, 10017, 1042, 1075)
	assertTravelSaved(t, s, b, 10017, 1042, 1075)
}

func TestSummonAllContinuesAfterFailedRecipient(t *testing.T) {
	s, players, _ := chatFixture(t)
	gm, failed, other := players[0], players[1], players[2]
	conn := &failedWorldConn{}
	failed.conn = conn
	s.SetGMLevel(gm.account.ID, 1)
	if err := s.dispatch(context.Background(), gm, travelPacket(":summonall")); err != nil {
		t.Fatal(err)
	}
	if !conn.closed || !gm.ready {
		t.Fatal("failed recipient was not isolated")
	}
	assertTravelSaved(t, s, other, 10017, 1042, 1075)
	if other.ready {
		t.Fatal("later recipient was skipped")
	}
}

func TestConcurrentPublicTravelAndSummon(t *testing.T) {
	s, players, _ := chatFixture(t)
	gm, c := players[0], players[1]
	addTravelMaps(s, 11094)
	s.SetGMLevel(gm.account.ID, 1)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, actor := range []*Session{gm, c} {
		wg.Add(1)
		go func(actor *Session) {
			defer wg.Done()
			for i := 0; i < 12; i++ {
				text := ":carnie"
				if actor == gm {
					text = ":summonall"
				}
				if err := s.dispatch(context.Background(), actor, travelPacket(text)); err != nil {
					errs <- err
					return
				}
				if actor == c {
					if err := s.dispatch(context.Background(), c, []byte{12, 1}); err != nil {
						errs <- err
						return
					}
				}
			}
		}(actor)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	assertTravelSaved(t, s, c, c.character.Map, c.character.X, c.character.Y)
}

func TestTeleportScrollRecordAndAtomicTravel(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.Travel.Scrolls = map[uint16]byte{34163: 1}
	s.Assets.Items[34163] = game.ItemDefinition{ID: 34163, Type: 10}
	c.character.Bag[1] = game.Item{ID: 34163, Count: 2}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{74, 1, 2, 1})
	points, err := s.Store.TeleportPoints(context.Background(), store.CharacterRef{Account: c.account.ID, ID: c.character.ID})
	if err != nil || points[1].Map != c.character.Map || points[1].X != c.character.X {
		t.Fatal("record", points, err)
	}
	oldX := c.character.X
	c.character.X += 20 // Already validated pending walking, still on the same map.
	wires[0].Reset()
	tradeDo(t, s, c, []byte{74, 2, 2, 1})
	if c.character.X != oldX || c.character.Bag[1].Count != 1 || c.ready {
		t.Fatal("scroll travel")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].X != oldX || saved[0].Bag[1].Count != 1 {
		t.Fatal("debit and location not durable", err)
	}
	tradeDo(t, s, c, []byte{12, 1})
	s.Store.Close()
	wires[0].Reset()
	if err := s.dispatch(context.Background(), c, []byte{74, 2, 2, 1}); err == nil || c.character.Bag[1].Count != 1 || wires[0].Len() != 0 {
		t.Fatal("failed SQL published travel", err)
	}
}

func TestBambooDragonflyRejectsUnavailableDestination(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	s.Assets.Travel.BambooItems = []uint16{34169}
	s.Assets.Travel.Destinations = map[byte]assets.TravelDestination{1: {Map: 60000, X: 100, Y: 100}}
	s.Assets.Items[34169] = game.ItemDefinition{ID: 34169, Type: 10}
	c.character.Bag[1] = game.Item{ID: 34169, Count: 1}
	if err := s.commit(context.Background(), c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	tradeDo(t, s, c, []byte{23, 89, 1, 2, 0})
	if c.character.Bag[1].Count != 1 || c.character.Map != 10017 {
		t.Fatal("unavailable destination consumed item")
	}
	if len(wires[0].packets(t)) != 1 {
		t.Fatal("missing rejection")
	}
}

func TestKelanReturnScrollAtomicDebit(t *testing.T) {
	s, players, wires := tradeFixture(t)
	c := players[0]
	ctx := context.Background()
	s.Assets.Travel.ReturnScrolls = map[uint16]assets.TravelDestination{30001: {Map: 12000, X: 892, Y: 734}}
	s.Assets.Maps[12000] = assets.Map{ID: 12000}
	s.Assets.Items[30001] = game.ItemDefinition{ID: 30001, Type: 21}
	s.World = world.New(s.Assets)
	c.character.Bag[1] = game.Item{ID: 30001, Count: 2}
	if err := s.commit(ctx, c, c.character.Clone()); err != nil {
		t.Fatal(err)
	}
	if err := s.useItemOn(ctx, c, 2, 1, 0); err != nil || c.character.Map != 12000 || c.character.Bag[1].Count != 1 {
		t.Fatal("return", err)
	}
	saved, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || saved[0].Map != 12000 || saved[0].Bag[1].Count != 1 {
		t.Fatal("not durable", err)
	}
	if err := s.dispatch(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Store.Close()
	wires[0].Reset()
	if err := s.useItem(ctx, c, 2); err == nil || c.character.Bag[1].Count != 1 || wires[0].Len() != 0 {
		t.Fatal("failed debit published", err)
	}
}

func vehicleFixture(t *testing.T) (*Server, *Session, []*captureConn) {
	t.Helper()
	s, c, wires := mountFixture(t)
	for _, id := range []uint16{48001, 48010, 48016, 48028} {
		s.Assets.Items[id] = game.ItemDefinition{ID: id, Type: game.VehicleType}
	}
	next := c.character.Clone()
	next.Bag[1] = game.Item{ID: 48016, Count: 1}
	next.Bag[2] = game.Item{ID: 48016, Count: 1} // Another identical raft must survive.
	next.Bag[3] = game.Item{ID: 48001, Count: 1}
	next.Bag[4] = game.Item{ID: 48028, Count: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, c, wires
}

func vehicleRequest(sub, slot byte, id uint16) []byte { return protocol.Builder{15, sub, slot}.U16(id) }
func vehicleDo(t *testing.T, s *Server, c *Session, sub, slot byte, id uint16) {
	t.Helper()
	if err := s.dispatch(context.Background(), c, vehicleRequest(sub, slot, id)); err != nil {
		t.Fatal(err)
	}
}

func TestVehiclePlacementBoardingAndLanding(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	c.character.X, c.character.Y = 110, 110
	terrain := assets.Terrain{Width: 180, Height: 180, GridWidth: 9, GridHeight: 9, Cells: make([]byte, 81)}
	terrain.Cells[6*9+5], terrain.Cells[7*9+5] = 2, 2
	s.Assets.Terrains = map[uint16]assets.Terrain{c.character.Map: terrain}
	before := c.character.Bag
	vehicleDo(t, s, c, 9, 2, 48016)
	manual := protocol.Builder{15, 12, 2}.U32(c.character.ID).U16(48016).U32(130).U32(110)
	for _, wire := range wires[:2] {
		packets := wire.packets(t)
		if len(packets) != 1 || !bytes.Equal(packets[0], manual) {
			t.Fatal("inventory placement did not publish unoccupied raft", packets)
		}
	}
	if c.character.ActiveVehicle != 0 || c.character.Bag != before || wires[2].Len() != 0 {
		t.Fatal("manual placement boarded, consumed or crossed maps")
	}
	vehicleDo(t, s, c, 14, 2, 48016)
	want := protocol.Builder{15, 18, 2}.U32(c.character.ID).U16(48016).U32(uint32(c.character.X)).U32(uint32(c.character.Y))
	packets := wires[0].packets(t)
	if len(want) != 17 || len(packets) != 1 || !bytes.Equal(packets[0], want) || c.character.ActiveVehicle != 0 || wires[1].Len() != 0 {
		t.Fatal("placement boarded or replicated", packets)
	}
	vehicleDo(t, s, c, 7, 2, 48016)
	want = protocol.Builder{15, 10, 2}.U32(c.character.ID).U16(48016)
	for _, wire := range wires[:2] {
		p := wire.packets(t)
		if len(p) != 1 || !bytes.Equal(p[0], want) {
			t.Fatal("boarding packet", p)
		}
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].ActiveVehicle != 48016 || stored[0].VehicleSlot != 2 {
		t.Fatal("boarding not durable", err)
	}
	if !contains(s.peerPetPackets(c.character), want) {
		t.Fatal("vehicle missing in peer snapshot")
	}
	vehicleDo(t, s, c, 9, 2, 48016)
	if wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("repeated boarding replayed")
	}
	vehicleDo(t, s, c, 10, 2, 48010)
	if c.character.ActiveVehicle != 48016 || wires[0].Len() != 0 {
		t.Fatal("stale landing dismounted another vehicle")
	}
	vehicleDo(t, s, c, 10, 99, 48016) // Landing discriminator is not the inventory slot.
	p := wires[0].packets(t)
	if len(p) != 1 || !bytes.Equal(p[0], protocol.Builder{15, 11, 2}.U32(c.character.ID)) {
		t.Fatal("landing sequence", p)
	}
	if len(wires[1].packets(t)) != 1 || wires[2].Len() != 0 {
		t.Fatal("landing map isolation")
	}
	if c.character.Bag[1].ID != 48016 || c.character.Bag[2].ID != 48016 || c.character.ActiveVehicle != 0 {
		t.Fatal("landing consumed a raft")
	}
	stored, err = s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Bag[1].ID != 48016 || stored[0].ActiveVehicle != 0 {
		t.Fatal("landing not saved", err)
	}
	vehicleDo(t, s, c, 10, 99, 48016)
	if wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("landing callback replayed")
	}
}

func TestRaftManualPlacementRejectsInvalidTerrainWithoutPublishing(t *testing.T) {
	for _, name := range []string{"missing geometry", "solid ground", "isolated water", "distant water", "blocked corner", "alternate water", "raft capsule"} {
		t.Run(name, func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			c.character.X, c.character.Y = 110, 110
			terrain := assets.Terrain{Width: 180, Height: 180, GridWidth: 9, GridHeight: 9, Cells: make([]byte, 81)}
			valid := false
			slot, id := byte(2), uint16(game.DisposableRaftItemID)
			switch name {
			case "isolated water":
				terrain.Cells[6*9+5] = 2
			case "distant water":
				terrain.Cells[7*9+5], terrain.Cells[8*9+5] = 2, 2
			case "blocked corner":
				terrain.Cells[6*9+6], terrain.Cells[7*9+6] = 2, 2
				terrain.Cells[6*9+5], terrain.Cells[5*9+6] = 4, 4
			case "alternate water":
				terrain.Cells[6*9+5], terrain.Cells[7*9+5] = 8, 8
				valid = true
			case "raft capsule":
				terrain.Cells[6*9+5], terrain.Cells[7*9+5] = 2, 2
				slot, id = 5, 48028
				valid = true
			}
			s.Assets.Terrains = map[uint16]assets.Terrain{}
			if name != "missing geometry" {
				s.Assets.Terrains[c.character.Map] = terrain
			}
			before := c.character.Clone()
			vehicleDo(t, s, c, protocol.PetControlPlaceVehicleManual, slot, id)
			for _, wire := range wires[:2] {
				packets := wire.packets(t)
				if valid {
					if len(packets) != 1 {
						t.Fatal("missing valid placement", packets)
					}
				} else if len(packets) != 0 {
					t.Fatal("invalid placement published", packets)
				}
			}
			if c.character.Bag != before.Bag || c.character.X != before.X || c.character.Y != before.Y || c.character.ActiveVehicle != 0 || wires[2].Len() != 0 {
				t.Fatal("placement changed inventory, position or another map")
			}
		})
	}
}

func TestVehicleRejectsMalformedAndUnownedCommands(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	for _, p := range [][]byte{{15, 7}, {15, 7, 2}, {15, 14, 2, 0}, {15, 7, 2, 0, 0, 0}, {15, 13, 0}} {
		if err := s.worldCommand(context.Background(), c, p); err == nil {
			t.Fatal("accepted malformed vehicle command", p)
		}
	}
	for _, p := range [][]byte{vehicleRequest(7, 0, 48016), vehicleRequest(7, 51, 48016), vehicleRequest(7, 1, 48016), vehicleRequest(7, 2, 48010), vehicleRequest(14, 2, 48028)} {
		if err := s.worldCommand(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
	}
	c.character.Bag[1].Damage = 100
	vehicleDo(t, s, c, 7, 2, 48016)
	c.character.Bag[1].Damage = 0
	c.event = &eventSession{}
	vehicleDo(t, s, c, 7, 2, 48016)
	c.event = nil
	if c.character.ActiveVehicle != 0 || wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("invalid boarding changed state")
	}
}

func TestVehicleMovesAreBlockedAndRemovalDismounts(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	wires[0].Reset()
	wires[1].Reset()
	for _, p := range [][]byte{{23, 10, 2, 1, 8}, {23, 10, 3, 1, 2}} {
		if err := s.worldCommand(context.Background(), c, p); err != nil {
			t.Fatal(err)
		}
	}
	if c.character.Bag[1].ID != 48016 || c.character.Bag[2].ID != 48016 || wires[0].Len() != 0 {
		t.Fatal("boarded slot moved")
	}
	if err := s.openService(c, world.ServicePropsKeeper); err != nil {
		t.Fatal(err)
	}
	if err := s.worldCommand(context.Background(), c, []byte{30, 2, 2}); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Storage[0].ID != 48016 {
		t.Fatal("storage failed to release mount")
	}
	packet := protocol.Builder{15, 11, 2}.U32(c.character.ID)
	if !contains(wires[0].packets(t), packet) || !contains(wires[1].packets(t), packet) {
		t.Fatal("removal not replicated")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].ActiveVehicle != 0 || saved[0].VehicleSlot != 0 || !saved[0].Bag[1].Empty() || saved[0].Storage[0].ID != 48016 {
		t.Fatal("storage and dismount were not saved together", err)
	}
}

func TestVehicleStorageFailureKeepsMount(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	if err := s.openService(c, world.ServicePropsKeeper); err != nil {
		t.Fatal(err)
	}
	for _, wire := range wires {
		wire.Reset()
	}
	before := c.character.Clone()
	s.Store.Close()
	if err := s.worldCommand(context.Background(), c, []byte{30, 2, 2}); err == nil {
		t.Fatal("failed storage save accepted")
	}
	if c.character.Bag != before.Bag || c.character.Storage != before.Storage || c.character.ActiveVehicle != before.ActiveVehicle || c.character.VehicleSlot != before.VehicleSlot {
		t.Fatal("failed storage save changed mount or resources")
	}
	for _, wire := range wires {
		if wire.Len() != 0 {
			t.Fatal("failed storage save published a dismount")
		}
	}
}

func TestVehicleRaftWearAndShore(t *testing.T) {
	for _, id := range []uint16{48016, 48028, 48001} {
		t.Run(fmt.Sprint(id), func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			slot := byte(2)
			if id == 48028 {
				slot = 5
			}
			if id == 48001 {
				slot = 4
			}
			vehicleDo(t, s, c, 7, slot, id)
			wires[0].Reset()
			wires[1].Reset()
			move := protocol.Builder{6, 1, 0}.U16(c.character.X + 1).U16(c.character.Y)
			if err := s.worldCommand(context.Background(), c, move); err != nil {
				t.Fatal(err)
			}
			damage := byte(1)
			if id == 48001 || id == 48016 {
				damage = 0
			}
			if c.character.Bag[slot-1].Damage != damage {
				t.Fatal("wrong movement wear", id)
			}
			wires[0].Reset()
			wires[1].Reset()
			if err := s.worldCommand(context.Background(), c, move); err != nil {
				t.Fatal(err)
			}
			if c.character.Bag[slot-1].Damage != damage {
				t.Fatal("stationary packet wore vehicle")
			}
			if id == 48001 || id == 48016 {
				vehicleDo(t, s, c, 10, 99, id)
				if c.character.ActiveVehicle != 0 || c.character.Bag[slot-1].ID != id {
					t.Fatal("ordinary landing destroyed vehicle")
				}
				return
			}
			next := c.character.Clone()
			next.Bag[slot-1].Damage = 99
			if err := s.commit(context.Background(), c, next); err != nil {
				t.Fatal(err)
			}
			if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(c.character.X+1).U16(c.character.Y)); err != nil {
				t.Fatal(err)
			}
			if !c.character.Bag[slot-1].Empty() || c.character.ActiveVehicle != 0 {
				t.Fatal("exhausted raft retained")
			}
		})
	}
	s, c, _ := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	c.character.Map = 11016
	s.Assets.Terrains = map[uint16]assets.Terrain{11016: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: make([]byte, 10000)}}
	if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(280).U16(950)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Bag[2].ID != 48016 {
		t.Fatal("authored beach did not wreck exact raft")
	}
}

func TestVehiclesAndCompanionMountsExcludeEachOther(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 1}.U32(14156)); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	wires[1].Reset()
	vehicleDo(t, s, c, 7, 2, 48016)
	if c.character.ActiveMount != 0 || c.character.ActiveVehicle != 48016 {
		t.Fatal("boarding kept companion mount")
	}
	if !contains(wires[0].packets(t), protocol.Builder{15, 17}.U32(c.character.ID)) {
		t.Fatal("companion not unmounted")
	}
	wires[1].Reset()
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 48016 || c.character.ActiveMount != 0 || wires[1].Len() != 0 {
		t.Fatal("pet could be ridden while vehicle active")
	}
	vehicleDo(t, s, c, 10, 2, 48016)
	if err := s.worldCommand(context.Background(), c, protocol.Builder{15, 11, 2}.U32(12032)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || c.character.ActiveMount != 12178 || c.character.Bag[1].ID != 48016 {
		t.Fatal("companion mount kept or destroyed item vehicle")
	}
	if !contains(wires[1].packets(t), protocol.Builder{15, 11, 2}.U32(c.character.ID)) {
		t.Fatal("item dismount not sent")
	}
}

func TestVehicleSaveFailuresPublishNothing(t *testing.T) {
	for _, op := range []string{"board", "land", "wreck", "wear", "remove", "mount"} {
		t.Run(op, func(t *testing.T) {
			s, c, wires := vehicleFixture(t)
			if op != "board" && op != "mount" {
				vehicleDo(t, s, c, 7, 2, 48016)
			}
			if op == "wear" {
				vehicleDo(t, s, c, 7, 5, 48028)
			}
			for _, w := range wires {
				w.Reset()
			}
			before := c.character.Clone()
			s.Store.Close()
			var err error
			switch op {
			case "board":
				err = s.vehicleCommand(context.Background(), c, vehicleRequest(7, 2, 48016))
			case "land":
				err = s.vehicleCommand(context.Background(), c, vehicleRequest(10, 2, 48016))
			case "wreck":
				err = s.wreckVehicle(context.Background(), c)
			case "wear":
				err = s.wearVehicle(context.Background(), c)
			case "remove":
				err = s.destroyItem(context.Background(), c, 2, 1)
			case "mount":
				err = s.mountCommand(context.Background(), c, protocol.Builder{15, 11, 2}.U32(12032))
			}
			if err == nil || c.character.ActiveVehicle != before.ActiveVehicle || c.character.ActiveMount != before.ActiveMount || c.character.Bag != before.Bag || wires[0].Len() != 0 || wires[1].Len() != 0 {
				t.Fatal("failed save adopted or published vehicle mutation", err)
			}
		})
	}
}

func TestVehicleSnapshotRestoreAndPortalLanding(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	s.depart(c, c.character.WarpPacket(0))
	c.character = &stored[0]
	for _, w := range wires {
		w.Reset()
	}
	if err := s.acknowledgeWorld(c); err != nil {
		t.Fatal(err)
	}
	mount := serverwire.VehicleMountPacket(c.character)
	if !contains(wires[0].packets(t), mount) || !contains(wires[1].packets(t), mount) {
		t.Fatal("map acknowledgment did not restore vehicle")
	}
	// Robinson's raft cannot sail out of Starter Beach through a regular exit.
	s.Assets.Maps[11016] = assets.Map{ID: 11016, Warps: []assets.Warp{{ClickID: 1, MapID: 10017, X: 1042, Y: 1075}}}
	s.World = world.New(s.Assets)
	c.character.Map = 11016
	c.arrived = false
	for _, w := range wires {
		w.Reset()
	}
	if err := s.enterPortalStep(context.Background(), c, 1, true); err != nil {
		t.Fatal(err)
	}
	if c.character.Map != 11016 || c.character.ActiveVehicle != 48016 || c.character.Bag[1].ID != 48016 || c.character.Bag[2].ID != 48016 {
		t.Fatal("regular beach portal bypassed landing")
	}
	if !contains(wires[0].packets(t), []byte{20, 8}) || contains(wires[0].packets(t), protocol.Builder{15, 15}.U32(c.character.ID).U16(48016)) {
		t.Fatal("blocked portal did not resume or broke raft")
	}
}

func TestVehicleStaleBagSlotNeverConsumesAnotherItem(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	// Simulate stale imported/runtime state. No search for the other raft is allowed.
	c.character.Bag[1] = game.Item{ID: 48001, Count: 1}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.wreckVehicle(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || c.character.Bag[1].ID != 48001 || c.character.Bag[2].ID != 48016 {
		t.Fatal("wreck found an unrelated replacement")
	}
	p := wires[0].packets(t)
	if len(p) != 1 || !bytes.Equal(p[0], protocol.Builder{15, 11, 2}.U32(c.character.ID)) {
		t.Fatal("stale mount sent break/removal", p)
	}
}

func TestVehicleLoginAfterDatabaseReopen(t *testing.T) {
	s, c, _ := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	// A separate persisted DB exercises load/serialization and a fresh TCP session.
	path := filepath.Join(t.TempDir(), "reconnect.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.Register(context.Background(), "Alice", "password", "")
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	char := chars[0]
	char.ID = account.CharacterID(1)
	if err := db.CreateCharacter(context.Background(), account, char); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := New(config.Default(), db, s.Assets, s.Log)
	wire := &captureConn{}
	session := &Session{conn: wire, info: SessionInfo{ID: 10}}
	if err := server.dispatch(context.Background(), session, loginPayload("Alice")); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := server.dispatch(context.Background(), session, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if session.character.ActiveVehicle != 48016 || session.character.VehicleSlot != 2 || session.character.Bag[1].Count != 1 {
		t.Fatal("login lost or duplicated boarded item")
	}
	for _, p := range wire.packets(t) {
		if len(p) > 1 && p[0] == 15 && p[1] == 10 {
			t.Fatal("mount sent before map acknowledgment")
		}
	}
	if err := server.dispatch(context.Background(), session, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if !contains(wire.packets(t), serverwire.VehicleMountPacket(session.character)) {
		t.Fatal("reopened vehicle was not synchronized")
	}
}

func TestVehicleSwitchAndBattleGate(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	for _, w := range wires {
		w.Reset()
	}
	c.battle = &battleRun{}
	vehicleDo(t, s, c, 7, 4, 48001)
	c.battle = nil
	if c.character.ActiveVehicle != 48016 || wires[0].Len() != 0 {
		t.Fatal("boarded during battle")
	}
	vehicleDo(t, s, c, 7, 4, 48001)
	for _, w := range wires[:2] {
		p := w.packets(t)
		if len(p) != 2 || !bytes.Equal(p[0], protocol.Builder{15, 11, 2}.U32(c.character.ID)) || !bytes.Equal(p[1], protocol.Builder{15, 10, 4}.U32(c.character.ID).U16(48001)) {
			t.Fatal("switch ordering", p)
		}
	}
	if c.character.Bag[1].ID != 48016 || c.character.ActiveVehicle != 48001 {
		t.Fatal("switch consumed previous vehicle")
	}
}

func TestVehicleLandingStopPersistsLandWithoutExtraWear(t *testing.T) {
	s, c, _ := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	x, y := c.character.X+20, c.character.Y+20
	damage := c.character.Bag[1].Damage
	// Go reports the relocated landing point while mounted, before requesting
	// dismount. Native stop layout must not trigger raft wear or a beach wreck.
	stop := protocol.Builder{6, 2, 12}.U16(x).U16(y).Bytes(make([]byte, 8))
	if err := s.dispatch(context.Background(), c, stop); err != nil {
		t.Fatal(err)
	}
	if c.character.X != x || c.character.Y != y || c.character.ActiveVehicle != 48016 || c.character.Bag[1].Damage != damage {
		t.Fatal("landing stop changed resources or lost position")
	}
	vehicleDo(t, s, c, 10, 2, 48016)
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) != 1 {
		t.Fatal("landing state read", err)
	}
	if stored[0].X != x || stored[0].Y != y || stored[0].ActiveVehicle != 0 || stored[0].Bag[1].ID != 48016 || stored[0].Bag[2].ID != 48016 {
		t.Fatal("landing recovery position or raft retention incorrect")
	}
}

func TestVehicleLegacyLandingThenMovementPreservesRaft(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	vehicleDo(t, s, c, 7, 2, 48016)
	before := c.character.Bag
	for _, wire := range wires {
		wire.Reset()
	}
	// Raw request from the real-server trace. The native discriminator 12
	// need not match the authoritative owned inventory slot (2 here).
	if err := s.dispatch(context.Background(), c, []byte{15, 10, 12, 144, 187}); err != nil {
		t.Fatal(err)
	}
	if packets := wires[0].packets(t); len(packets) != 1 || !bytes.Equal(packets[0], serverwire.VehicleDismountPacket(&game.Character{ID: c.character.ID, VehicleSlot: 2})) {
		t.Fatal("normal landing emitted removal, wreck or refresh", packets)
	}
	// Native sends the shore coordinate as AC6:1 after dismount, not AC6:2.
	if err := s.dispatch(context.Background(), c, []byte{6, 1, 1, 184, 11, 32, 8, 2, 0, 235, 11, 233, 17, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if err := s.advancePlayerWalk(c, c.walk.started.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 0 || c.character.X != 3000 || c.character.Y != 2080 || c.character.Bag != before {
		t.Fatal("native landing lost position or changed inventory")
	}
	if err := s.autosaveSession(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) != 1 {
		t.Fatal("landing checkpoint", err)
	}
	if stored[0].ActiveVehicle != 0 || stored[0].X != 3000 || stored[0].Y != 2080 || stored[0].Bag != before {
		t.Fatal("landing checkpoint lost retained raft or land position")
	}
}

func TestStarterBeachRaftWaitsForLand(t *testing.T) {
	s, c, wires := vehicleFixture(t)
	cells := make([]byte, 100*100)
	for i := range cells {
		cells[i] = 2
	}
	cells[20*100+50] = 0
	s.Assets.Terrains = map[uint16]assets.Terrain{11016: {Width: 2000, Height: 2000, GridWidth: 100, GridHeight: 100, Cells: cells}}
	next := c.character.Clone()
	next.Map, next.X, next.Y = 11016, 480, 1000
	next.ActiveVehicle, next.VehicleSlot = 48016, 2
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.wearVehicle(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 48016 || c.character.Bag[1].ID != 48016 || wires[0].Len() != 0 {
		t.Fatal("raft broke while on water")
	}
	vehicleDo(t, s, c, 10, 2, 48016)
	if c.character.ActiveVehicle != 48016 || wires[0].Len() != 0 {
		t.Fatal("early dismount bypassed beach landing")
	}
	// Report clear shore through the real movement handler; consumption and
	// landing coordinates must commit together before the break receipts.
	s.Assets.Maps[11016] = assets.Map{ID: 11016}
	s.World = world.New(s.Assets)
	if err := s.movementCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(410).U16(1010).Bytes(make([]byte, 8))); err != nil {
		t.Fatal(err)
	}
	if c.character.ActiveVehicle != 48016 {
		t.Fatal("raft broke at announced destination before arrival")
	}
	for _, w := range wires {
		w.Reset()
	}
	if err := s.advancePlayerWalk(c, c.walk.started.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if c.character.X != 410 || c.character.Y != 1010 || c.character.ActiveVehicle != 0 || !c.character.Bag[1].Empty() || c.character.Bag[2].ID != 48016 {
		t.Fatalf("invalid beach landing: %+v", c.character)
	}
	packets := wires[0].packets(t)
	if len(packets) < 3 || !bytes.Equal(packets[0], []byte{23, 9, 2, 1}) || !bytes.Equal(packets[1], protocol.Builder{15, 15}.U32(c.character.ID).U16(48016)) || !bytes.Equal(packets[2], protocol.Builder{15, 11, 2}.U32(c.character.ID)) || contains(packets, []byte{5, 4}) {
		t.Fatal("incorrect captured break order or extra refresh", packets)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].X != 410 || stored[0].Y != 1010 || stored[0].ActiveVehicle != 0 || !stored[0].Bag[1].Empty() {
		t.Fatal("beach landing not durable", err)
	}
	for _, ack := range [][]byte{{15, 13}, {15, 13, 1, 0, 0, 0}} {
		if err := s.dispatch(context.Background(), c, ack); err != nil {
			t.Fatal("native dismount ACK rejected", err)
		}
	}
	if wires[0].Len() != 0 {
		t.Fatal("dismount ACK replayed break")
	}
}
