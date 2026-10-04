package server

import (
	"context"
	"reflect"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func xaolanRewardFixture(t *testing.T) (*Server, *Session, *captureConn, *eventSession) {
	s, c, wire := eventFixture(t)
	s.Assets.Items[43002] = game.ItemDefinition{ID: 43002, Type: 10, Name: "Shale"}
	s.Assets.Items[30025] = game.ItemDefinition{ID: 30025, Type: 10, Name: "Star"}
	s.Assets.Marks[13032], s.Assets.Marks[13033] = 14, 0
	ev := &assets.Event{ClickID: 5, Branches: []assets.Branch{{Index: 6, Condition: evCond(4, 1, 1, 0, 0, 0), Operations: []assets.Operation{
		evOp(13, 1, 1, 1, 43002, 2), evOp(14, 5, 13032, 2, 1, 0), evOp(15, 5, 13033, 1, 1, 1),
	}}}}
	s.Assets.Maps[12002] = assets.Map{ID: 12002, Events: []assets.Event{*ev}}
	s.World = world.New(s.Assets)
	next := c.character.Clone()
	next.Map = 12002
	next.Bag = game.Inventory{}
	next.Quests[13032] = game.Quest{ID: 13032, State: game.InProgress, Step: 1}
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	return s, c, wire, &eventSession{mapID: 12002, click: 2, ev: ev, branch: 0, index: 1}
}
func TestXaolanRescueRewardAndMarksCommitTogether(t *testing.T) {
	s, c, wire, es := xaolanRewardFixture(t)
	op := world.DecodeOp(es.ev.Branches[0].Operations[0])
	ok, err := s.questItem(context.Background(), c, es, op)
	if !ok || err != nil || es.index != 3 || bagCount(c.character.Bag, 43002) != 2 || bagCount(c.character.Bag, 30025) != 1 || c.character.Quests[13032].State != game.Completed || c.character.Quests[13033].Step != 1 {
		t.Fatal("reward checkpoint", ok, err, es.index)
	}
	if !contains(wire.packets(t), []byte{20, 10}) {
		t.Fatal("fanfare")
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || !reflect.DeepEqual(chars[0].Quests, c.character.Quests) || chars[0].Bag != c.character.Bag {
		t.Fatal("durability", err)
	}
	c.character = &chars[0]
	es.index = 1
	wire.Reset()
	if ok, err := s.questItem(context.Background(), c, es, op); !ok || err != nil || es.index != 3 || wire.Len() != 0 || c.character.Quests[13033].Step != 1 || bagCount(c.character.Bag, 30025) != 1 {
		t.Fatal("replayed reward", ok, err)
	}
}
func TestXaolanRescueRewardRejectsPartialDelivery(t *testing.T) {
	for _, scenario := range []string{"full", "missing_star", "cancelled_save", "unsupported_tail"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, wire, es := xaolanRewardFixture(t)
			ctx := context.Background()
			switch scenario {
			case "full":
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
				c.character.Bag[0] = game.Item{ID: 43002, Count: 48}
			case "missing_star":
				delete(s.Assets.Items, 30025)
			case "cancelled_save":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "unsupported_tail":
				es.ev.Branches[0].Operations[2] = evOp(15, 5, 13034, 1, 1, 1)
			}
			before := c.character.Clone()
			if scenario == "full" || scenario == "missing_star" {
				es.index = 0
				if s.canDeliver(c, es) {
					t.Fatal("incomplete reward preflight")
				}
				es.index = 1
			}
			ok, err := s.questItem(ctx, c, es, world.DecodeOp(es.ev.Branches[0].Operations[0]))
			if ok || !reflect.DeepEqual(before, *c.character) || es.index != 1 {
				t.Fatal("partial mutation", ok, err)
			}
			if contains(wire.packets(t), []byte{20, 10}) {
				t.Fatal("premature success")
			}
		})
	}
}
func TestXaolanRescueReceiptFailureDoesNotDuplicate(t *testing.T) {
	s, c, _, es := xaolanRewardFixture(t)
	c.conn = &gatheringFailedReceipt{}
	op := world.DecodeOp(es.ev.Branches[0].Operations[0])
	if ok, err := s.questItem(context.Background(), c, es, op); !ok || err == nil {
		t.Fatal("expected failed receipt", ok, err)
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Quests[13032].State != game.Completed || chars[0].Quests[13033].Step != 1 || bagCount(chars[0].Bag, 30025) != 1 {
		t.Fatal("lost committed reward", err)
	}
	c.character = &chars[0]
	c.conn = &captureConn{}
	es.index = 1
	if ok, err := s.questItem(context.Background(), c, es, op); !ok || err != nil || bagCount(c.character.Bag, 43002) != 2 || c.character.Quests[13033].Step != 1 {
		t.Fatal("duplicate after receipt loss", ok, err)
	}
}
func TestXaolanRepairIsScopedToNativeRescue(t *testing.T) {
	_, _, _, es := xaolanRewardFixture(t)
	branch := es.ev.Branches[0]
	op := world.DecodeOp(branch.Operations[0])
	if !xaolanRobberyReward(12002, branch, op) {
		t.Fatal("source repair absent")
	}
	for _, mapID := range []uint16{12003, 11077, 10017} {
		if xaolanRobberyReward(mapID, branch, op) {
			t.Fatal("foreign reward repaired")
		}
	}
	branch.Condition = evCond(4, 2, 1, 0, 0, 0)
	if xaolanRobberyReward(12002, branch, op) {
		t.Fatal("foreign formation repaired")
	}
	branch = es.ev.Branches[0]
	branch.Operations = branch.Operations[:1]
	if xaolanRobberyReward(12002, branch, op) {
		t.Fatal("ordinary shale reward repaired")
	}
}
func TestNativeXaolanRobberyRewardBranches(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	native, ok := catalog.Maps[12002]
	if !ok {
		t.Fatal("native home absent")
	}
	count := 0
	for _, ev := range native.Events {
		for i, branch := range ev.Branches {
			for _, raw := range branch.Operations {
				if !xaolanRobberyReward(12002, branch, world.DecodeOp(raw)) {
					continue
				}
				count++
				s, c, _, _ := xaolanRewardFixture(t)
				s.Assets, s.World = catalog, world.New(catalog)
				if err := s.startEvent(context.Background(), c, 2, &ev, i, true); err != nil {
					t.Fatal(err)
				}
				for n := 0; c.event != nil && n < 40; n++ {
					if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
						t.Fatal(err)
					}
				}
				if c.event != nil || bagCount(c.character.Bag, 43002) != 2 || bagCount(c.character.Bag, 30025) != 1 || c.character.Quests[13032].State != game.Completed || c.character.Quests[13033].Step != 1 {
					t.Fatal("native rescue reward incomplete", ev.ClickID, branch.Index)
				}
			}
		}
	}
	if count != 2 {
		t.Fatal("native rescue reward census", count)
	}
}
