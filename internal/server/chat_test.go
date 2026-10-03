package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

type closeConn struct {
	captureConn
	mu     sync.Mutex
	closed bool
}

func (c *closeConn) Close() error { c.mu.Lock(); c.closed = true; c.mu.Unlock(); return nil }

// chatFixture publishes Alice and Bobby on 10017 and Carol on 20000, all registered as
// online sessions.
func chatFixture(t *testing.T) (*Server, []*Session, []*captureConn) {
	s, players, wires := worldFixture(t)
	s.Assets.Maps[20000] = assets.Map{ID: 20000}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.info.CharacterID = c.character.ID
		s.sessions[c.info.ID] = c
		s.accounts[c.account.ID] = c.info.ID
		if err := s.worldCommand(context.Background(), c, []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
		wires[i].Reset()
	}
	for _, w := range wires {
		w.Reset()
	}
	return s, players, wires
}

func say(t *testing.T, s *Server, c *Session, text string) {
	t.Helper()
	if err := s.worldCommand(context.Background(), c, append([]byte{2, 2}, text...)); err != nil {
		t.Fatal(err)
	}
}

func TestChatBroadcastAndCommandGate(t *testing.T) {
	s, players, wires := chatFixture(t)
	say(t, s, players[0], "hello there")
	want := protocol.Builder{2, 2}.U32(players[0].character.ID).Bytes([]byte("hello there"))
	if p := wires[1].packets(t); len(p) != 1 || !bytes.Equal(p[0], want) {
		t.Fatal(p)
	}
	if wires[0].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("chat echoed to sender or leaked to another map")
	}
	say(t, s, players[0], ":gold 999")
	say(t, s, players[0], "/b takeover")
	for i, w := range wires {
		if w.Len() != 0 {
			t.Fatalf("non-GM command produced output for %d", i)
		}
	}
	if players[0].character.Gold != 0 {
		t.Fatal("non-GM changed gold")
	}
}

func TestGMCommands(t *testing.T) {
	s, players, wires := chatFixture(t)
	ctx := context.Background()
	gm, bobby, carol := players[0], players[1], players[2]
	s.SetGMLevel(gm.account.ID, 1)

	say(t, s, gm, ":gold 777")
	if p := wires[0].packets(t); len(p) != 1 || !bytes.Equal(p[0], protocol.Builder{26, 4}.U32(777)) {
		t.Fatal(p)
	}
	say(t, s, gm, ":item add 32176 3")
	if p := wires[0].packets(t); len(p) != 1 || p[0][1] != 5 || p[0][2] != 2 || p[0][5] != 3 {
		t.Fatal("item grant", p)
	}
	chars, err := s.Store.Characters(ctx, gm.account.ID)
	if err != nil || chars[0].Gold != 777 || chars[0].Bag[1].Count != 3 {
		t.Fatal("GM changes not persisted", err)
	}
	gm.character.HP = 1
	say(t, s, gm, ":heal")
	if p := wires[0].packets(t); len(p) != 17 || gm.character.HP != gm.character.MaxHP {
		t.Fatal("heal", len(p), gm.character.HP)
	}

	say(t, s, gm, ":b Server restart soon")
	notice := protocol.Builder{2, 4}.U32(0).Bytes([]byte("Server restart soon"))
	for i, w := range wires {
		if p := w.packets(t); len(p) != 1 || !bytes.Equal(p[0], notice) {
			t.Fatalf("notice for %d: %v", i, p)
		}
	}

	// Summon moves Carol to the GM; Bobby keeps seeing nothing from Carol's old map.
	say(t, s, gm, ":summon car")
	p := wires[2].packets(t)
	if len(p) < 6 || !bytes.Equal(p[0], []byte{20, 7}) || !bytes.Equal(p[4], protocol.Builder{12}.U32(carol.character.ID).U16(10017).U16(1042).U16(1075).U16(0).U8(0)) || carol.ready {
		t.Fatal("summon", p)
	}
	if err := s.worldCommand(ctx, carol, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if b := wires[1].packets(t); len(b) != 3 || b[0][0] != 4 {
		t.Fatal("summoned player not shown to the GM's map", b)
	}
	wires[0].Reset()
	wires[2].Reset()

	say(t, s, gm, ":warp 20000 100 200")
	if gm.character.Map != 20000 || gm.character.X != 100 || gm.ready {
		t.Fatal("warp", gm.character)
	}
	say(t, s, gm, ":warp 31999")
	if gm.character.Map != 20000 {
		t.Fatal("warped to a missing map")
	}

	conn := &closeConn{}
	bobby.conn = conn
	if err := s.worldCommand(ctx, gm, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	say(t, s, gm, ":kick Bobby")
	if !conn.closed {
		t.Fatal("kick did not disconnect")
	}

	s.SetGMLevel(gm.account.ID, 0)
	say(t, s, gm, ":gold 1")
	if gm.character.Gold != 777 {
		t.Fatal("revoked GM still effective")
	}
}

func TestSummonRacesTargetActions(t *testing.T) {
	s, players, _ := chatFixture(t)
	ctx := context.Background()
	gm, target := players[0], players[1]
	s.SetGMLevel(gm.account.ID, 1)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			say(t, s, gm, ":summon Bobby")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			// Stale movement after a summon is ignored, never fatal.
			if err := s.worldCommand(ctx, target, protocol.Builder{6, 1, 0}.U16(uint16(1000+i)).U16(1000)); err != nil {
				t.Error(err)
				return
			}
			if err := s.worldCommand(ctx, target, []byte{12, 1}); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	wg.Wait()
	if _, err := s.Store.Characters(ctx, target.account.ID); err != nil {
		t.Fatal(err)
	}
}

func TestEmotes(t *testing.T) {
	s, players, wires := chatFixture(t)
	ctx := context.Background()
	alice := players[0].character.ID
	for _, p := range [][]byte{{32, 1, 9}, {32, 1, 9}, {32, 2, 4}, {32, 3}, {32, 3}} {
		if err := s.worldCommand(ctx, players[0], p); err != nil {
			t.Fatal(err)
		}
	}
	got := wires[1].packets(t)
	want := [][]byte{protocol.Builder{32, 1}.U32(alice).U8(9), protocol.Builder{32, 2}.U32(alice).U8(4), protocol.Builder{32, 2}.U32(alice).U8(0)}
	if len(got) != len(want) {
		t.Fatal(got)
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatal(i, got[i])
		}
	}
	if wires[0].Len() != 0 {
		t.Fatal("pose echoed to sender")
	}
	// A held pose is replayed to a character arriving later.
	if err := s.worldCommand(ctx, players[0], []byte{32, 1, 7}); err != nil {
		t.Fatal(err)
	}
	s.leaveWorld(players[1])
	wires[1].Reset()
	if err := s.worldCommand(ctx, players[1], []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	got = wires[1].packets(t)
	if len(got) != 5 || !bytes.Equal(got[3], protocol.Builder{32, 2}.U32(alice).U8(7)) {
		t.Fatal("pose not replayed", got)
	}
}
