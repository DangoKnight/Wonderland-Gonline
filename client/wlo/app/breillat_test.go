package app

import (
	"bytes"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/game"
)

func TestBreillatVoucherExchangePackets(t *testing.T) {
	for _, notice := range []string{"none", "replaced", "hidden-by-event-end"} {
		t.Run(notice, func(t *testing.T) {
			c, _, _ := enteredClient(t)
			if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
				useCompiledPetAssets(t, c, bundle)
			}
			c.dispatch(selfPacket(10001, 2, 10002, 450, 450, 0, 444444444, 444444444, nil, "Tester"))
			c.mapReady = true
			if notice != "none" {
				beforePets := c.InventoryState.Pets
				c.InventoryState.Pets[0] = inventory.UsePet{ID: 17162, Name: []byte("S.Monkey")}
				c.announceNewPets(beforePets)
				c.eventTick()
				if !c.petAnnouncement || !c.Talk.Shown() {
					t.Fatal("pet notice absent")
				}
				if notice == "hidden-by-event-end" {
					// A map/event completion closes the local notice without a user click.
					c.dispatch([]byte{20, 8})
				}
			}
			c.InventoryState.Bag[0] = game.Item{ID: 30002, Count: 2}
			sent := wire(t, c)
			c.dispatch([]byte{6, 2, 1})
			c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 3, 5, 0, 1, 0, 0, 0, 0, 236, 119, 0})
			if !c.Talk.Shown() {
				t.Fatal("dialogue absent", c.Talk.Lines())
			}
			if c.petAnnouncement {
				t.Fatal("NPC dialogue retained the local notice flag")
			}
			c.GroundClick(400, 400)
			c.Frame()
			if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{20, 6}) {
				t.Fatal("dialogue not acknowledged", p, c.event, c.World.RoutesDone())
			}
			c.Frame()
			if len(sent()) != 1 {
				t.Fatal("dialogue acknowledgement repeated before the server replied")
			}
			// Independent payloads recorded from the installed Cabin exchange.
			for _, p := range [][]byte{
				{23, 9, 1, 2},
				{23, 5, 1, 72, 125, 2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
				{20, 10}, {6, 2, 0}, {20, 8},
			} {
				c.dispatch(p)
			}
			c.Frame()
			if c.event.active || c.held || c.Talk.Shown() {
				t.Fatal("event not released", c.event, c.held, c.Talk.Shown())
			}
			if c.InventoryState.Bag[0].ID != 32072 || c.InventoryState.Bag[0].Count != 2 {
				t.Fatal("reward absent", c.InventoryState.Bag[0])
			}
		})
	}
}

func TestMinigameResultAckSurvivesServerReplyBeforeNextFrame(t *testing.T) {
	for _, reply := range []string{"event-end", "next-dialogue"} {
		t.Run(reply, func(t *testing.T) {
			c, _, _ := enteredClient(t)
			sent := wire(t, c)
			c.sendSportResult(false)
			// A fast server can reply before the next client frame.
			c.dispatch([]byte{57, 2})
			if reply == "event-end" {
				c.dispatch([]byte{6, 2, 0})
				c.dispatch([]byte{20, 8})
			}
			// Breillat's first line must not replace the minigame acknowledgement.
			c.dispatch([]byte{6, 2, 1})
			c.dispatch([]byte{20, 1, 0, 0, 0, 1, 1, 3, 5, 0, 1, 0, 0, 0, 0, 236, 119, 0})
			c.eventTick()
			p := sent()
			if len(p) != 2 || !bytes.Equal(p[0], []byte{57, 1, 0}) || !bytes.Equal(p[1], []byte{20, 6}) || !c.Talk.Shown() {
				t.Fatal("lost minigame acknowledgement or advanced NPC dialogue", p)
			}
			c.GroundClick(400, 400)
			c.eventTick()
			if p := sent(); len(p) != 3 || !bytes.Equal(p[2], []byte{20, 6}) {
				t.Fatal("missing independent NPC dialogue acknowledgement", p)
			}
			c.eventTick()
			if len(sent()) != 3 {
				t.Fatal("duplicate acknowledgement")
			}
		})
	}
}
