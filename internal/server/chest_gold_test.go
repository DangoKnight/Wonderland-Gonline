package server

import (
	"context"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func TestChestGoldCommitsCooldownAndRejectsReplay(t *testing.T) {
	s, players, _ := tradeFixture(t)
	c := players[0]
	s.Assets.ChestPools = []assets.ChestPool{{MapID: 10017, RespawnSeconds: 60, Rewards: []assets.ChestReward{{Gold: 100, Weight: 1}}}}
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Condition: evCond(3, 1, 3, 0, 5, 0), Operations: []assets.Operation{evOp(1, 1, 1, 1, 32176, 1)}}}}
	m := s.Assets.Maps[c.character.Map]
	m.Events = []assets.Event{ev}
	s.Assets.Maps[c.character.Map] = m
	s.World = world.New(s.Assets)
	if err := s.startEvent(context.Background(), c, 1, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != 200 || c.character.ChestRespawns[world.ChestKey(10017, 1)].IsZero() {
		t.Fatal("gold/cooldown")
	}
	if err := s.startEvent(context.Background(), c, 1, &ev, 0, true); err != nil || c.character.Gold != 200 {
		t.Fatal("duplicate gold", err)
	}
	stored := tradeStored(t, s, players)
	if stored[0].Gold != 200 || stored[0].ChestRespawns[world.ChestKey(10017, 1)].IsZero() {
		t.Fatal("gold not durable")
	}
}

func TestSharedPropGoldLimitRollsBackClaim(t *testing.T) {
	s, players, _ := tradeFixture(t)
	c := players[0]
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(c *game.Character) error { c.Gold = game.MaxGold; return nil }); err != nil {
		t.Fatal(err)
	}
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	_, _, err := s.Store.ClaimMapPropReward(context.Background(), ref, c.character.Map, 9, game.Item{}, 0, 100, time.Now(), time.Minute, s.Assets.Items)
	if err != store.ErrPropRewardUnavailable {
		t.Fatal(err)
	}
	props, err := s.Store.ActiveMapProps(context.Background(), c.character.Map, time.Now())
	if err != nil || len(props) != 0 {
		t.Fatal("failed reward consumed node", props, err)
	}
}
