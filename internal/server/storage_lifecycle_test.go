package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestStorageServiceWaitsForNativeWindowClose(t *testing.T) {
	for _, service := range []uint16{world.ServicePropsKeeper, world.ServiceStockKeeper} {
		for _, authored := range []bool{false, true} {
			s, players, wires := worldFixture(t)
			c, wire := players[0], wires[0]
			ctx := context.Background()
			if authored {
				ev := &assets.Event{ClickID: 65000, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionServiceOrTeleport, service, 0, 0, 0)}}}}
				if err := s.startEvent(ctx, c, 38, ev, 0, false); err != nil {
					t.Fatal(err)
				}
			} else if err := s.openService(c, service); err != nil {
				t.Fatal(err)
			}
			opened := wire.packets(t)
			if c.event == nil || c.event.serviceOwner != c.character.ID || c.storageMap != c.character.Map {
				t.Fatal("storage lost its close acknowledgement owner")
			}
			if !contains(opened, []byte{29, 6}) || !contains(opened, []byte{20, 9}) {
				t.Fatal("native storage opening sequence missing", opened)
			}
			if contains(opened, []byte{20, 8}) {
				t.Fatal("storage resumed before its native close acknowledgement", opened)
			}
			for _, p := range opened {
				if len(p) > 1 && p[0] == 9 && p[1] == 12 {
					t.Fatal("storage sent unrelated spawn-point status")
				}
			}
			wire.Reset()
			// Captured Legacy close: FUN_001ded68 sets +0x7108; the event
			// loop sends AC20:6 and waits for resume before further NPC/warps.
			if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			closed := wire.packets(t)
			resumes := 0
			for _, p := range closed {
				if bytes.Equal(p, []byte{20, 8}) {
					resumes++
				}
			}
			if resumes != 1 || c.event != nil || c.storageMap != 0 || !contains(closed, []byte{6, 2, 0}) {
				t.Fatal("native storage close did not release interaction exactly once", closed)
			}
			wire.Reset()
			if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if wire.Len() != 0 {
				t.Fatal("duplicate close restarted an acknowledgement loop")
			}
			for _, peer := range wires[1:] {
				if peer.Len() != 0 {
					t.Fatal("storage control leaked to another client")
				}
			}
		}
	}
}

func TestStorageCancellationAndTeleportClearCloseOwner(t *testing.T) {
	for _, mode := range []string{"cancel", "teleport"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			if err := s.openService(c, world.ServicePropsKeeper); err != nil {
				t.Fatal(err)
			}
			es := c.event
			if mode == "cancel" {
				if err := s.eventCommand(context.Background(), c, []byte{20, 9, 40}); err != nil {
					t.Fatal(err)
				}
			} else if err := s.teleport(context.Background(), c, world.Destination{Map: 20000, X: 500, Y: 500}, 0); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || es.serviceOwner != 0 || c.storageMap != 0 {
				t.Fatal("storage ownership survived leaving/cancelling")
			}
			wires[0].Reset()
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if wires[0].Len() != 0 {
				t.Fatal("late acknowledgement revived the old service")
			}
		})
	}
}
