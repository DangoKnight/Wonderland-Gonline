package server

import (
	"bytes"
	"context"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func monkeyFixture(t *testing.T) (*Server, *Session, *captureConn) {
	s, c, wire := eventFixture(t)
	s.Assets.NPCs = map[uint16]assets.NPC{17162: {ID: 17162, Name: "S.Monkey", Type: 4, Stats: [5]uint16{10, 10, 10, 10, 10}}}
	s.Assets.Marks[12002] = 0
	s.Assets.Marks[12003] = 0
	ev := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, 2, 1, 0, 20038, 0)}}}}
	s.Assets.Maps[10017] = assets.Map{ID: 10017, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17162, X: 1050, Y: 1080, Flags: 1, Events: []byte{1}}}, Events: []assets.Event{ev}}
	s.World = world.New(s.Assets)
	wire.Reset()
	return s, c, wire
}
func TestMonkeyNativeDialogueAtomicRecruitAndReplay(t *testing.T) {
	s, c, wire := monkeyFixture(t)
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{20, 1, 1, 0}); err != nil {
		t.Fatal(err)
	}
	first := []byte{20, 1, 0, 0, 0, 1, 1, 7, 0, 0, 1, 0, 0, 0, 0, 70, 78, 0}
	if got := wire.packets(t); len(got) != 2 || !bytes.Equal(got[1], first) {
		t.Fatal("first native dialogue", got)
	}
	portraits := []byte{7, 3, 7, 7, 3, 7, 7, 3, 7, 7, 3, 7, 3, 7, 7, 3}
	for step := 2; step <= 16; step++ {
		if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
		packets := wire.packets(t)
		speaker := byte(1)
		if portraits[step-1] == 7 {
			speaker = 0
		}
		if len(packets) != 1 || !bytes.Equal(packets[0], dialogueFrame(byte(step), portraits[step-1], speaker, uint32(20037+step))) || len(c.character.Pets) != 0 {
			t.Fatal("dialogue sequence/reward early", step, packets)
		}
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if len(c.character.Pets) != 1 || c.character.Pets[0].ID != 17162 || c.character.Quests[12002].State != game.Completed || c.character.Quests[12003].Step != 1 || c.event != nil {
		t.Fatal("rescue state")
	}
	if got := wire.packets(t); !contains(got, []byte{22, 10, 1, 0, 255, 255}) || !contains(got, []byte{20, 10}) {
		t.Fatal("success frames", got)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars[0].Pets) != 1 || chars[0].Quests[12002].CompletedAt == nil || chars[0].Quests[12003].Step != 1 {
		t.Fatal("rescue not persisted", err)
	}
	// Direct reference controller replay uses its short squeak and cannot mint a pet.
	if err := s.startMonkey(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	if got := wire.packets(t); len(got) != 2 || !bytes.Equal(got[1], dialogueFrame(1, 3, 1, 20042)) {
		t.Fatal("completed squeak", got)
	}
	if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil || len(c.character.Pets) != 1 {
		t.Fatal("duplicate rescue", err)
	}
}
func TestMonkeyFullPartyCancellationAndFailedSave(t *testing.T) {
	t.Run("full", func(t *testing.T) {
		s, c, wire := monkeyFixture(t)
		next := c.character.Clone()
		for i := 0; i < 4; i++ {
			next.Pets = append(next.Pets, game.Pet{ID: uint32(14080 + i), Slot: byte(i + 1), Name: "Companion", Level: 1, Amity: 60})
		}
		if err := s.commit(context.Background(), c, next); err != nil {
			t.Fatal(err)
		}
		if err := s.worldCommand(context.Background(), c, []byte{20, 1, 1, 0}); err != nil {
			t.Fatal(err)
		}
		wire.packets(t)
		for i := 0; i < 13; i++ {
			if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		if c.event != nil || len(c.character.Pets) != 4 || c.character.Quests[12002].State == game.Completed {
			t.Fatal("full party consumed rescue")
		}
		if !contains(wire.packets(t), dialogueFrame(12, 7, 0, 31146)) {
			t.Fatal("missing full-party line")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		s, c, _ := monkeyFixture(t)
		ctx := context.Background()
		if err := s.worldCommand(ctx, c, []byte{20, 1, 1, 0}); err != nil {
			t.Fatal(err)
		}
		if err := s.worldCommand(ctx, c, []byte{20, 9, 40}); err != nil {
			t.Fatal(err)
		}
		if c.event != nil || len(c.character.Pets) != 0 || c.character.Quests[12002].ID != 0 || c.character.Quests[12003].ID != 0 {
			t.Fatal("cancel granted rescue")
		}
		c.resumeAt = time.Time{}
		if err := s.worldCommand(ctx, c, []byte{20, 1, 1, 0}); err != nil || c.event == nil {
			t.Fatal("cannot retry", err)
		}
	})
	t.Run("save failure", func(t *testing.T) {
		s, c, wire := monkeyFixture(t)
		ctx := context.Background()
		if err := s.worldCommand(ctx, c, []byte{20, 1, 1, 0}); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 15; i++ {
			if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
		}
		wire.packets(t)
		if err := s.Store.Close(); err != nil {
			t.Fatal(err)
		}
		if err := s.worldCommand(ctx, c, []byte{20, 6}); err == nil {
			t.Fatal("failed save accepted")
		}
		if len(c.character.Pets) != 0 || c.character.Quests[12002].ID != 0 || c.character.Quests[12003].ID != 0 || wire.Len() != 0 || c.view.Hidden[1] {
			t.Fatal("success before persistence")
		}
	})
}
