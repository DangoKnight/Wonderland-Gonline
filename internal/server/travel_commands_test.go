package server

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
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
	c.lastWarp = time.Time{}
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
