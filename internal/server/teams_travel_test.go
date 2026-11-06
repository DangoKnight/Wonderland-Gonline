package server

import (
	"context"
	"testing"
	"time"

	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestPartyCommandWarpCarriesAllFollowers(t *testing.T) {
	s, players, wires := nativeParty(t)
	ctx := context.Background()
	if err := s.partyCommand(ctx, players[2], protocol.Builder{13, 1}.U32(players[0].character.ID)); err != nil {
		t.Fatal(err)
	}
	if err := s.partyCommand(ctx, players[0], protocol.Builder{13, 3, 1}.U32(players[2].character.ID)); err != nil {
		t.Fatal(err)
	}
	p := players[0].party
	// Scripted map exits use commandTeleport rather than the regular portal
	// path. All three members must move before split-map validation runs.
	if err := s.commandTeleport(ctx, players[0], world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
		t.Fatal(err)
	}
	for i, member := range players {
		if member.party != p || member.character.Map != 20000 || member.character.X != 500 || member.character.Y != 600 || member.ready {
			t.Fatalf("member %d did not start the shared map load", i)
		}
		saved, err := s.Store.Characters(ctx, member.account.ID)
		if err != nil || len(saved) == 0 || saved[0].Map != 20000 {
			t.Fatalf("member %d warp not durable: %v", i, err)
		}
	}
	// Asynchronous map acknowledgements may arrive follower-first.
	for _, i := range []int{2, 1, 0} {
		if err := s.worldCommand(ctx, players[i], []byte{12, 1}); err != nil {
			t.Fatal(err)
		}
	}
	for i, member := range players {
		if member.party != p || !member.ready {
			t.Fatalf("member %d lost membership while loading", i)
		}
	}
	if !hasPacket(wires[1].packets(t), protocol.Builder{13, 6}.U32(players[0].character.ID).U8(2).U32(players[1].character.ID).U32(players[2].character.ID)) {
		t.Fatal("follower did not receive restored formation")
	}
}

func TestPartySplitMapsDisband(t *testing.T) {
	for _, mode := range []string{"independent follower warp", "unavailable follower", "unexpected map change", "already saved warp"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := nativeParty(t)
			ctx := context.Background()
			switch mode {
			case "independent follower warp":
				if err := s.commandTeleport(ctx, players[1], world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
					t.Fatal(err)
				}
			case "unavailable follower":
				players[1].tentOwner = players[1].character.ID
				if err := s.commandTeleport(ctx, players[0], world.Destination{Map: 20000, X: 500, Y: 600}); err != nil {
					t.Fatal(err)
				}
			case "unexpected map change":
				players[1].character.Map = 20000
				s.simulateWorldTick(time.Now(), false)
			case "already saved warp":
				if err := s.teleportAfterSave(players[1], world.Destination{Map: 20000, X: 500, Y: 600}, 0); err != nil {
					t.Fatal(err)
				}
			}
			for i, member := range players[:2] {
				if member.party != nil || member.view.Team.Load() != 0 {
					t.Fatalf("member %d retained split-map party", i)
				}
				if countPrefix(wires[i].packets(t), 13, 4) == 0 {
					t.Fatalf("member %d did not receive disband", i)
				}
			}
		})
	}
}
