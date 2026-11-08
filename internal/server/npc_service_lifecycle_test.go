package server

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/world"
)

// Minimal independent model of the native event gate, not a server ACK stub.
// Receive AC20:9 clears +0x7109; AC20:8 ends the event. Service hide/treatment
// completion sets +0x7108, and the native loop sends AC20:6 only with no wait.
type nativeServiceGate struct {
	waiting, released bool
}

func (g *nativeServiceGate) receive(packets [][]byte) {
	for _, p := range packets {
		if bytes.Equal(p, []byte{20, 9}) {
			g.waiting = false
		}
		if bytes.Equal(p, []byte{20, 8}) {
			g.waiting, g.released = false, true
		}
	}
}

func (g *nativeServiceGate) complete(t *testing.T) []byte {
	t.Helper()
	if g.waiting || g.released {
		t.Fatal("native client cannot acknowledge service completion", *g)
	}
	return []byte{20, 6}
}

func TestNPCServiceOwnedCompletion(t *testing.T) {
	for _, kind := range []uint16{world.ServiceWeaponShop, world.ServicePropsShop, world.ServiceArmorShop, world.ServicePropsKeeper, world.ServiceStockKeeper, world.ServicePetHotel, world.ServiceClinic} {
		for _, authored := range []bool{false, true} {
			t.Run(fmt.Sprintf("service_%d/authored_%t", kind, authored), func(t *testing.T) {
				s, players, wires := worldFixture(t)
				c, wire := players[0], wires[0]
				if authored {
					ev := &assets.Event{ClickID: 65000, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionServiceOrTeleport, kind, 0, 0, 0)}}}}
					if err := s.startEvent(context.Background(), c, 38, ev, 0, false); err != nil {
						t.Fatal(err)
					}
				} else if err := s.openService(c, kind); err != nil {
					t.Fatal(err)
				}
				if c.event == nil || c.event.serviceOwner != c.character.ID || c.event.serviceKind != kind {
					t.Fatal("service has no completion owner")
				}
				gate := nativeServiceGate{waiting: true}
				gate.receive(wire.packets(t))
				if gate.released {
					t.Fatal("service released before close/completion")
				}
				if err := s.eventCommand(context.Background(), c, gate.complete(t)); err != nil {
					t.Fatal(err)
				}
				packets := wire.packets(t)
				gate.receive(packets)
				resumes := 0
				for _, p := range packets {
					if bytes.Equal(p, []byte{20, 8}) {
						resumes++
					}
				}
				if resumes != 1 || c.event != nil || !gate.released {
					t.Fatal("service did not complete exactly once", packets)
				}
				if c.storageMap != 0 || c.hotelMap != 0 || c.restMap != 0 || c.saleMap != 0 || len(c.view.ShopItems) != 0 {
					t.Fatal("closed service retained resource authority")
				}
				if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
				if wire.Len() != 0 {
					t.Fatal("duplicate completion started a resume loop")
				}
				for _, peer := range wires[1:] {
					if peer.Len() != 0 {
						t.Fatal("private service packets leaked")
					}
				}
			})
		}
	}
}

func TestNPCSellClearsNativeWaitBeforeClose(t *testing.T) {
	for _, mode := range []int{0, 1} {
		t.Run(fmt.Sprintf("mode_%d", mode), func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c, wire := players[0], wires[0]
			c.event = &eventSession{mapID: c.character.Map}
			c.saleMode, c.saleMap = mode, c.character.Map
			if err := s.openNPCSaleWindow(c); err != nil {
				t.Fatal(err)
			}
			// Independent wire golden: AC27:3/4 opens Sell; AC20:9 clears
			// +0x7109. FUN_0036c8c8 only sets +0x7108 on close, so without
			// the latter packet the native loop cannot send its AC20:6.
			sub := byte(4)
			if mode == 1 {
				sub = 3
			}
			packets := wire.packets(t)
			if len(packets) != 2 || !bytes.Equal(packets[0], []byte{27, sub}) || !bytes.Equal(packets[1], []byte{20, 9}) {
				t.Fatal("Sell left native completion blocked", packets)
			}
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || c.saleMap != 0 || !contains(wire.packets(t), []byte{20, 8}) {
				t.Fatal("Sell close did not release player")
			}
		})
	}
}

