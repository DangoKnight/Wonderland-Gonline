package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestStallListDuringMapLoadingAndGameplay(t *testing.T) {
	for _, ready := range []bool{false, true} {
		name := "loading"
		if ready {
			name = "ready"
		}
		t.Run(name, func(t *testing.T) {
			s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			wire := &captureConn{}
			c := &Session{conn: wire, character: &game.Character{ID: 10001, Map: 10017}, ready: ready}
			before := c.character.Clone()
			for range 2 {
				if err := s.dispatch(context.Background(), c, []byte{23, 77}); err != nil {
					t.Fatal(err)
				}
				got := wire.packets(t)
				if len(got) != 2 || !bytes.Equal(got[0], []byte{23, 4, 0}) || !bytes.Equal(got[1], []byte{23, 102}) {
					t.Fatalf("stall list replies %x", got)
				}
			}
			if err := s.dispatch(context.Background(), c, []byte{5, 7, 0}); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			if len(got) != 1 || !bytes.Equal(got[0], []byte{5, 8, 0x11, 0x27, 0, 0, 0}) {
				t.Fatalf("sprite refresh %x", got)
			}
			if err := s.dispatch(context.Background(), c, []byte{5, 4}); err != nil {
				t.Fatal(err)
			}
			if got := wire.packets(t); len(got) != 0 {
				t.Fatalf("state ping unexpectedly echoed %x", got)
			}
			if c.ready != ready || c.character.ID != before.ID || c.character.Map != before.Map || c.character.Bag != before.Bag {
				t.Fatal("read-only request changed world or inventory state")
			}
			if !ready {
				if err := s.dispatch(context.Background(), c, []byte{5, 17, 1}); !errors.Is(err, protocol.ErrMalformed) {
					t.Fatalf("teleport allowed before map acknowledgment: %v", err)
				}
				for _, request := range [][]byte{{23, 26, 1, 0}, {75, 1, 1, 1, 0, 4, 1, 1, 0}} {
					if err := s.dispatch(context.Background(), c, request); !errors.Is(err, protocol.ErrMalformed) {
						t.Fatalf("mall purchase allowed before map acknowledgment: %v", err)
					}
				}
				// Only read-only synchronization bypasses the map-loading guard.
				if err := s.dispatch(context.Background(), c, []byte{23, 10, 1, 1, 2}); !errors.Is(err, protocol.ErrMalformed) {
					t.Fatalf("inventory mutation allowed before map acknowledgment: %v", err)
				}
			}
		})
	}
	s := New(config.Default(), nil, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := s.dispatch(context.Background(), &Session{}, []byte{23, 77}); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatalf("list allowed without selected character: %v", err)
	}
}
