package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestWorldEntryRejectsInvalidCreationAllocation(t *testing.T) {
	for _, stats := range [][]byte{{0, 0, 0, 0, 0}, {1, 1, 1, 1, 0}, {5, 5, 5, 5, 5}, {255, 255, 255, 255, 255}} {
		t.Run(string(rune(stats[0]+65)), func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			account, err := db.Register(context.Background(), "tester", "password", "")
			if err != nil {
				t.Fatal(err)
			}
			s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			wire := &captureConn{}
			c := &Session{account: account, slot: 1, conn: wire, info: SessionInfo{ID: 1}}
			if _, err := s.reserveName(context.Background(), c, "BudgetHero"); err != nil {
				t.Fatal(err)
			}
			p := createPayload()
			copy(p[len(p)-5:], stats)
			if err := s.dispatch(context.Background(), c, p); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if len(got) != 1 || !bytes.Equal(got[0], []byte{0, 30}) {
				t.Fatal(got)
			}
			chars, err := db.Characters(context.Background(), account.ID)
			if err != nil || len(chars) != 0 || c.character != nil {
				t.Fatal("rejection persisted or entered world", err)
			}
			// Correcting the allocation on the same connection retains the reserved name.
			if err := s.dispatch(context.Background(), c, createPayload()); err != nil || c.character == nil {
				t.Fatal("valid retry failed", err)
			}
		})
	}
}

func TestWorldEntryStoryConstellations(t *testing.T) {
	c := &game.Character{Quests: map[uint32]game.Quest{}}
	if got := storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 0}) {
		t.Fatal(got)
	}
	for _, id := range []uint32{13087, 13173, 13203, 13151} {
		c.Quests[id] = game.Quest{State: game.InProgress, Step: 1}
	}
	if got := storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 4, 1, 6, 8, 20}) {
		t.Fatal(got)
	}
	c.Quests[13087] = game.Quest{State: game.Completed, Step: 1}
	c.Quests[13173] = game.Quest{State: game.InProgress, Step: 0}
	if got := storyConstellations(c); !bytes.Equal(got, []byte{15, 19, 2, 8, 20}) {
		t.Fatal(got)
	}
}

func TestWorldEntrySnapshotRestoresOptionalState(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0].character
	s.Assets.NPCs = map[uint16]assets.NPC{42: {BookIndex: 1}, 43: {BookIndex: 5500}, 44: {BookIndex: 5501}, 45: {BookIndex: 0}}
	c.DiscoveredMonsters = []uint16{43, 42, 42, 44, 45, 60000}
	c.Quests[13087] = game.Quest{State: game.InProgress, Step: 1}
	packets, err := s.worldEntryPackets(*c, world.NewView(), newPetRoster())
	if err != nil {
		t.Fatal(err)
	}
	var discoveries [][]byte
	star, ready := -1, -1
	for i, p := range packets {
		if p[0] == 53 {
			discoveries = append(discoveries, p)
		}
		if bytes.Equal(p, []byte{15, 19, 1, 1}) {
			star = i
		}
		if bytes.Equal(p, []byte{1, 11}) {
			ready = i
		}
	}
	if len(discoveries) != 2 || !bytes.Equal(discoveries[0], []byte{53, 9, 42, 0, 0, 0}) || !bytes.Equal(discoveries[1], []byte{53, 9, 43, 0, 0, 0}) {
		t.Fatal(discoveries)
	}
	if star < 0 || ready < star {
		t.Fatal("story state absent or after ready marker")
	}
	if c.DiscoveredMonsters[0] != 43 {
		t.Fatal("snapshot mutated durable discovery order")
	}
	clone := c.Clone()
	clone.DiscoveredMonsters[0] = 99
	if c.DiscoveredMonsters[0] != 43 {
		t.Fatal("discovery state aliases clone")
	}
}