func TestNPCServiceFallbackQuestionAcknowledgements(t *testing.T) {
	for _, test := range []struct {
		name    string
		npc     world.NPC
		choices []byte
		kind    uint16
	}{
		{"sell", world.NPC{ClickID: 22, Template: npcPropsMerchantTemplate}, []byte{31}, world.ServicePropsShop},
		{"buy_then_sell", world.NPC{ClickID: 22, Template: npcPropsMerchantTemplate}, []byte{30, 31}, world.ServicePropsShop},
		{"buy", world.NPC{ClickID: 22, Template: npcPropsMerchantTemplate}, []byte{30, 30}, world.ServicePropsShop},
		{"treatment", world.NPC{ClickID: 22, Template: 14151}, []byte{30}, world.ServiceClinic},
		{"hotel", world.NPC{ClickID: 22, Template: 14151}, []byte{32}, world.ServicePetHotel},
		{"record_point", world.NPC{ClickID: 22, Template: 14151}, []byte{31}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c, wire := players[0], wires[0]
			ctx := context.Background()
			if handled, err := s.fallbackService(ctx, c, test.npc); err != nil || !handled {
				t.Fatal(handled, err)
			}
			wire.Reset()
			for _, choice := range test.choices {
				if err := s.eventCommand(ctx, c, []byte{20, 9, choice}); err != nil {
					t.Fatal(err)
				}
				if c.event != nil && c.event.serviceOwner != 0 {
					gate := nativeServiceGate{waiting: true}
					packets := wire.packets(t)
					gate.receive(packets)
					gate.complete(t) // Do not inject a close the native client cannot send.
				}
				// Mode-zero answer ACK is not the window-close/treatment ACK.
				if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
					t.Fatal(err)
				}
				if c.event == nil || contains(wire.packets(t), []byte{20, 8}) {
					t.Fatal("answer acknowledgement closed pending interaction")
				}
			}
			if c.event.serviceKind != test.kind {
				t.Fatal("wrong service", c.event.serviceKind)
			}
			if err := s.eventCommand(ctx, c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
				t.Fatal("window/dialogue close was not acknowledged")
			}
		})
	}
}

func TestClinicCompletionWaitsForTreatmentAcknowledgement(t *testing.T) {
	s, players, wires := worldFixture(t)
	c, wire := players[0], wires[0]
	next := c.character.Clone()
	next.HP = 1
	if err := s.commit(context.Background(), c, next); err != nil {
		t.Fatal(err)
	}
	if err := s.openService(c, world.ServiceClinic); err != nil {
		t.Fatal(err)
	}
	wire.Reset()
	if err := s.confirmRest(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	if !contains(packets, []byte{31, 10}) || contains(packets, []byte{20, 8}) || c.event == nil {
		t.Fatal("treatment completed before animation acknowledgement")
	}
	if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
		t.Fatal(err)
	}
	if c.event != nil || !contains(wire.packets(t), []byte{20, 8}) {
		t.Fatal("treatment completion left interaction locked")
	}
}

func TestIndependentNPCServiceOpensAfterConversation(t *testing.T) {
	for _, test := range []struct {
		kind uint16
		open []byte
	}{
		{world.ServiceBank, []byte{29, 5}},
		{world.ServiceMessageBoard, []byte{82, 9}},
		{world.ServiceRebornJobSelection, []byte{protocol.CommandRebirthJobSelection, protocol.RebirthJobConfirm}},
		{world.ServiceCompanionRebirth, []byte{protocol.CommandCompanionRebirth, protocol.CompanionRebirthOpen}},
	} {
		t.Run(fmt.Sprint(test.kind), func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c, wire := players[0], wires[0]
			c.character.Level = 100
			ev := &assets.Event{ClickID: 65000, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{evOp(1, world.ActionServiceOrTeleport, test.kind, 0, 0, 0)}}}}
			if err := s.startEvent(context.Background(), c, 38, ev, 0, false); err != nil {
				t.Fatal(err)
			}
			resume, opened := -1, -1
			for i, p := range wire.packets(t) {
				if bytes.Equal(p, []byte{20, 8}) {
					resume = i
				}
				if bytes.Equal(p, test.open) {
					opened = i
				}
			}
			if resume < 0 || opened <= resume || c.event != nil {
				t.Fatal("independent form opened inside an active conversation", resume, opened)
			}
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if wire.Len() != 0 {
				t.Fatal("trailing dialogue ACK closed/reopened independent form")
			}
		})
	}
}

func TestNPCServiceCancellationReleasesEveryOwnedAuthority(t *testing.T) {
	for _, kind := range []uint16{world.ServicePropsShop, world.ServicePropsKeeper, world.ServicePetHotel, world.ServiceClinic} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			s, players, wires := worldFixture(t)
			c := players[0]
			if err := s.openService(c, kind); err != nil {
				t.Fatal(err)
			}
			old := c.event
			if err := s.eventCommand(context.Background(), c, []byte{20, 9, 40}); err != nil {
				t.Fatal(err)
			}
			if c.event != nil || old.serviceOwner != 0 || c.storageMap != 0 || c.hotelMap != 0 || c.restMap != 0 || c.saleMap != 0 || len(c.view.ShopItems) != 0 {
				t.Fatal("cancel retained service state")
			}
			wires[0].Reset()
			if err := s.eventCommand(context.Background(), c, []byte{20, 6}); err != nil {
				t.Fatal(err)
			}
			if wires[0].Len() != 0 {
				t.Fatal("cancelled service revived on late ACK")
			}
		})
	}
}
