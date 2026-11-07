package server

import (
	"bytes"
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/world"
)

func TestImmediateQuestRewardKeepsConversationAndAwardsOnce(t *testing.T) {
	for _, scenario := range []string{"same_event", "linked_event", "intro_item", "other_actor", "full_bag", "save_failure", "intervening_battle", "unmet_item", "fallback_reward", "unchanged_mark"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			source := assets.Branch{Index: 1, Condition: evCond(5, 900, 1, 1, 5, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), evOp(2, 5, 900, 1, 0, 1)}}
			target := assets.Branch{Index: 2, Condition: evCond(5, 900, 1, 1, 5, 1), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10003, 0), evOp(2, 1, 1, 1, 32176, 1), evOp(3, 5, 900, 2, 0, 0)}}
			if scenario == "intro_item" {
				source.Operations = append([]assets.Operation{evOp(0, 1, 1, 1, 24001, 1)}, source.Operations...)
			}
			if scenario == "intervening_battle" {
				target.Operations = append([]assets.Operation{evOp(1, 4, 1, 0, 0, 0)}, target.Operations...)
			}
			if scenario == "fallback_reward" {
				target.Condition = evCond(0, 0, 0, 0, 0, 0)
			}
			if scenario == "unchanged_mark" {
				source.Operations[1] = evOp(2, 5, 900, 1, 0, 0)
				target.Condition = evCond(5, 900, 1, 1, 5, 0)
			}
			ev := assets.Event{ClickID: 80, Branches: []assets.Branch{source, target}}
			if scenario == "unmet_item" {
				ev.Branches[1].Condition[12] = 2
				ev.Branches = append(ev.Branches, assets.Branch{Index: 3, Condition: evCond(2, 1, 1, 32176, 4, 1)})
			}
			m := s.Assets.Maps[10017]
			m.Events = []assets.Event{ev}
			m.NPCs = []assets.MapNPC{{ClickID: 5, Flags: 1, X: uint32(c.character.X), Y: uint32(c.character.Y), Events: []byte{80}}}
			if scenario == "linked_event" || scenario == "other_actor" {
				m.Events[0].Branches = []assets.Branch{source}
				m.Events = append(m.Events, assets.Event{ClickID: 81, Branches: []assets.Branch{target}})
				m.NPCs[0].Events = []byte{80, 81}
				if scenario == "other_actor" {
					m.NPCs[0].Events = []byte{80}
					m.NPCs = append(m.NPCs, assets.MapNPC{ClickID: 6, Flags: 1, Events: []byte{81}})
				}
			}
			s.Assets.Maps[10017] = m
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Quests = map[uint32]game.Quest{}
			next.Bag = game.Inventory{}
			if scenario == "full_bag" {
				for i := range next.Bag {
					next.Bag[i] = game.Item{ID: 24001, Count: 1}
				}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			wire.Reset()
			if scenario == "save_failure" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err := s.npcClick(ctx, c, []byte{5, 0}); err != nil {
				t.Fatal(err)
			}
			if c.event == nil {
				t.Fatal("initial line did not hold interaction")
			}
			wire.Reset()
			if scenario == "save_failure" {
				if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err == nil {
					t.Fatal("failed save accepted")
				}
				if bagCount(c.character.Bag, 32176) != 0 {
					t.Fatal("failed save delivered")
				}
				return
			}
			if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			continuation := scenario == "same_event" || scenario == "linked_event" || scenario == "intro_item" || scenario == "full_bag" || scenario == "fallback_reward"
			if continuation {
				if c.event == nil {
					t.Fatal("interaction ended before reward dialogue")
				}
				for _, p := range wire.packets(t) {
					if bytes.Equal(p, []byte{20, 8}) || bytes.Equal(p, []byte{6, 2, 0}) {
						t.Fatal("released movement between lines", p)
					}
				}
				if bagCount(c.character.Bag, 32176) != 0 {
					t.Fatal("reward preceded its dialogue acknowledgment")
				}
				if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
				want := 1
				if scenario == "full_bag" {
					want = 0
				}
				if c.event != nil || bagCount(c.character.Bag, 32176) != want {
					t.Fatal("reward/completion", c.event, c.character.Bag)
				}
				if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
				if bagCount(c.character.Bag, 32176) != want {
					t.Fatal("duplicate acknowledgment repeated reward")
				}
			} else if c.event != nil || bagCount(c.character.Bag, 32176) != 0 {
				t.Fatal("continued an ineligible/non-dialogue branch", scenario, c.event)
			}
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || chars[0].Bag != c.character.Bag {
				t.Fatal("SQL state mismatch", err)
			}
		})
	}
}

func TestImmediateQuestRewardSurvivesChoiceCallbacks(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	ev := assets.Event{ClickID: 80, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(5, 900, 1, 1, 5, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10001, 0), evOp(2, 5, 900, 1, 0, 1), evOp(3, 2, 5, 6, 77, 0)}},
		{Index: 2, Condition: evCond(7, 77, 30, 0, 0, 0), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10002, 0)}},
		{Index: 3, Condition: evCond(5, 900, 1, 1, 5, 1), Operations: []assets.Operation{evOp(1, 2, 5, 0, 10003, 0), evOp(2, 1, 1, 1, 32176, 1), evOp(3, 5, 900, 2, 0, 0)}},
	}}
	m := s.Assets.Maps[10017]
	m.Events = []assets.Event{ev}
	m.NPCs = []assets.MapNPC{{ClickID: 5, Flags: 1, X: uint32(c.character.X), Y: uint32(c.character.Y), Events: []byte{80}}}
	s.Assets.Maps[10017] = m
	s.World = world.New(s.Assets)
	next := c.character.Clone()
	next.Quests = map[uint32]game.Quest{}
	next.Bag = game.Inventory{}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.npcClick(ctx, c, []byte{5, 0}); err != nil {
		t.Fatal(err)
	}
	for _, p := range [][]byte{{20, 6}, {20, 9, 30}, {20, 6}} {
		if err := s.eventCommand(ctx, c, p); err != nil {
			t.Fatal(err)
		}
	}
	wire.Reset()
	if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event == nil || c.event.ev.Branches[c.event.branch].Index != 3 {
		t.Fatal("choice callback lost conversation checkpoint")
	}
	for _, p := range wire.packets(t) {
		if bytes.Equal(p, []byte{20, 8}) || bytes.Equal(p, []byte{6, 2, 0}) {
			t.Fatal("choice followup released conversation")
		}
	}
	if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || bagCount(c.character.Bag, 32176) != 1 {
		t.Fatal("missing final reward")
	}
}