func TestWorldEntryMonsterDiscoveryVictoryPersistence(t *testing.T) {
	s, c, wire := battleFixture(t, 1)
	s.Assets.NPCs[10500] = assets.NPC{ID: 10500, BookIndex: 2}
	wire.Reset()
	if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, 2, 2}); err != nil {
		t.Fatal(err)
	}
	settle(t, s, c)
	if !contains(wire.packets(t), []byte{53, 9, 4, 41, 0, 0}) {
		t.Fatal("missing committed monster discovery")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(saved) != 1 || len(saved[0].DiscoveredMonsters) != 1 || saved[0].DiscoveredMonsters[0] != 10500 {
		t.Fatal("discovery not persisted", err)
	}
	run := &battleRun{b: &battle.Battle{Defenders: []*battle.Fighter{{Template: 10500}, {Template: 10500, Captured: true}, {Template: 70000}}}}
	if got := s.discoverBattleMonsters(c.character, run); len(got) != 0 {
		t.Fatal("repeat discovery", got)
	}
}

func TestWorldEntrySceneReadyGoldenAndWelcomeOnce(t *testing.T) {
	for _, first := range [][]byte{{89, 0}, {89, 0, 0x11, 0x27, 0, 0}, {89, 0, 0xff, 0xff, 0xff, 0xff}, {92, 1}, {92, 1, 0}, {92, 1, 1}} {
		s, players, wires := worldFixture(t)
		c := players[0]
		wire := wires[0]
		s.motd = "Welcome"
		// These sync requests are valid while loading, but cannot publish presence.
		if err := s.dispatch(context.Background(), c, first); err != nil {
			t.Fatal(err)
		}
		got := wire.packets(t)
		welcome := []byte{23, 57, 0, 7, 'W', 'e', 'l', 'c', 'o', 'm', 'e'}
		if !contains(got, welcome) {
			t.Fatal(got)
		}
		if first[0] == 89 && !contains(got, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
			t.Fatal(got)
		}
		if c.ready || len(s.world) != 0 || c.character.ID != 10001 || c.account.ID != 1 {
			t.Fatal("scene sync published map readiness")
		}
		for _, p := range [][]byte{{89, 0}, {89, 0, 0x11, 0x27, 0, 0}, {92, 1}, {92, 1, 0}, {92, 1, 1}} {
			if err := s.dispatch(context.Background(), c, p); err != nil {
				t.Fatal(err)
			}
		}
		if contains(wire.packets(t), welcome) {
			t.Fatal("welcome repeated")
		}
		char := c.character.Clone()
		view, pets := world.NewView(), newPetRoster()
		packets, err := s.worldEntryPackets(char, view, pets)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.enterWorld(c, char, view, pets, packets); err != nil {
			t.Fatal(err)
		}
		wire.Reset()
		if err := s.dispatch(context.Background(), c, []byte{92, 1}); err != nil {
			t.Fatal(err)
		}
		if !contains(wire.packets(t), welcome) {
			t.Fatal("new character login did not reset welcome")
		}
		for _, p := range [][]byte{{89}, {92}, {89, 0, 1}, {89, 0, 1, 2}, {89, 0, 1, 2, 3}, {89, 0, 1, 2, 3, 4, 5}, {92, 1, 2}, {92, 1, 0xff}, {92, 1, 0, 0}, {92, 1, 1, 2, 3, 4}} {
			if err := s.dispatch(context.Background(), c, p); !errors.Is(err, protocol.ErrMalformed) {
				t.Fatal(p, err)
			}
		}
		for _, p := range [][]byte{{89, 1}, {92, 0}} {
			if err := s.dispatch(context.Background(), c, p); !errors.Is(err, ErrUnsupported) {
				t.Fatal(p, err)
			}
		}
	}
}

