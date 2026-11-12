package app

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
	"wonderland-gonline/internal/protocol"
)

func TestAgentWalkAcceptsPendingWaterBoarding(t *testing.T) {
	c, _, sent := shoreClient(2)
	c.UI = &seui.Manager{}
	if _, err := c.AgentSubmit([]byte(`{"command":"walk.to","arguments":{"x":82,"y":55}}`)); err != nil {
		t.Fatal(err)
	}
	c.agentTick()
	results := c.AgentCommandResults()
	if len(*sent) != 1 || !bytes.Equal((*sent)[0], []byte{15, 14, 1, 144, 187}) || c.waterTravel.requestID == 0 {
		t.Fatal("boarding was not requested", *sent)
	}
	if results[len(results)-1].Status != "executed" {
		t.Fatal("pending successful boarding was reported rejected", results)
	}
}

func TestHeadlessMapTransitionsReleaseAgentMovement(t *testing.T) {
	c, now, _ := enteredClient(t)
	for _, mapID := range []uint16{10027, 10017} {
		c.dispatch(protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.World.Player.ID).U16(mapID).U16(674).U16(1067))
		if !c.mapLoading.active {
			t.Fatal("warp did not start loading")
		}
		c.World.Areas = nil
		c.Sprites.WaitNative()
		for range 3 {
			*now = now.Add(time.Second)
			c.frame(false)
		}
		if !c.sceneFrozen() {
			t.Fatal("headless loading bypassed server readiness")
		}
		if c.AgentDetails()["mapLoading"] != true || c.AgentDetails()["sceneFrozen"] != true {
			t.Fatal("API omitted the loading input blocker")
		}
		c.dispatch([]byte{protocol.CommandCharacterState, protocol.CharacterStateRefresh})
		for range 4 {
			*now = now.Add(time.Second)
			c.frame(false)
		}
		if c.sceneFrozen() || c.mapLoading.active {
			t.Fatal("headless simulation retained a completed loading lock", mapID)
		}
		if c.AgentDetails()["mapLoading"] != false || c.AgentDetails()["sceneFrozen"] != false {
			t.Fatal("API retained the loading input blocker")
		}
		// Use a small clear patch to isolate command acceptance from map content.
		c.World.Scene.Ground = clientassets.GroundPrefix{GridWidth: 10, GridHeight: 10, Cells: make([]byte, 100)}
		c.World.Player.X, c.World.Player.Y = 22, 55
		if !c.AgentWalkWorld(82, 55) || !c.World.Walking() {
			t.Fatal("agent could not walk after headless map transition", mapID)
		}
	}
}
