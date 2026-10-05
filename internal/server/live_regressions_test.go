package server

import (
	"context"
	"fmt"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/world"
)

func TestFishingNativeSlotAndPresentationRequests(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	if err := s.dispatch(ctx, c, []byte{32, 2, 0}); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 1, 1}); err != nil || c.fishing == nil {
		t.Fatal("native four-byte start", err)
	}
	due := c.fishing.NextAt
	if err := s.catchFishing(ctx, c, due); err != nil || c.character.Bag[1].ID != 44001 {
		t.Fatal("due catch", err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 54, 0, 0}); err != nil || c.fishing != nil {
		t.Fatal("compatibility padded stop", err)
	}
	// Idle stop retains legacy mall-balance behavior without rejecting padding.
	if err := s.dispatch(ctx, c, []byte{23, 54, 0, 0}); err != nil {
		t.Fatal("idle native stop", err)
	}
	before := c.character.Clone()
	for _, bad := range [][]byte{{23, 53, 0}, {23, 53, 51, 1}, {23, 53, 0, 0, 0}, {23, 54, 0}, {23, 54, 0, 1}} {
		if err := s.dispatch(ctx, c, bad); err == nil {
			t.Fatal("accepted malformed fishing request", bad)
		}
	}
	if c.fishing != nil || c.character.Bag != before.Bag {
		t.Fatal("malformed request mutated cast/inventory")
	}
}

func TestNativeCarnieScriptedExitUsesReturnDestination(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, savedReturn := range []bool{true, false} {
		t.Run(fmt.Sprintf("return_%t", savedReturn), func(t *testing.T) {
			s, players, _ := worldFixture(t)
			c := players[0]
			s.Assets, s.World = catalog, world.New(catalog)
			origin := world.Destination{Map: 11016, X: 1000, Y: 1000}
			expected := origin
			if !savedReturn {
				expected = world.Destination{Map: 11016, X: 1181, Y: 243}
			}
			next := c.character.Clone()
			next.Map, next.X, next.Y = origin.Map, origin.X, origin.Y
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			c.ready = true
			if err := s.visitCarnie(ctx, c); err != nil {
				t.Fatal(err)
			}
			if c.character.Map != 11094 {
				t.Fatal("Carnie visit failed")
			}
			if !savedReturn {
				c.carnieReturn = nil
			}
			if err := s.dispatch(ctx, c, []byte{12, 1}); err != nil {
				t.Fatal(err)
			}
			c.arrived = false
			area, ok := s.World.Entry(11094, 1)
			if !ok {
				t.Fatal("native exit area absent")
			}
			c.character.X = uint16((area.X1 - 1) * world.RegionCellPixels)
			c.character.Y = uint16((area.Y1 - 1) * world.RegionCellPixels)
			if err := s.dispatch(ctx, c, []byte{20, 8, 1, 0}); err != nil {
				t.Fatal(err)
			}
			assertTravelSaved(t, s, c, expected.Map, expected.X, expected.Y)
			if err := s.dispatch(ctx, c, []byte{12, 1}); err != nil {
				t.Fatal(err)
			}
			// Reproduce walking after the exit followed by the old fishing rejection.
			c.character.X++
			if err := s.autosaveSession(ctx, c); err != nil {
				t.Fatal("post-warp checkpoint conflict", err)
			}
			stored, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || stored[0].X != expected.X+1 {
				t.Fatal("post-warp movement not saved", err)
			}
		})
	}
}

func TestTeleportCheckpointPreservesPendingStateAndFailedSave(t *testing.T) {
	s, players, _ := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	addTravelMaps(s, 11094)
	setAutosaveBaseline(c)
	old := c.autosaveBaseline.Clone()
	c.character.X++
	c.character.HP-- // Pending recoverable state must not be declared durable by a narrow warp.
	failed, cancel := context.WithCancel(ctx)
	cancel()
	dst := world.Destination{Map: 11094, X: 1180, Y: 875}
	if err := s.teleport(failed, c, dst, 0); err == nil {
		t.Fatal("failed warp succeeded")
	}
	if c.autosaveBaseline.Map != old.Map || c.autosaveBaseline.HP != old.HP {
		t.Fatal("failed warp advanced checkpoint")
	}
	if err := s.teleport(ctx, c, dst, 0); err != nil {
		t.Fatal(err)
	}
	if c.autosaveBaseline.Map != dst.Map || c.autosaveBaseline.HP != old.HP || c.character.HP != old.HP-1 {
		t.Fatal("narrow warp incorrectly checkpointed pending vitals")
	}
	if err := s.autosaveSession(ctx, c); err != nil {
		t.Fatal("post-warp full checkpoint", err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].Map != dst.Map || stored[0].HP != old.HP-1 {
		t.Fatal("pending vitals not saved", err)
	}
}

func TestFishingTraceContainsNativeRequestBytes(t *testing.T) {
	attrs := packetTraceAttrs([]byte{23, 53, 1, 1}, false)
	found := false
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == "payload_hex" {
			found = attrs[i+1] == "17350101"
		}
	}
	if !found {
		t.Fatal("native fishing request bytes not traced", attrs)
	}
	attrs = packetTraceAttrs([]byte{protocol.CommandCharacterCreation, 1, 's', 'e', 'c', 'r', 'e', 't'}, false)
	for i := 0; i+1 < len(attrs); i += 2 {
		if attrs[i] == "payload_hex" {
			t.Fatal("credential request traced")
		}
	}
}

func TestFishingNativeUsesRequestedOwnedRodNotPresentation(t *testing.T) {
	s, c, _ := fishingFixture(t)
	ctx := context.Background()
	s.Assets.Fishing.Rods = append(s.Assets.Fishing.Rods, assets.FishingRod{ItemID: 37082, MaxGrade: 10})
	s.Assets.Items[37082] = game.ItemDefinition{ID: 37082, Type: 28}
	next := c.character.Clone()
	next.Bag[7] = game.Item{ID: 37082, Count: 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	// This exact user capture selects the weaker rod in slot 1, even though a
	// stronger rod exists. Presentation bytes never choose grade or grant items.
	if err := s.dispatch(ctx, c, []byte{23, 53, 1, 1}); err != nil || c.fishing == nil || c.fishing.Slot != 1 || c.fishing.Rod != 37105 {
		t.Fatal("native selected rod ignored", err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 54}); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 8, 255}); err != nil || c.fishing == nil || c.fishing.Rod != 37082 {
		t.Fatal("presentation value changed gameplay validation", err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 54}); err != nil {
		t.Fatal(err)
	}
	if err := s.dispatch(ctx, c, []byte{23, 53, 2, 1}); err != nil || c.fishing != nil {
		t.Fatal("empty slot bypassed rod ownership", err)
	}
	c.character.Bag[0].Locked = true
	if err := s.dispatch(ctx, c, []byte{23, 53, 1, 1}); err != nil || c.fishing != nil {
		t.Fatal("locked rod accepted", err)
	}
}