func TestWorldEntryWelcomeByteLimit(t *testing.T) {
	s, _, _ := worldFixture(t)
	settings := s.RuntimeSettings()
	settings.MOTD = strings.Repeat("x", 255)
	if err := s.UpdateRuntimeSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	settings.MOTD = strings.Repeat("x", 256)
	if err := s.UpdateRuntimeSettings(context.Background(), settings); err == nil {
		t.Fatal("accepted unencodable welcome")
	}
	if len(s.RuntimeSettings().MOTD) != 255 {
		t.Fatal("invalid settings published")
	}
}

func TestWorldEntryKeepsAuthenticatedSocketOnBothServices(t *testing.T) {
	for _, service := range []string{"login", "world"} {
		t.Run(service, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			_, err = db.Register(context.Background(), "tester", "password", "")
			if err != nil {
				t.Fatal(err)
			}
			s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); s.accept(ctx, service, listener) }()
			client, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
			if err != nil {
				cancel()
				listener.Close()
				<-done
				t.Fatal(err)
			}
			defer func() { cancel(); client.Close(); listener.Close(); <-done; s.wg.Wait() }()
			client.SetDeadline(time.Now().Add(10 * time.Second))
			send := func(p []byte) {
				t.Helper()
				if err := protocol.Write(client, p); err != nil {
					t.Fatal(err)
				}
			}
			read := func() []byte {
				t.Helper()
				p, err := protocol.Read(client)
				if err != nil {
					t.Fatal(err)
				}
				return p
			}
			send(loginPayload("tester"))
			for i := 0; i < 3; i++ {
				read()
			}
			send([]byte{63, 2, 1})
			if p := read(); !bytes.Equal(p, []byte{1, 3, 0}) {
				t.Fatal(p)
			}
			send([]byte{9, 2, 'S', 'o', 'c', 'k', 'e', 't', 'H', 'e', 'r', 'o'})
			if p := read(); !bytes.Equal(p, []byte{9, 3, 0}) {
				t.Fatal(p)
			}
			send(createPayload())
			for count := 0; ; count++ {
				if count > 100 {
					t.Fatal("entry snapshot did not finish")
				}
				p := read()
				if p[0] == 35 && p[1] == 12 {
					break
				}
			}
			// Native aLogin sends this six-byte sync while the map is loading.
			// Its acknowledgment must arrive without closing the authenticated stream.
			send([]byte{89, 0, 0x11, 0x27, 0, 0})
			if p := read(); !bytes.Equal(p, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
				t.Fatal(p)
			}
			send([]byte{92, 1, 0})
			send([]byte{12, 1})
			for {
				if bytes.Equal(read(), []byte{5, 4}) {
					break
				}
			}
			// A world action on the original stream proves that no reconnect/ticket is required.
			send([]byte{89, 0})
			if p := read(); !bytes.Equal(p, []byte{90, 1, 0, 1, 1, 3, 2, 3}) {
				t.Fatal(p)
			}
		})
	}
}

func TestWorldEntryFailedDiscoverySavePublishesNothing(t *testing.T) {
	s, c, wire := battleFixture(t, 1000)
	s.Assets.NPCs[10500] = assets.NPC{ID: 10500, BookIndex: 1}
	wire.Reset()
	s.Store.Close()
	s.endBattle(c.battle, battle.Victory)
	if len(c.character.DiscoveredMonsters) != 0 || wire.Len() != 0 {
		t.Fatal("failed save published/adopted discovery")
	}
}

func TestWorldEntryStoryRewardSynchronizesAfterCommit(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	wire := wires[0]
	s.Assets.Marks = map[uint16]uint16{13087: 1}
	op := world.Op{Code: world.ActionQuestMark, D1: 13087, D2: 1}
	// Value packs its low byte in the high byte of D4.
	op.D4 = 1 << 8
	ok, err := s.questMark(context.Background(), c, op)
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
	if !contains(wire.packets(t), []byte{15, 19, 1, 1}) {
		t.Fatal("story reward not synchronized")
	}
	saved, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || saved[0].Quests[13087].Step != 1 {
		t.Fatal("story mark not committed", err)
	}
}
