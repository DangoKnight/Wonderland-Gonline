package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

func TestInstalledBreillatVoucherExchange(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	for _, mapID := range []uint16{10002, 10021, 10022, 10023, 10024, 10025, 10026, 10027, 10028, 10029, 10030} {
		for _, count := range []byte{1, 2, 3} {
			t.Run(fmt.Sprintf("map%d/count%d", mapID, count), func(t *testing.T) {
				s, players, _ := worldFixture(t)
				c := players[0]
				s.Assets, s.World = catalog, w
				c.character.Map = mapID
				c.character.Bag = game.Inventory{}
				c.character.Bag[0] = game.Item{ID: 30002, Count: count}
				c.character.Quests[50030] = game.Quest{ID: 50030, State: game.InProgress, Step: 10}
				saveBreillatFixture(t, s, c)
				checkpoint := c.character.Clone()
				c.autosaveBaseline = &checkpoint
				c.character.X += 20
				before := c.character.Clone()
				ev, _ := w.Event(mapID, 4)
				branch := w.FindBranch(c.character, c.view, mapID, ev, 0, 0, 0, -1)
				wantBranch, reward, rewardCount, debit := byte(5), uint16(32072), byte(2), byte(2)
				if mapID != 10002 {
					rewardCount = 1
				}
				if count == 1 {
					wantBranch, reward, rewardCount, debit = 6, 32071, 1, 1
				}
				if ev.Branches[branch].Index != wantBranch {
					t.Fatal("greeting or transformation shadows exchange")
				}
				if err := s.startEvent(context.Background(), c, 5, ev, branch, true); err != nil {
					t.Fatal(err)
				}
				if c.character.Bag != before.Bag {
					t.Fatal("items changed before dialogue acknowledgment")
				}
				finishBreillatDialogue(t, s, c)
				if c.event != nil {
					t.Fatal("exchange did not finish")
				}
				var vouchers, rewards byte
				for _, it := range c.character.Bag {
					if it.ID == 30002 {
						vouchers += it.Count
					}
					if it.ID == reward {
						rewards += it.Count
					}
				}
				if vouchers != count-debit || rewards != rewardCount {
					t.Fatalf("vouchers=%d reward=%d", vouchers, rewards)
				}
				if !reflect.DeepEqual(before.Quests, c.character.Quests) || before.Body != c.character.Body {
					t.Fatal("exchange changed transformation progress")
				}
				after := c.character.Clone()
				if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
				if c.character.Bag != after.Bag {
					t.Fatal("duplicate acknowledgment repeated exchange")
				}
				stored, err := breillatStoredCharacter(s, c)
				if err != nil || stored.Bag != after.Bag || stored.X != checkpoint.X || c.character.X != before.X {
					t.Fatal("exchange not durable", err)
				}
			})
		}
	}
}

