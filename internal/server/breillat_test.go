package server

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/world"
)

func finishBreillatDialogue(t *testing.T, s *Server, c *Session) {
	t.Helper()
	for i := 0; c.event != nil && c.event.onComplete != nil; i++ {
		if i > 20 {
			t.Fatal("dialogue did not finish")
		}
		if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
			t.Fatal(err)
		}
	}
}

func saveBreillatFixture(t *testing.T, s *Server, c *Session) {
	t.Helper()
	if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error { *stored = c.character.Clone(); return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestInstalledBreillatTenTalksAndPermanentConversion(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	for _, mapID := range []uint16{10002, 10021, 10022, 10023, 10024, 10025, 10026, 10027, 10028, 10029, 10030} {
		ev, ok := w.Event(mapID, 4)
		if !ok || !world.IsBreillat(ev) {
			t.Fatalf("native Breillat script not recognized on %d", mapID)
		}
		sourceBefore, _ := json.Marshal(ev)
		s, players, wires := worldFixture(t)
		c := players[0]
		s.Assets, s.World = catalog, w
		c.character.Map = mapID
		saveBreillatFixture(t, s, c)
		original := c.character.Clone()
		for count := 1; count <= 10; count++ {
			branch := w.FindBranch(c.character, c.view, mapID, ev, 0, 0, 0, -1)
			if err := s.startEvent(context.Background(), c, 5, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			finishBreillatDialogue(t, s, c)
			if q := c.character.Quests[50030]; q.Step != count {
				t.Fatalf("map %d talk %d counted %d", mapID, count, q.Step)
			}
			if count < 10 && c.event != nil {
				t.Fatalf("early offer at talk %d", count)
			}
		}
		if c.event == nil || c.event.onChoice == nil || c.event.ev.Branches[c.event.branch].Index != 7 {
			t.Fatal("tenth talk did not open offer", mapID)
		}
		// Declining leaves the count and appearance unchanged; the next click offers again.
		if err := s.eventCommand(context.Background(), c, []byte{20, 9, 31}); err != nil {
			t.Fatal(err)
		}
		finishBreillatDialogue(t, s, c)
		if c.event != nil || c.character.Body != original.Body || c.character.Quests[50030].Step != 10 {
			t.Fatal("decline changed character")
		}
		branch := w.FindBranch(c.character, c.view, mapID, ev, 0, 0, 0, -1)
		if ev.Branches[branch].Index != 7 {
			t.Fatal("greeting still shadows offer")
		}
		if err := s.startEvent(context.Background(), c, 5, ev, branch, true); err != nil {
			t.Fatal(err)
		}
		finishBreillatDialogue(t, s, c)
		wires[0].Reset()
		players[1].character.Map = mapID
		s.world[c.info.ID] = c
		s.world[players[1].info.ID] = players[1]
		s.world[players[2].info.ID] = players[2]
		if err := s.eventCommand(context.Background(), c, []byte{20, 9, 30}); err != nil {
			t.Fatal(err)
		}
		// Acceptance only becomes durable after its final dialogue acknowledgment.
		if c.character.Quests[50031].Step != 0 || c.character.Quests[50030].State != game.InProgress {
			t.Fatal("acceptance mutated marks before transformation")
		}
		finishBreillatDialogue(t, s, c)
		if c.event != nil || c.character.Body != 4 || c.character.Head != 3 || c.character.Quests[50031].Step != 1 || c.character.Quests[50030].State != game.Completed {
			t.Fatal("conversion did not finish", mapID, c.character)
		}
		chars, err := s.Store.Characters(context.Background(), c.account.ID)
		if err != nil || !reflect.DeepEqual(chars[0], *c.character) {
			t.Fatal("conversion not durable", err)
		}
		if c.character.Bag != original.Bag || c.character.Equipment != original.Equipment || c.character.Base != original.Base || c.character.Name != original.Name || c.character.EXP != original.EXP {
			t.Fatal("conversion destroyed player state")
		}
		found := false
		id := c.character.ID
		want := []byte{5, 12, byte(id), byte(id >> 8), byte(id >> 16), byte(id >> 24), 13}
		for _, p := range wires[0].packets(t) {
			if bytes.Equal(p, want) {
				found = true
			}
		}
		if !found {
			t.Fatal("missing native model packet", want)
		}
		peers := wires[1].packets(t)
		if len(peers) != 2 || peers[0][0] != 4 || peers[0][5] != 4 || peers[0][15] != 3 || peers[0][16] != 0 || !bytes.Equal(peers[1], want) {
			t.Fatal("peer appearance missing", peers)
		}
		if wires[2].Len() != 0 {
			t.Fatal("conversion leaked to another map")
		}
		if !c.view.Hidden[5] {
			t.Fatal("Breillat was not hidden after conversion")
		}
		sourceAfter, _ := json.Marshal(ev)
		if !bytes.Equal(sourceBefore, sourceAfter) {
			t.Fatal("shared SQL script mutated")
		}
		if w.FindBranch(c.character, c.view, mapID, ev, 0, 0, 0, -1) >= 0 {
			t.Fatal("converted character can repeat interaction")
		}
	}
}

func TestBreillatConversionRejectsStaleAndFailedClaims(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	ev, _ := w.Event(10002, 4)
	for _, scenario := range []string{"too few talks", "already converted", "moved", "canceled", "valid stale inventory", "zero conversion mark"} {
		t.Run(scenario, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			s.Assets, s.World = catalog, w
			c.character.Map = 10002
			c.character.Quests[50030] = game.Quest{ID: 50030, State: game.InProgress, Step: 10}
			saveBreillatFixture(t, s, c)
			ctx := context.Background()
			if scenario == "canceled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error {
				switch scenario {
				case "too few talks":
					q := stored.Quests[50030]
					q.Step = 9
					stored.Quests[50030] = q
				case "already converted":
					stored.Quests[50031] = game.Quest{ID: 50031, State: game.InProgress, Step: 1}
				case "moved":
					stored.Map = 10017
				case "zero conversion mark":
					stored.Quests[50031] = game.Quest{ID: 50031, State: game.InProgress, Step: 0}
				case "valid stale inventory":
					stored.Gold = 12345
					stored.Bag[3] = game.Item{ID: 30002, Count: 1}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			es := &eventSession{mapID: 10002, click: 5, ev: ev, branch: 8}
			c.event = es
			op := world.DecodeOp(ev.Branches[8].Operations[len(ev.Branches[8].Operations)-1])
			ok, err := s.convertBreillat(ctx, c, es, op)
			if scenario == "valid stale inventory" || scenario == "zero conversion mark" {
				if !ok || err != nil || c.character.Body != 4 || scenario == "valid stale inventory" && (c.character.Gold != 12345 || c.character.Bag[3].ID != 30002) {
					t.Fatal("fresh player state lost", ok, err)
				}
				ok, err = s.convertBreillat(ctx, c, es, op)
				if ok || err != nil {
					t.Fatal("conversion replay succeeded", ok, err)
				}
			} else {
				if ok || scenario == "canceled" && err == nil {
					t.Fatal("invalid claim succeeded", ok, err)
				}
				chars, e := s.Store.Characters(context.Background(), c.account.ID)
				if e != nil || chars[0].Body == 4 || chars[0].Quests[50030].State == game.Completed {
					t.Fatal("invalid claim mutated durable appearance", e)
				}
				for _, p := range wires[0].packets(t) {
					if len(p) > 1 && p[0] == 5 && p[1] == 12 {
						t.Fatal("failed claim published transformation")
					}
				}
			}
		})
	}
}

func TestBreillatTransformRequiresRecognizedAcceptance(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ev := &assets.Event{ClickID: 4, Branches: []assets.Branch{{Index: 9, Operations: []assets.Operation{evOp(1, 12, 1, 13, 0, 0)}}}}
	if err := s.startEvent(context.Background(), c, 5, ev, 0, true); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || c.character.Body == 4 {
		t.Fatal("unrecognized transform executed")
	}
	for _, p := range wires[0].packets(t) {
		if len(p) > 1 && p[0] == 5 && p[1] == 12 {
			t.Fatal("unrecognized transform published")
		}
	}
}

func TestBreillatFailedReceiptKeepsConversionDurable(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	s, players, _ := worldFixture(t)
	c := players[0]
	s.Assets, s.World = catalog, world.New(catalog)
	ev, _ := s.World.Event(10002, 4)
	c.character.Map = 10002
	c.character.Quests[50030] = game.Quest{ID: 50030, State: game.InProgress, Step: 255}
	saveBreillatFixture(t, s, c)
	if branch := s.World.FindBranch(c.character, c.view, 10002, ev, 0, 0, 0, -1); branch != 6 {
		t.Fatal("saturated historical counter blocked offer", branch)
	}
	c.conn = &failedWorldConn{}
	es := &eventSession{mapID: 10002, click: 5, ev: ev, branch: 8}
	c.event = es
	op := world.DecodeOp(ev.Branches[8].Operations[len(ev.Branches[8].Operations)-1])
	ok, err := s.convertBreillat(context.Background(), c, es, op)
	if !ok || err == nil {
		t.Fatal("receipt did not fail", ok, err)
	}
	chars, e := s.Store.Characters(context.Background(), c.account.ID)
	if e != nil || chars[0].Body != 4 || chars[0].Head != 3 || chars[0].Quests[50031].Step != 1 {
		t.Fatal("failed receipt lost durable conversion", e)
	}
}
