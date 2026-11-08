package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestAccountGroundPickupIsolationAndArrival(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	mapID := players[0].character.Map
	s.Assets.Story.PrivateScenes = []uint16{mapID}
	s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "personal_ground", Kind: assets.StoryAccountGroundOnce, Scope: assets.StoryScope{Maps: []uint16{mapID}}, Value: int64(mapID)})
	s.Assets.Items[41066] = game.ItemDefinition{ID: 41066, Type: 10}
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, Scene: mapID, Items: []assets.GroundItem{{ClickID: 2, ItemID: 41066, X: uint32(players[0].character.X), Y: uint32(players[0].character.Y)}}}
	s.World = world.New(s.Assets)
	for _, c := range players[:2] {
		next := c.character.Clone()
		next.Bag = game.Inventory{}
		next.X, next.Y = players[0].character.X, players[0].character.Y
		if err := s.commit(ctx, c, next); err != nil {
			t.Fatal(err)
		}
		s.world[c.info.ID] = c
	}
	// Old shared clocks must not suppress personal resources after an upgrade.
	s.World.RestoreGroundCooldowns(mapID, map[uint16]time.Time{2: time.Now().Add(time.Hour)})
	for i, c := range players[:2] {
		for _, wire := range wires {
			wire.Reset()
		}
		if err := s.pickup(ctx, c, 2); err != nil {
			t.Fatal(err)
		}
		if !contains(wires[i].packets(t), []byte{23, 2, 2, 0, 1}) {
			t.Fatal("missing native pickup confirmation")
		}
		if wires[1-i].Len() != 0 {
			t.Fatal("another account's resource was removed")
		}
		before := c.character.Bag
		if err := s.pickup(ctx, c, 2); err != nil || c.character.Bag != before {
			t.Fatal("replay granted twice", err)
		}
		packets, err := s.arrivalPackets(*c.character, c.view, c.pets, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range packets {
			if len(p) >= 2 && bytes.Equal(p[:2], []byte{23, 4}) {
				t.Fatal("claimed node returned on map entry", p)
			}
		}
	}
	if got := s.World.Respawn(time.Now().Add(365 * 24 * time.Hour)); len(got) != 0 {
		t.Fatal("personal nodes respawned", got)
	}
}

func TestSharedGroundRemovalIgnoresPlayerPrivacy(t *testing.T) {
	s, players, wires := worldFixture(t)
	a, b := players[0], players[1]
	mapID := a.character.Map
	s.Assets.Story.PrivateScenes = []uint16{mapID}
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, Scene: mapID, Items: []assets.GroundItem{{ClickID: 2, ItemID: 41066, X: uint32(a.character.X), Y: uint32(a.character.Y)}}}
	s.Assets.Items[41066] = game.ItemDefinition{ID: 41066, Type: 10}
	s.World = world.New(s.Assets)
	s.world[a.info.ID], s.world[b.info.ID] = a, b
	if err := s.pickup(context.Background(), a, 2); err != nil {
		t.Fatal(err)
	}
	if !contains(wires[1].packets(t), []byte{23, 2, 2, 0}) {
		t.Fatal("shared resource removal hidden by player privacy")
	}
}
