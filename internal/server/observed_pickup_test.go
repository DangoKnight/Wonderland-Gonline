package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestObservedGroundPickupRecipients(t *testing.T) {
	for _, mode := range []string{"gonline", "legacy", "hidden", "other_map", "personal"} {
		t.Run(mode, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			a, b := players[0], players[1]
			a.character.X, a.character.Y = 1000, 2000
			if err := s.commit(context.Background(), a, a.character.Clone()); err != nil {
				t.Fatal(err)
			}
			mapID := a.character.Map
			s.Assets.Items[41066] = game.ItemDefinition{ID: 41066, Type: 10}
			s.Assets.Maps[mapID] = assets.Map{ID: mapID, Items: []assets.GroundItem{{ClickID: 7, ItemID: 41066, X: 1000, Y: 2000}}}
			b.connectionNegotiated = mode != "legacy"
			b.connectionFeatures = protocol.ConnectionObservedPickups
			a.invisible = mode == "hidden"
			if mode == "other_map" {
				b.character.Map++
			}
			if mode == "personal" {
				s.Assets.Story.Rules = append(s.Assets.Story.Rules, assets.StoryRule{ID: "personal", Kind: assets.StoryAccountGroundOnce, Scope: assets.StoryScope{Maps: []uint16{mapID}}, Value: int64(mapID)})
			}
			s.World = world.New(s.Assets)
			s.world[a.info.ID], s.world[b.info.ID] = a, b
			if err := s.pickup(context.Background(), a, 7); err != nil {
				t.Fatal(err)
			}
			packets := wires[1].packets(t)
			if mode == "other_map" || mode == "personal" {
				if len(packets) != 0 {
					t.Fatal("pickup leaked to another scene/account", packets)
				}
				return
			}
			if mode == "gonline" {
				if len(packets) != 1 {
					t.Fatal(packets)
				}
				p, ok := protocol.ParseObservedGroundPickup(packets[0])
				if !ok || p.Slot != 7 || p.Collector != a.character.ID || p.X != 1000 || p.Y != 2000 {
					t.Fatal("wrong collector", p, ok)
				}
			} else if !contains(packets, []byte{23, 2, 7, 0}) {
				t.Fatal("missing silent native removal", packets)
			}
		})
	}
}
