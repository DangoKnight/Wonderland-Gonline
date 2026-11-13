package app

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestAgentNPCIDUsesLiveActorPosition(t *testing.T) {
	for _, tc := range []struct {
		name         string
		startX, endX int
		want         bool
	}{{"approaching", 1300, 1100, true}, {"departing", 1100, 1300, false}} {
		t.Run(tc.name, func(t *testing.T) {
			c := automationClient(t)
			now := time.Unix(100, 0)
			c.Now = func() time.Time { return now }
			c.mapReady = true
			c.Net = login.NewNet()
			cat := &world.NPC{ClickID: 8, Template: 11000, X: tc.startX, Y: 1000, Shown: true}
			c.World = &world.World{Player: world.Player{X: 1000, Y: 1000}, NPCs: map[uint16]*world.NPC{8: cat}}
			sent := wire(t, c)
			// Capture an old position, then let the server-driven walking leg age
			// without the ordinary frame update. The command must catch it up.
			if !c.World.ApplyActorWalk(protocol.Builder{}.U16(8).U16(uint16(tc.endX)).U16(1000).U8(8), now) {
				t.Fatal("actor walk rejected")
			}
			now = now.Add(2 * time.Second)
			if _, err := c.AgentSubmit([]byte(`{"command":"world.interactNPC","arguments":{"id":8}}`)); err != nil {
				t.Fatal(err)
			}
			c.agentTick()
			result := c.AgentCommandResults()[0]
			packets := sent()
			if tc.want {
				if result.Status != "executed" || len(packets) != 1 || !bytes.Equal(packets[0], []byte{20, 1, 8, 0}) {
					t.Fatal("ID interaction missed the moving cat", result, packets)
				}
			} else if result.Status != "rejected" || len(packets) != 0 || c.event.active {
				t.Fatal("stale nearby coordinates dispatched an out-of-range interaction", result, packets)
			}
			if cat.X != tc.endX {
				t.Fatal("actor position was not refreshed", cat.X)
			}
		})
	}
}

func TestAgentNPCIDRejectsInvalidTargetsRangeAndFailedSends(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		id                   uint16
		x, y                 int
		hidden, disconnected bool
		want                 bool
	}{
		{"template is not actor ID", 11000, 1000, 1000, false, false, false},
		{"hidden", 8, 1000, 1000, true, false, false},
		{"native axis edge", 8, 1169, 1000, false, false, true},
		{"outside native axis", 8, 1170, 1000, false, false, false},
		{"server circle edge", 8, 1120, 1160, false, false, true},
		{"outside server circle", 8, 1120, 1161, false, false, false},
		{"square corner", 8, 1160, 1160, false, false, false},
		{"disconnected", 8, 1000, 1000, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := automationClient(t)
			c.Net = login.NewNet()
			c.mapReady = true
			c.World = &world.World{Player: world.Player{X: 1000, Y: 1000}, NPCs: map[uint16]*world.NPC{8: {ClickID: 8, Template: 11000, X: tc.x, Y: tc.y, Shown: !tc.hidden}}}
			var sent func() [][]byte
			if !tc.disconnected {
				sent = wire(t, c)
			}
			err := c.AgentInteractNPCByID(tc.id)
			if (err == nil) != tc.want || c.event.active != tc.want {
				t.Fatal("wrong interaction eligibility or event lock", err, c.event.active)
			}
			if sent != nil && len(sent()) != map[bool]int{false: 0, true: 1}[tc.want] {
				t.Fatal("unexpected NPC click packets")
			}
		})
	}
}
