package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestSharedMapPropClaimsVisibilityRecoveryAndRespawn(t *testing.T) {
	s, players, wires := worldFixture(t)
	ctx := context.Background()
	mapID := players[0].character.Map
	s.Assets.Maps[mapID] = assets.Map{ID: mapID, NPCs: []assets.MapNPC{{ClickID: 9, Template: 12001, Name: "Clay Mine", Flags: 1, X: 1050, Y: 1080}}}
	s.Assets.ChestPools = []assets.ChestPool{{MapID: mapID, RespawnSeconds: 60, Rewards: []assets.ChestReward{{Item: 32176, Count: 2, Weight: 1}}}}
	s.World = world.New(s.Assets)
	for i, c := range players {
		c.ready = true
		s.world[c.info.ID] = c
		wires[i].Reset()
	}
	n, _ := s.World.NPC(mapID, 9)
	before := bagCount(players[0].character.Bag, 32176)
	otherBefore := bagCount(players[1].character.Bag, 32176)
	s.worldMu.Lock()
	handled, err := s.harvestProp(ctx, players[0], n)
	s.worldMu.Unlock()
	if err != nil || !handled || bagCount(players[0].character.Bag, 32176) != before+2 {
		t.Fatal("claim", handled, err)
	}
	want := protocol.Builder{22, 1}.U16(9).U8(1)
	for _, i := range []int{0, 1} {
		if !contains(wires[i].packets(t), want) {
			t.Fatal("missing broken frame", i)
		}
	}
	if wires[2].Len() != 0 {
		t.Fatal("foreign map broadcast")
	}
	if handled, err := s.harvestProp(ctx, players[1], n); err != nil || !handled || bagCount(players[1].character.Bag, 32176) != otherBefore {
		t.Fatal("duplicate shared claim", err)
	}
	wires[1].Reset()
	players[1].view = world.NewView()
	if err := s.syncMapProps(ctx, players[1], time.Now()); err != nil || !contains(wires[1].packets(t), want) {
		t.Fatal("reconnect frame", err)
	}
	players[1].view.Hidden[9] = true
	wires[1].Reset()
	players[0].tentOwner = players[0].character.ID
	wires[0].Reset()
	s.respawnMapProps(ctx, time.Now().Add(time.Minute))
	if wires[0].Len() != 0 || wires[1].Len() != 0 || wires[2].Len() != 0 {
		t.Fatal("hidden/tent/foreign reset leak")
	}
	props, err := s.Store.ActiveMapProps(ctx, mapID, time.Now())
	if err != nil || len(props) != 0 {
		t.Fatal("expiry", props, err)
	}
	// An event finish replays a still-broken node after authored prop frames.
	players[0].tentOwner = 0
	players[1].view.Hidden[9] = false
	_, _, err = s.Store.ClaimMapProp(ctx, store.CharacterRef{Account: players[1].account.ID, ID: players[1].character.ID}, mapID, 9, game.Item{ID: 32176, Count: 1}, 50, time.Now(), time.Minute, nil)
	if err != nil {
		t.Fatal(err)
	}
	wires[1].Reset()
	es := &eventSession{}
	players[1].event = es
	players[1].view.Props[9] = 0
	if err := s.finishEvent(players[1], es); err != nil {
		t.Fatal(err)
	}
	if got := wires[1].packets(t); len(got) == 0 || !bytes.Equal(got[len(got)-1], want) {
		t.Fatal("event finish lost shared broken frame", got)
	}
}

func bagCount(bag game.Inventory, id uint16) int {
	n := 0
	for _, item := range bag {
		if item.ID == id {
			n += int(item.Count)
		}
	}
	return n
}