func TestInstalledBreillatExchangeFailures(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	w := world.New(catalog)
	ev, _ := w.Event(10002, 4)
	for _, scenario := range []string{"no vouchers", "wrong voucher", "full bag", "locked voucher", "stale voucher", "fresh full bag", "save failure"} {
		t.Run(scenario, func(t *testing.T) {
			s, players, _ := worldFixture(t)
			c := players[0]
			s.Assets, s.World = catalog, w
			c.character.Map = 10002
			c.character.Bag = game.Inventory{}
			c.character.Bag[0] = game.Item{ID: 30002, Count: 3}
			if scenario == "no vouchers" {
				c.character.Bag[0] = game.Item{}
			}
			if scenario == "wrong voucher" {
				c.character.Bag[0].ID = 30030
			}
			fill := func(char *game.Character) {
				for i := 1; i < len(char.Bag); i++ {
					char.Bag[i] = game.Item{ID: 32071, Count: 50}
				}
			}
			if scenario == "full bag" {
				fill(c.character)
			}
			if scenario == "locked voucher" {
				c.character.Bag[0].Locked = true
			}
			saveBreillatFixture(t, s, c)
			branch := w.FindBranch(c.character, c.view, 10002, ev, 0, 0, 0, -1)
			if scenario == "no vouchers" || scenario == "wrong voucher" {
				if ev.Branches[branch].Index == 5 || ev.Branches[branch].Index == 6 {
					t.Fatal("invalid voucher accepted")
				}
				return
			}
			eventCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if err := s.startEvent(eventCtx, c, 5, ev, branch, true); err != nil {
				t.Fatal(err)
			}
			if scenario == "stale voucher" || scenario == "fresh full bag" {
				if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error {
					if scenario == "stale voucher" {
						stored.Bag[0] = game.Item{}
					} else {
						fill(stored)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			before, err := breillatStoredCharacter(s, c)
			if err != nil {
				t.Fatal(err)
			}
			if c.event != nil && c.event.onComplete != nil {
				if scenario == "save failure" {
					cancel()
				}
				err := s.eventCommand(context.Background(), c, []byte{20, 6})
				if scenario == "save failure" && err == nil {
					t.Fatal("save failure hidden")
				}
				if scenario != "save failure" && err != nil {
					t.Fatal(err)
				}
			}
			after, err := breillatStoredCharacter(s, c)
			if err != nil || after.Bag != before.Bag {
				t.Fatal("failed exchange changed durable inventory", err)
			}
			if c.event != nil {
				t.Fatal("failed exchange retained event")
			}
		})
	}
}

func breillatStoredCharacter(s *Server, c *Session) (game.Character, error) {
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		return game.Character{}, err
	}
	for _, char := range chars {
		if char.ID == c.character.ID {
			return char, nil
		}
	}
	return game.Character{}, fmt.Errorf("missing character %d", c.character.ID)
}

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
		if !ok || !w.ValidStoryProgram(mapID, ev) {
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
		if c.character.Bag != original.Bag || c.character.Base != original.Base || c.character.Name != original.Name || c.character.EXP != original.EXP {
			t.Fatal("conversion destroyed player state")
		}
		if got := c.character.Equipment.IDs(); got != [6]uint16{22007, 21009, 10002, 0, 24009, 0} {
			t.Fatal("missing Breillat outfit", got)
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
		// Cabin and its native map variants are personal introductory scenes.
		// Conversion still reaches its owner, but must not expose them to peers.
		if peers := wires[1].packets(t); len(peers) != 0 {
			t.Fatal("conversion leaked to another player in Cabin", peers)
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
			ok, err := s.transformStoryCharacter(ctx, c, es, op)
			if scenario == "valid stale inventory" || scenario == "zero conversion mark" {
				if !ok || err != nil || c.character.Body != 4 || scenario == "valid stale inventory" && (c.character.Gold != 12345 || c.character.Bag[3].ID != 30002) {
					t.Fatal("fresh player state lost", ok, err)
				}
				ok, err = s.transformStoryCharacter(ctx, c, es, op)
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
	ok, err := s.transformStoryCharacter(context.Background(), c, es, op)
	if !ok || err == nil {
		t.Fatal("receipt did not fail", ok, err)
	}
	chars, e := s.Store.Characters(context.Background(), c.account.ID)
	if e != nil || chars[0].Body != 4 || chars[0].Head != 3 || chars[0].Quests[50031].Step != 1 || chars[0].Equipment.IDs() != [6]uint16{22007, 21009, 10002, 0, 24009, 0} {
		t.Fatal("failed receipt lost durable conversion", e)
	}
}

func TestInstalledBreillatOutfitReplacement(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"mixed gear", "full bag", "locked standard", "locked worn", "fresh SQL gear"} {
		t.Run(scenario, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			s.Assets, s.World = catalog, world.New(catalog)
			c.character.Map = 10002
			c.character.Quests[50030] = game.Quest{ID: 50030, State: game.InProgress, Step: 10}
			c.character.Bag = game.Inventory{}
			c.character.Bag[0] = game.Item{ID: 21001, Count: 1}
			c.character.Bag[1] = game.Item{ID: 24001, Count: 1}
			returned := game.Item{ID: 23001, Count: 1, Damage: 17}
			returned.Metadata[game.ForgeMetadataOffset] = 5
			returned.Metadata[0] = 42
			c.character.Equipment[3] = returned
			switch scenario {
			case "full bag":
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
				}
			case "locked standard":
				c.character.Bag[0].Locked = true
			case "locked worn":
				c.character.Equipment[3].Locked = true
			}
			saveBreillatFixture(t, s, c)
			if scenario == "fresh SQL gear" {
				if err := s.Store.UpdateCharacter(context.Background(), c.account.ID, c.character.ID, func(stored *game.Character) error {
					stored.Equipment[3].Damage = 23
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				returned.Damage = 23
			}
			before, err := breillatStoredCharacter(s, c)
			if err != nil {
				t.Fatal(err)
			}
			ev, _ := s.World.Event(10002, 4)
			es := &eventSession{mapID: 10002, click: 5, ev: ev, branch: 8}
			c.event = es
			op := world.DecodeOp(ev.Branches[8].Operations[len(ev.Branches[8].Operations)-1])
			ok, err := s.transformStoryCharacter(context.Background(), c, es, op)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := breillatStoredCharacter(s, c)
			if err != nil {
				t.Fatal(err)
			}
			successful := scenario == "mixed gear" || scenario == "fresh SQL gear"
			if ok != successful {
				t.Fatal("unexpected conversion", ok)
			}
			if !successful {
				if !reflect.DeepEqual(before, stored) {
					t.Fatal("failed conversion changed durable state")
				}
				for _, p := range wires[0].packets(t) {
					if len(p) > 1 && (p[0] == 23 || p[0] == 5 && p[1] == 12) {
						t.Fatal("failed conversion published item/model changes", p)
					}
				}
				return
			}
			if stored.Equipment.IDs() != [6]uint16{22007, 21009, 10002, 0, 24009, 0} || stored.Bag[0] != returned || !stored.Bag[1].Empty() {
				t.Fatal("outfit replacement was not durable", stored)
			}
			if !reflect.DeepEqual(stored, *c.character) {
				t.Fatal("session differs from SQL")
			}
			foundBag, foundEquipment := false, false
			clientBag := before.Bag
			for _, p := range wires[0].packets(t) {
				if len(p) > 1 && p[0] == 23 && p[1] == 5 {
					foundBag = true
					for offset := 2; offset < len(p); offset += 5 + game.ItemMetadataBytes {
						item := game.Item{ID: binary.LittleEndian.Uint16(p[offset+1:]), Count: p[offset+3], Damage: p[offset+4]}
						copy(item.Metadata[:], p[offset+5:offset+5+game.ItemMetadataBytes])
						if !clientBag[p[offset]-1].Empty() {
							t.Fatal("returned equipment overwrote a client item")
						}
						clientBag[p[offset]-1] = item
					}
				}
				if len(p) == 4 && p[0] == 23 && p[1] == 9 {
					if err := clientBag.Remove(p[2], p[3]); err != nil {
						t.Fatal("invalid bag removal receipt", err)
					}
				}
				if bytes.Equal(p, stored.EquipmentPacket()) {
					foundEquipment = true
				}
			}
			if !foundBag || !foundEquipment {
				t.Fatal("missing committed item receipts", foundBag, foundEquipment)
			}
			if clientBag != stored.Bag {
				t.Fatal("client bag receipts differ from committed state")
			}
		})
	}
}

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
		if len(packets) != 1 || !bytes.Equal(packets[0], serverwire.DialogueFrame(byte(step), portraits[step-1], speaker, uint32(20037+step))) || len(c.character.Pets) != 0 {
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
	if err := s.startRecruitment(ctx, c, 1); err != nil {
		t.Fatal(err)
	}
	if got := wire.packets(t); len(got) != 2 || !bytes.Equal(got[1], serverwire.DialogueFrame(1, 3, 1, 20042)) {
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
		if !contains(wire.packets(t), serverwire.DialogueFrame(12, 7, 0, 31146)) {
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

func TestMonkeyAuthoredMoviesBeforeAtomicRecruitment(t *testing.T) {
	for _, scenario := range []string{"success", "full", "cancel", "save failure"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, wire := monkeyFixture(t)
			ctx := context.Background()
			ev := assets.Event{ClickID: 1, Branches: []assets.Branch{{Index: 7, Operations: []assets.Operation{
				evOp(1, 8, 2, 0, 0, 12000), evOp(2, 8, 1, 0, 0, 11011),
			}}}}
			s.Assets.Maps[11016] = assets.Map{ID: 11016, NPCs: []assets.MapNPC{{ClickID: 1, Template: 17162, Flags: 1, X: 1050, Y: 1080, Events: []byte{1}}}, Events: []assets.Event{ev}}
			s.World = world.New(s.Assets)
			next := c.character.Clone()
			next.Map = 11016
			if scenario == "full" {
				for i := 0; i < 4; i++ {
					next.Pets = append(next.Pets, game.Pet{ID: uint32(14080 + i), Slot: byte(i + 1), Name: "Companion", Level: 1})
				}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			wire.Reset()
			if err := s.worldCommand(ctx, c, []byte{20, 1, 1, 0}); err != nil {
				t.Fatal(err)
			}
			first := []byte{20, 1, 0, 0, 0, 1, 5, 0, 0, 0, 1, 224, 46, 0, 0, 0, 0, 7}
			if packets := wire.packets(t); len(packets) != 4 || !bytes.Equal(packets[1], first) {
				t.Fatal("first authored movie", packets)
			}
			if scenario == "cancel" {
				if err := s.worldCommand(ctx, c, []byte{20, 9, 40}); err != nil {
					t.Fatal(err)
				}
				if c.event != nil || len(c.character.Pets) != 0 || c.character.Quests[12002].ID != 0 {
					t.Fatal("cancel granted rescue")
				}
				return
			}
			if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if scenario == "full" {
				if packets := wire.packets(t); !contains(packets, serverwire.DialogueFrame(1, 7, 0, 31146)) {
					t.Fatal("full-party follow-up", packets)
				}
				for i := 0; i < 2; i++ {
					if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
						t.Fatal(err)
					}
				}
				if c.event != nil || c.character.Quests[12002].ID != 0 {
					t.Fatal("full party consumed rescue")
				}
				return
			}
			second := []byte{20, 1, 0, 0, 0, 2, 5, 0, 0, 0, 1, 3, 43, 0, 0, 0, 0, 7}
			if packets := wire.packets(t); len(packets) != 1 || !bytes.Equal(packets[0], second) {
				t.Fatal("second authored movie", packets)
			}
			if c.character.Quests[12002].ID != 0 {
				t.Fatal("rescue committed before movies finished")
			}
			if scenario == "save failure" {
				if err := s.Store.Close(); err != nil {
					t.Fatal(err)
				}
				if err := s.worldCommand(ctx, c, []byte{20, 6}); err == nil {
					t.Fatal("failed save accepted")
				}
				if len(c.character.Pets) != 0 || c.character.Quests[12002].ID != 0 || wire.Len() != 0 || c.view.Hidden[1] {
					t.Fatal("success before durable commit")
				}
				return
			}
			if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if scenario == "full" {
				if packets := wire.packets(t); !contains(packets, serverwire.DialogueFrame(1, 7, 0, 31146)) {
					t.Fatal("missing full-party explanation", packets)
				}
				for i := 0; i < 2; i++ {
					if err := s.worldCommand(ctx, c, []byte{20, 6}); err != nil {
						t.Fatal(err)
					}
				}
				if c.event != nil || len(c.character.Pets) != 4 || c.character.Quests[12002].ID != 0 {
					t.Fatal("full party consumed rescue")
				}
				return
			}
			if c.event != nil || len(c.character.Pets) != 1 || c.character.Quests[12002].State != game.Completed {
				t.Fatal("movies did not finish rescue")
			}
		})
	}
}

func TestRobinsonRecoveryCandidateScopeAndOrdering(t *testing.T) {
	for _, scenario := range []string{"rescue", "fresh", "completed", "other_map", "other_actor", "owned_npc_alias", "owned_pet_alias", "already_linked", "missing_event"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, _ := eventFixture(t)
			c.character.Map = 10035
			c.character.Quests[12047] = game.Quest{ID: 12047, State: game.InProgress, Step: 1}
			greeting := assets.Event{ClickID: 8, Branches: []assets.Branch{{Index: 1}}}
			recruitment := assets.Event{ClickID: 19, Branches: []assets.Branch{{Index: 5}}}
			m := assets.Map{ID: 10035, Events: []assets.Event{greeting, recruitment}}
			click := uint16(1)
			switch scenario {
			case "fresh":
				delete(c.character.Quests, 12047)
			case "completed":
				c.character.Quests[12047] = game.Quest{ID: 12047, State: game.Completed}
			case "other_map":
				c.character.Map = 11016
			case "other_actor":
				click = 7
			case "owned_npc_alias":
				c.character.Pets = []game.Pet{{ID: 12032}}
			case "owned_pet_alias":
				c.character.Pets = []game.Pet{{ID: 12178}}
			case "missing_event":
				m.Events = m.Events[:1]
			}
			s.Assets.Maps[10035] = m
			s.World = world.New(s.Assets)
			greet, _ := s.World.Event(10035, 8)
			rescue, _ := s.World.Event(10035, 19)
			before := []*assets.Event{greet}
			if scenario == "already_linked" {
				before = append(before, rescue)
			}
			got := s.storyCandidates(c.character, click, before)
			if scenario == "rescue" {
				if len(got) != 2 || got[0] != rescue || got[1] != greet {
					t.Fatal("recovery priority", got)
				}
			} else if !reflect.DeepEqual(got, before) {
				t.Fatal("changed unrelated/existing candidate order", got)
			}
		})
	}
}

// The same unmodified SQL EVE scripts drive both original rescue maps. This
// reproduces reference Test-RobinsonFullChain without its mocked Windows runtime.
func TestNativeRobinsonFullChainAndRecovery(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, mapID := range []uint16{10035, 10039} {
		for _, resume := range []bool{false, true} {
			t.Run(fmt.Sprintf("map_%d_resume_%t", mapID, resume), func(t *testing.T) {
				s, c, wire := eventFixture(t)
				s.Assets, s.World = catalog, world.New(catalog)
				next := c.character.Clone()
				next.Map = mapID
				next.Bag = game.Inventory{}
				next.Quests = map[uint32]game.Quest{}
				next.Pets = nil
				if resume {
					next.Quests[12046] = game.Quest{ID: 12046, State: game.Completed, Step: 1}
					next.Quests[12047] = game.Quest{ID: 12047, State: game.InProgress, Step: 1}
				}
				if err := s.commit(context.Background(), c, next); err != nil {
					t.Fatal(err)
				}
				ev, ok := s.World.Event(mapID, 19)
				if !ok {
					t.Fatal("native recruitment event absent")
				}
				wire.Reset()
				if resume {
					handled, err := s.runNPCEvent(context.Background(), c, 1)
					if err != nil || !handled {
						t.Fatal("recovery click", handled, err)
					}
				} else {
					branch := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1)
					if branch < 0 || ev.Branches[branch].Index != 1 {
						t.Fatal("initial native branch", branch)
					}
					if err := s.startEvent(context.Background(), c, 7, ev, branch, true); err != nil {
						t.Fatal(err)
					}
					if c.event == nil || c.event.ev.Branches[c.event.branch].Index != 3 || bagCount(c.character.Bag, 48016) != 1 {
						t.Fatal("raft-to-dialogue handoff")
					}
					for step := 0; c.event != nil && step < 32; step++ {
						if err := s.worldCommand(context.Background(), c, []byte{20, 6}); err != nil {
							t.Fatal(err)
						}
					}
				}
				if c.event != nil || len(c.character.Pets) != 1 || c.character.Pets[0].ID != 12032 || c.character.Quests[12046].State != game.Completed || c.character.Quests[12047].Step != 1 || c.character.Quests[15282].State != game.Completed || c.character.Quests[15283].Step != 1 {
					t.Fatal("recruitment chain incomplete", c.character.Quests, c.character.Pets)
				}
				if !c.view.Hidden[1] || s.World.Visible(c.character, mapID, 1) {
					t.Fatal("Robinson actor remained visible")
				}
				if !resume && bagCount(c.character.Bag, 48016) != 1 || resume && bagCount(c.character.Bag, 48016) != 0 {
					t.Fatal("raft redelivered during recovery")
				}
				saved, err := s.Store.Characters(context.Background(), c.account.ID)
				if err != nil || len(saved[0].Pets) != 1 || saved[0].Quests[15283].Step != 1 || saved[0].Bag != c.character.Bag {
					t.Fatal("recruitment not durable", err)
				}
				// Re-entering Event 19 cannot deliver another raft, pet or mark increment.
				if branch := s.World.FindBranch(c.character, c.view, mapID, ev, world.TriggerEntry, 0, 0, -1); branch >= 0 {
					t.Fatal("completed recruitment has a replayable branch", branch)
				}
			})
		}
	}
}

func TestNativeRobinsonRecoveryCapacityAndFailedSave(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"full_party", "failed_save"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, wire := eventFixture(t)
			s.Assets, s.World = catalog, world.New(catalog)
			ctx := context.Background()
			next := c.character.Clone()
			next.Map = 10035
			next.Bag = game.Inventory{}
			next.Quests = map[uint32]game.Quest{12046: {ID: 12046, State: game.Completed, Step: 1}, 12047: {ID: 12047, State: game.InProgress, Step: 1}}
			if scenario == "full_party" {
				for i, id := range []uint32{14162, 14081, 14156, 14175} {
					next.Pets = append(next.Pets, game.NewPet(id, "Existing", byte(i+1), s.petTemplate(id), catalog.Items))
				}
			}
			if err := s.commit(ctx, c, next); err != nil {
				t.Fatal(err)
			}
			before := c.character.Clone()
			requestCtx := ctx
			if scenario == "failed_save" {
				var cancel context.CancelFunc
				requestCtx, cancel = context.WithCancel(ctx)
				cancel()
			}
			wire.Reset()
			handled, err := s.runNPCEvent(requestCtx, c, 1)
			if !handled || (scenario == "failed_save" && err == nil) || (scenario == "full_party" && err != nil) {
				t.Fatal("rejected recovery", handled, err)
			}
			if !reflect.DeepEqual(c.character.Quests, before.Quests) || !reflect.DeepEqual(c.character.Pets, before.Pets) || c.character.Bag != before.Bag {
				t.Fatal("failed recovery published rewards/progress")
			}
			saved, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil || !reflect.DeepEqual(saved[0].Quests, before.Quests) || !reflect.DeepEqual(saved[0].Pets, before.Pets) {
				t.Fatal("failed recovery persisted changes", err)
			}
			if scenario == "full_party" {
				next = c.character.Clone()
				next.Pets = next.Pets[:game.MaxPets-1]
				if err := s.commit(ctx, c, next); err != nil {
					t.Fatal(err)
				}
			}
			s.endEvent(c)
			if handled, err := s.runNPCEvent(ctx, c, 1); !handled || err != nil {
				t.Fatal("recovery retry", handled, err)
			}
			if _, owned := c.character.Pet(12032); !owned || c.character.Quests[15283].Step != 1 || bagCount(c.character.Bag, 48016) != 0 {
				t.Fatal("retry did not recruit exactly once without a raft")
			}
		})
	}
}
