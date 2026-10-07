package server

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"wonderland-gonline/internal/game"
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
