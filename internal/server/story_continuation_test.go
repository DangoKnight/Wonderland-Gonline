package server

import (
	"context"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

func TestStoryDinnerCheckpointAndClickRedirect(t *testing.T) {
	s, c, wire := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Map = 12000
	next.Quests[12018] = game.Quest{ID: 12018, State: game.InProgress, Step: 1}
	next.Quests[12043] = game.Quest{ID: 12043, State: game.InProgress, Step: 1}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Marks[12018] = 0
	s.Assets.Maps[12000] = assets.Map{ID: 12000, NPCs: []assets.MapNPC{{ClickID: 1, Template: 14140, X: 1050, Y: 1080}}}
	s.World = world.New(s.Assets)
	if handled, err := s.storyClick(ctx, c, 1); handled || err != nil || c.character.Quests[12018].Step != 2 || !c.view.Marks[12018] {
		t.Fatal("dinner repair", handled, err)
	}
	if !contains(wire.packets(t), protocol.Builder{24, 1}.U16(12018).U8(2)) {
		t.Fatal("missing journal refresh")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].Quests[12018].Step != 2 {
		t.Fatal("checkpoint not saved", err)
	}
	// Clive resumes the offer event instead of the ordinary click branch.
	next = c.character.Clone()
	next.Map = 11040
	next.Quests[13102] = game.Quest{ID: 13102, State: game.InProgress, Step: 4}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	ev := assets.Event{ClickID: 2, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 2, 4, 0, 10001, 0)}}}}
	s.Assets.Maps[11040] = assets.Map{ID: 11040, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	if handled, err := s.storyClick(ctx, c, 4); !handled || err != nil || c.event == nil || c.event.ev.ClickID != 2 {
		t.Fatal("clive checkpoint", handled, err)
	}
}
func TestNissRestControllerPreservesCompanionUntilCheckpoint(t *testing.T) {
	s, c, _ := eventFixture(t)
	ctx := context.Background()
	next := c.character.Clone()
	next.Map = 12052
	next.Pets = []game.Pet{{ID: 14081, Slot: 1, Name: "Niss", Level: 1, Amity: 60, HP: 10, MaxHP: 100, SP: 5, MaxSP: 50}}
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	c.pets.register(14081)
	s.Assets.NPCs = map[uint16]assets.NPC{14081: {ID: 14081, Name: "Niss", Type: 4}}
	s.Assets.Marks[13052] = 0
	ev := assets.Event{ClickID: 6, Branches: []assets.Branch{
		{Index: 1, Condition: evCond(5, 13052, 2, 0, 0, 0), Operations: []assets.Operation{evOp(1, 3, 2, 14081, 0, 0), evOp(2, 5, 13052, 1, 0, 1)}},
		{Index: 6, Condition: evCond(5, 13052, 1, 0, 5, 1), Operations: []assets.Operation{evOp(1, 3, 2, 14081, 0, 0), evOp(2, 2, 1, 0, 10001, 0), evOp(3, 5, 13052, 1, 0, 1)}},
		{Index: 12, Condition: evCond(5, 13052, 1, 0, 5, 2), Operations: []assets.Operation{evOp(1, 10, 1, 0, 0, 0), evOp(2, 5, 13052, 1, 0, 1)}},
	}}
	s.Assets.Maps[12052] = assets.Map{ID: 12052, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	if err := s.startEvent(ctx, c, 1, &ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if c.event == nil || c.event.ev.Branches[c.event.branch].Index != 6 || len(c.character.Pets) != 1 || c.character.Quests[13052].Step != 1 {
		t.Fatal("premature dismissal")
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || len(c.character.Pets) != 0 || len(c.character.ReservePets) != 1 || c.character.Quests[13052].Step != 3 {
		t.Fatal("rest controller state", c.event, c.character)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].ReservePets) != 1 || chars[0].Quests[13052].Step != 3 {
		t.Fatal("rest recovery", err)
	}
}
