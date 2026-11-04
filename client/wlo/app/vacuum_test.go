package app

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/game"
)

func vacuumClient(t *testing.T, gonline bool, timer []byte) (*Client, *time.Time, func() [][]byte) {
	t.Helper()
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 2, 60000, 1282, 1315, 0, 444444444, 444444444, nil, "Dango"))
	c.mapReady = true
	c.InventoryState.Equipment[5] = game.Item{ID: 34077, Count: 1}
	sent := wire(t, c)
	if gonline {
		c.dispatch([]byte{1, 243, 71, 76, 67, 84, 1, 0})
	}
	c.clickNPC(c.World.NPCs[33])
	c.dispatch([]byte{23, 6, 76, 156, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch(timer)
	c.dispatch([]byte{20, 10})
	c.eventTick()
	c.dispatch([]byte{20, 8})
	if got := sent(); len(got) != 3 || !bytes.Equal(got[0], []byte{20, 1, 33, 0}) || !bytes.Equal(got[1], []byte{23, 73, 33}) || !bytes.Equal(got[2], []byte{20, 6}) {
		t.Fatal("native collection/start/ack sequence", got)
	}
	return c, now, sent
}

func TestVacuumServerSpecificRetryTiming(t *testing.T) {
	// Independent raw AC66:2 timer: native ID 11020, 30 seconds.
	timer := []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 0, 62, 64}
	for _, gonline := range []bool{false, true} {
		t.Run(map[bool]string{false: "Legacy fixed interval", true: "Gonline respawn plus grace"}[gonline], func(t *testing.T) {
			c, now, sent := vacuumClient(t, gonline, timer)
			wait := 190 * time.Second
			if gonline {
				wait = 40 * time.Second
			}
			if c.combat.enabled || !c.vacuum.next.Equal(now.Add(wait)) {
				t.Fatal("timing depends on combat or ignores timer", c.vacuum)
			}
			*now = now.Add(wait - time.Nanosecond)
			c.vacuumTick()
			if len(sent()) != 3 {
				t.Fatal("retry before deadline")
			}
			*now = now.Add(time.Nanosecond)
			c.vacuumTick()
			if got := sent(); len(got) != 4 || !bytes.Equal(got[3], []byte{20, 1, 33, 0}) {
				t.Fatal("missing retry", got)
			}
			c.vacuumTick()
			if len(sent()) != 4 {
				t.Fatal("duplicate in-flight retry")
			}
			// A newly authored 240-second cooldown replaces the previous 30 seconds.
			c.dispatch([]byte{23, 6, 76, 156, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
			c.dispatch([]byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 0, 110, 64})
			next := 190 * time.Second
			if gonline {
				next = 250 * time.Second
			}
			if !c.vacuum.next.Equal(now.Add(next)) {
				t.Fatal("changed respawn ignored", c.vacuum.next)
			}
		})
	}
}

func TestVacuumStopsAndConnectionIsolation(t *testing.T) {
	for _, reason := range []string{"movement", "unequip", "map", "battle"} {
		t.Run(reason, func(t *testing.T) {
			c, _, sent := vacuumClient(t, true, []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64})
			switch reason {
			case "movement":
				c.sendLeg(1, 1300, 1315)
			case "unequip":
				c.InventoryState.Equipment[5] = game.Item{}
				c.vacuumTick()
			case "map":
				c.World.Player.Map++
				c.vacuumTick()
			case "battle":
				c.battle.state.Active = true
				c.vacuumTick()
			}
			if c.vacuum.actor != 0 || !bytes.Equal(sent()[3], []byte{23, 73, 0}) {
				t.Fatal("run survived cancellation", c.vacuum, sent())
			}
		})
	}
	c, _, _ := enteredClient(t)
	for _, p := range [][]byte{{1, 243, 1}, {1, 243, 71, 76, 67, 84, 2, 0}, {1, 243, 71, 76, 67, 84, 1, 0, 0}} {
		c.dispatch(p)
		if c.gonlineCollectionTiming {
			t.Fatal("generic echo or malformed capability enabled custom timing")
		}
	}
	c.dispatch([]byte{1, 243, 71, 76, 67, 84, 1, 0})
	if !c.gonlineCollectionTiming || c.combat.enabled {
		t.Fatal("collection capability did not stay independent")
	}
	c.Servers.OnConnecting()
	if c.gonlineCollectionTiming || c.vacuum.actor != 0 {
		t.Fatal("capability leaked to next server")
	}
}

func TestVacuumDoesNotStartFromUnrelatedTimerOrNPC(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	c.InventoryState.Equipment[5] = game.Item{ID: 34077, Count: 1}
	c.prepareVacuum(256)
	if c.vacuum.actor != 0 {
		t.Fatal("truncated actor ID")
	}
	c.prepareVacuum(33)
	c.acquisitionActor = 34
	c.vacuum.reward = true
	c.eventTimerPacket([]byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64})
	if c.vacuum.started {
		t.Fatal("unrelated actor timer started vacuum")
	}
	c.acquisitionActor = 33
	c.vacuum.reward = false
	c.eventTimerPacket([]byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64})
	if c.vacuum.started {
		t.Fatal("ordinary NPC timer started vacuum without collection")
	}
}

func TestVacuumWaitsThroughBusyEventAndStartsOnExhaustedSpot(t *testing.T) {
	c, now, sent := vacuumClient(t, true, []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 0, 62, 64})
	*now = now.Add(40 * time.Second)
	c.held = true
	c.vacuumTick()
	if len(sent()) != 3 {
		t.Fatal("retried while locked")
	}
	c.held = false
	c.vacuumTick()
	if len(sent()) != 4 {
		t.Fatal("missed due retry after unlock")
	}
	// A failed attempt returns without an acquisition/timer. The run remains
	// active and spaces future attempts rather than retrying every frame.
	c.dispatch([]byte{20, 8})
	c.vacuumTick()
	if len(sent()) != 4 || !c.vacuum.started {
		t.Fatal("failed collection cancelled or spammed")
	}
	c.stopVacuum()
	c.clickNPC(c.World.NPCs[33])
	c.dispatch([]byte{20, 8})
	if !c.vacuum.started {
		t.Fatal("exhausted spot could not start a run")
	}
	if got := sent(); !bytes.Equal(got[len(got)-1], []byte{23, 73, 33}) {
		t.Fatal("missing native start", got)
	}
}
