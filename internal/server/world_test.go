package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

func worldFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	catalog := creationCatalog()
	s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var sessions []*Session
	var wires []*captureConn
	for i, name := range []string{"Alice", "Bobby", "Carol"} {
		a, err := db.Register(context.Background(), name, "password", "")
		if err != nil {
			t.Fatal(err)
		}
		char, err := game.NewCharacter(a.CharacterID(1), 1, name, game.Appearance{Body: 1, Element: 3}, catalog.StarterItems, catalog.Items, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if i == 2 {
			char.Map = 20000
		}
		if err := db.CreateCharacter(context.Background(), a, char); err != nil {
			t.Fatal(err)
		}
		wire := &captureConn{}
		sessions = append(sessions, &Session{conn: wire, info: SessionInfo{ID: uint64(i + 1)}, account: a, character: &char, view: world.NewView(), pets: newPetRoster()})
		wires = append(wires, wire)
	}
	return s, sessions, wires
}

func TestWorldVisibilityMovementAndLogout(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	for _, i := range []int{0, 2} {
		if err := s.worldCommand(ctx, players[i], []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
		wires[i].Reset()
	}
	if err := s.worldCommand(ctx, players[1], []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	a, b := wires[0].packets(t), wires[1].packets(t)
	if len(a) != 4 || a[0][0] != 4 || !bytes.Equal(a[2][:2], []byte{10, 3}) || !bytes.Equal(a[3][:2], []byte{5, 8}) {
		t.Fatalf("arrival packets: %v", a)
	}
	if len(b) != 4 || b[0][0] != 4 || b[2][0] != 7 || !bytes.Equal(b[3], []byte{5, 4}) {
		t.Fatalf("snapshot packets: %v", b)
	}
	if wires[2].Len() != 0 {
		t.Fatal("spawn leaked to another map")
	}
	if err := s.worldCommand(ctx, players[1], []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if wires[0].Len() != 0 || wires[1].Len() != 0 {
		t.Fatal("duplicate ACK replayed spawn")
	}
	move := protocol.Builder{6, 1, 2}.U16(1200).U16(1300)
	if err := s.worldCommand(ctx, players[0], move); err != nil {
		t.Fatal(err)
	}
	want := protocol.Builder{6, 1}.U32(players[0].character.ID).U8(2).U16(1200).U16(1300)
	for _, i := range []int{0, 1} {
		packets := wires[i].packets(t)
		if len(packets) != 1 || !bytes.Equal(packets[0], want) {
			t.Fatalf("movement for %d: %v", i, packets)
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("movement leaked to another map")
	}
	s.leaveWorld(players[0])
	packets := wires[1].packets(t)
	want = protocol.Builder{12}.U32(players[0].character.ID).U16(0).U16(0).U16(0).U16(0).U8(0)
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Fatalf("despawn: %v", packets)
	}
	s.leaveWorld(players[0])
	if wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("logout duplicated or leaked")
	}
	if err := s.worldCommand(ctx, players[0], []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if len(wires[1].packets(t)) != 4 {
		t.Fatal("returning player did not spawn exactly once")
	}
}

func TestConcurrentWorldEntryAndMovement(t *testing.T) {
	s, players, wires := worldFixture(t)
	players[2].character.Map = players[0].character.Map
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for _, player := range players {
		wg.Add(1)
		go func(c *Session) {
			defer wg.Done()
			if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
				errs <- err
				return
			}
			for n := 0; n < 10; n++ {
				if err := s.worldCommand(context.Background(), c, protocol.Builder{6, 1, 0}.U16(uint16(1100+n)).U16(1200)); err != nil {
					errs <- err
					return
				}
			}
		}(player)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	for i, wire := range wires {
		seen := map[uint32]bool{players[i].character.ID: true}
		for _, packet := range wire.packets(t) {
			if packet[0] == 4 {
				seen[protocol.NewReader(packet[1:]).U32()] = true
			}
			if packet[0] == 6 && !seen[protocol.NewReader(packet[2:]).U32()] {
				t.Fatal("movement arrived before appearance")
			}
		}
		if len(seen) != 3 {
			t.Fatal("concurrent entry lost a peer", seen)
		}
	}
}

type failedWorldConn struct {
	captureConn
	closed bool
}

func (c *failedWorldConn) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (c *failedWorldConn) Close() error              { c.closed = true; return nil }

func TestFailedWorldRecipientDoesNotDisconnectActor(t *testing.T) {
	s, players, _ := worldFixture(t)
	if err := s.acknowledgeWorld(players[0]); err != nil {
		t.Fatal(err)
	}
	failed := &failedWorldConn{}
	players[0].conn = failed
	if err := s.acknowledgeWorld(players[1]); err != nil {
		t.Fatal(err)
	}
	if !failed.closed || !players[1].ready {
		t.Fatal("failed recipient affected entrant")
	}
	failed = &failedWorldConn{}
	players[2].conn = failed
	if err := s.acknowledgeWorld(players[2]); err == nil {
		t.Fatal("failed entrant was published")
	}
	if _, ok := s.world[players[2].info.ID]; ok {
		t.Fatal("failed entrant retained in world")
	}
}

func TestPortalWarpBetweenMaps(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	s.Assets.Maps[10017] = assets.Map{ID: 10017, Warps: []assets.Warp{{ClickID: 1, MapID: 20000, X: 500, Y: 600}}}
	s.Assets.Maps[20000] = assets.Map{ID: 20000, Warps: []assets.Warp{{ClickID: 1, MapID: 10017, X: 1042, Y: 1075}}, NPCs: []assets.MapNPC{{ClickID: 1, Flags: 1, X: 10, Y: 20}}}
	s.World = world.New(s.Assets)
	for i := range players {
		if err := s.worldCommand(ctx, players[i], []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, w := range wires {
		w.Reset()
	}
	alice := players[0].character.ID
	if err := s.worldCommand(ctx, players[0], []byte{20, 8, 1, 0}); err != nil {
		t.Fatal(err)
	}
	a := wires[0].packets(t)
	want := [][]byte{{20, 7}, protocol.Builder{23, 32}.U32(alice), protocol.Builder{23, 112}.U32(alice), protocol.Builder{23, 132}.U32(alice), protocol.Builder{12}.U32(alice).U16(20000).U16(500).U16(600).U16(1).U8(0), protocol.Builder{7}.U32(alice).U16(20000).U16(500).U16(600)}
	for i := range want {
		if !bytes.Equal(a[i], want[i]) {
			t.Fatalf("warp packet %d: %v", i, a[i])
		}
	}
	// Map info releases the scene, then visibility sync and the quest journal follow.
	if !bytes.Equal(a[9][:2], []byte{22, 4}) || !bytes.Equal(a[14], []byte{20, 8}) || !bytes.Equal(a[15], []byte{22, 12, 1, 1, 0, 0}) || !bytes.Equal(a[len(a)-1][:2], []byte{24, 7}) {
		t.Fatalf("map info: %v", a[8:])
	}
	if b := wires[1].packets(t); len(b) != 1 || !bytes.Equal(b[0], want[4]) {
		t.Fatalf("old-map departure: %v", b)
	}
	if wires[2].Len() != 0 || players[0].ready {
		t.Fatal("warp published before map acknowledgment")
	}
	chars, err := s.Store.Characters(ctx, players[0].account.ID)
	if err != nil || chars[0].Map != 20000 || chars[0].X != 500 {
		t.Fatal("warp not persisted", chars, err)
	}
	if err := s.worldCommand(ctx, players[0], []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if c := wires[2].packets(t); len(c) != 3 || c[0][0] != 4 || !bytes.Equal(c[2][:2], []byte{10, 3}) {
		t.Fatalf("warp arrival must omit the login sprite refresh: %v", c)
	}
	if a = wires[0].packets(t); len(a) != 4 || a[0][0] != 4 || !bytes.Equal(a[3], []byte{5, 4}) {
		t.Fatalf("destination snapshot: %v", a)
	}
	// Immediate reuse is debounced and only releases the client.
	if err := s.worldCommand(ctx, players[0], []byte{20, 8, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if a = wires[0].packets(t); len(a) != 1 || !bytes.Equal(a[0], []byte{20, 8}) {
		t.Fatalf("cooldown: %v", a)
	}
	if err := s.worldCommand(ctx, players[0], protocol.Builder{6, 1, 0}.U16(510).U16(610)); err != nil {
		t.Fatal(err)
	}
	if wires[1].Len() != 0 || wires[2].Len() == 0 {
		t.Fatal("movement not routed to destination map")
	}
	if err := s.worldCommand(ctx, players[1], []byte{20, 8, 1}); err == nil {
		t.Fatal("truncated portal accepted")
	}
}

func TestNativeMovementTrailingData(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	for i, player := range players {
		if err := s.worldCommand(ctx, player, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
		wires[i].Reset()
	}
	for _, wire := range wires {
		wire.Reset()
	}
	// Native movement is 15 bytes. Deliberately nonzero opaque trailing data
	// proves that only the direction and coordinate prefix affects state.
	request := []byte{6, 1, 2, 0xb0, 0x04, 0x14, 0x05, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	if err := s.dispatch(ctx, players[0], request); err != nil {
		t.Fatal(err)
	}
	expected := []byte{6, 1, 0x11, 0x27, 0, 0, 2, 0xb0, 0x04, 0x14, 0x05}
	for _, i := range []int{0, 1} {
		got := wires[i].packets(t)
		if len(got) != 1 || !bytes.Equal(got[0], expected) {
			t.Fatalf("native movement for player %d: %x", i, got)
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("native movement leaked to another map")
	}
	s.autosaveCharacters(ctx)
	stored, err := s.Store.Characters(ctx, players[0].account.ID)
	if err != nil || len(stored) != 1 || stored[0].X != 1200 || stored[0].Y != 1300 || players[0].character.X != 1200 || players[0].character.Y != 1300 {
		t.Fatalf("native movement not persisted: %v, %v", stored, err)
	}
	for _, invalid := range [][]byte{request[:6], {6, 1, 8, 0, 0, 0, 0}} {
		if err := s.dispatch(ctx, players[0], invalid); !errors.Is(err, protocol.ErrMalformed) {
			t.Fatalf("invalid movement %x accepted: %v", invalid, err)
		}
	}
	if players[0].character.X != 1200 || players[0].character.Y != 1300 {
		t.Fatal("invalid movement changed coordinates")
	}
}
