package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/protocol"
)

func TestBattlePetSwitchConcurrentDepartureAndArrival(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native burst", true: "round chain"}[bundled], func(t *testing.T) {
			c, now, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
				useCompiledPetAssets(t, c, bundle)
			}
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
			c.battleFrame(false)
			cell := clientbattle.Cell{X: 3, Y: 2}
			attack := []byte{50, 1, 17, 0, 2, 2, 17, 39, 0, 1, 3, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}
			steps := []protocol.CombatStep{
				{Packets: [][]byte{{11, 1, 3, 2, 0}}},
				{Packets: [][]byte{remoteRecord(5, 4, 3, 2, 12032, 10001), {53, 5, 3, 2}}},
				{Packets: [][]byte{attack}},
			}
			if bundled {
				c.combat.enabled = true
				deliverCombatRound(t, c, 1, steps)
			} else {
				for _, step := range steps {
					for _, p := range step.Packets {
						c.dispatch(p)
					}
				}
			}
			c.battleFrame(false)
			old, exists := c.battle.petDepartures[cell]
			arrival := c.battle.arrivals[cell]
			if !exists || old.at.IsZero() || arrival.IsZero() || c.battle.state.At(cell).ID != 12032 || old.fighter.ID != 14162 || old.body == c.battleBody(*c.battle.state.At(cell)) || len(c.battle.playing) != 0 {
				t.Fatal("switch did not retain independent concurrent actors", old, arrival)
			}
			motion := battleArrivalMotion()
			duration := motion.Duration() - motion.ReturnStart()
			*now = arrival.Add(duration / 2)
			c.battleFrame(false)
			at := motion.PositionAt(cell, clientbattle.Cell{}, motion.ReturnStart()+duration/2)
			if at == cell.Position() || len(c.battle.petDepartures) != 1 || len(c.battle.playing) != 0 || c.battle.shown[cell].HP != 100 || motion.LiftAt(cell, clientbattle.Cell{}, motion.ReturnStart()+duration/2) != 0 {
				t.Fatal("entry did not walk while old pet exits, or damage appeared early")
			}
			if dir := os.Getenv("BATTLE_PET_SWITCH_SNAPSHOT"); dir != "" {
				c.battleFrame(true)
				c.Sprites.WaitNative()
				c.battleFrame(true)
				savePNG(t, filepath.Join(dir, map[bool]string{false: "native", true: "chain"}[bundled]+"-concurrent.png"), c)
			}
			// Entry can finish first; the outgoing actor must still complete its path.
			*now = old.at.Add(c.battleDepartureMotion().Duration())
			c.battleFrame(false)
			c.battleFrame(false)
			if len(c.battle.petDepartures) != 0 || len(c.battle.arrivals) != 0 || len(c.battle.playing) != 1 || c.battle.shown[cell].HP != 100 {
				t.Fatal("attacks did not wait for both switch tracks", len(c.battle.playing))
			}
			if len(sent()) != 0 {
				t.Fatal("cosmetic switch sent extra requests")
			}
		})
	}
}

func TestBattlePetLegacyWaitingModeAcrossTransports(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[bundled], func(t *testing.T) {
			c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			if bundle := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE"); bundle != "" {
				useCompiledPetAssets(t, c, bundle)
			}
			c.npcTemplates, _ = world.NPCTemplates(c.Assets)
			cell := clientbattle.Cell{X: 3, Y: 2}
			incoming := remoteRecord(5, 4, 3, 2, 12032, 10001)
			incoming[2], incoming[8] = 5, 2 // Captured native wait-next-turn mode and owned slot.
			attack := []byte{50, 1, 17, 0, 2, 2, 17, 39, 0, 1, 4, 2, 1, 0, 1, 25, 25, 0, 0, 0, 1}
			steps := []protocol.CombatStep{
				{Packets: [][]byte{{11, 1, 3, 2}, {19, 1, 0, 47, 0, 0}}},
				{Packets: [][]byte{incoming}, Delay: 350 * time.Millisecond},
				{Packets: [][]byte{attack}},
			}
			if bundled {
				c.combat.enabled = true
				deliverCombatRound(t, c, 1, steps)
			} else {
				for _, step := range steps {
					for _, p := range step.Packets {
						c.dispatch(p)
					}
				}
			}
			for i := 0; i < 80; i++ {
				c.battleFrame(false)
				*now = now.Add(100 * time.Millisecond)
			}
			pet := c.battle.state.At(cell)
			if pet.ID != 12032 || !pet.Waiting || !c.battle.petPresented(*pet) || pet.HP != 100 || c.battle.state.CanChoose(cell) || len(c.battle.arrivals) != 0 || len(c.battle.playing) != 0 || c.battle.state.At(clientbattle.Cell{X: 4, Y: 2}).HP != 75 {
				t.Fatal("visible waiting pet lost protection or blocked switching-round attacks", pet)
			}
			if dir := os.Getenv("BATTLE_PET_SWITCH_SNAPSHOT"); dir != "" {
				c.battleFrame(true)
				c.Sprites.WaitNative()
				c.battleFrame(true)
				savePNG(t, filepath.Join(dir, map[bool]string{false: "native", true: "chain"}[bundled]+"-waiting.png"), c)
			}
			// Readiness enables commands without replaying the completed cosmetic entry.
			if bundled {
				c.dispatch([]byte{254, 3, 2, 0, 0, 0, 4, 2})
			} else {
				c.dispatch([]byte{50, 6, 4, 2, 0})
				c.dispatch([]byte{52, 1})
			}
			c.battleFrame(false)
			if c.battle.state.At(cell).Waiting || len(c.battle.arrivals) != 0 {
				t.Fatal("next readiness replayed entry or failed to unlock commands")
			}
			if dir := os.Getenv("BATTLE_PET_SWITCH_SNAPSHOT"); dir != "" {
				c.battleFrame(true)
				c.Sprites.WaitNative()
				c.battleFrame(true)
				savePNG(t, filepath.Join(dir, map[bool]string{false: "native", true: "chain"}[bundled]+"-next-turn.png"), c)
			}
			*now = now.Add(time.Second)
			c.battleFrame(false)
			if len(c.battle.arrivals) != 0 || !c.battle.state.CanChoose(cell) {
				t.Fatal("next turn failed to enable replacement")
			}
		})
	}
}

func TestBattlePetSwitchQueuedSettlementSurvivesSceneClose(t *testing.T) {
	c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.battleFrame(false)
	for _, p := range [][]byte{
		{11, 1, 3, 2, 0}, remoteRecord(5, 4, 3, 2, 12032, 10001),
		{11, 12, 1}, {11, 0, 17, 39, 0, 0, 0, 0},
		{26, 4, 109, 0, 0, 0}, resultItemPacket(1, 300, 3),
	} {
		c.dispatch(p)
	}
	for i := 0; i < 10; i++ {
		c.battleFrame(false)
		*now = now.Add(time.Second)
	}
	if c.battle.state.Active || c.Stats.Gold != 109 || c.InventoryState.Bag[0].Count != 3 || len(c.battle.transitionPackets) != 0 {
		t.Fatal("queued settlement was discarded or duplicated", c.Stats.Gold, c.InventoryState.Bag[0])
	}
	c.battleFrame(false)
	if c.InventoryState.Bag[0].Count != 3 {
		t.Fatal("queued reward replayed")
	}
}

func TestBattlePetArrivalMirrorsAndResets(t *testing.T) {
	m := battleArrivalMotion()
	for _, cell := range []clientbattle.Cell{{X: 2, Y: 2}, {X: 3, Y: 2}} {
		start := m.PositionAt(cell, clientbattle.Cell{}, m.ReturnStart())
		end := m.PositionAt(cell, clientbattle.Cell{}, m.Duration())
		if end != cell.Position() || start == end || (start.X > end.X) != (cell.X > 2) {
			t.Fatal("entry did not mirror toward formation", cell, start, end)
		}
	}
	c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.battleFrame(false)
	c.dispatch([]byte{11, 1, 3, 2, 0})
	c.dispatch(remoteRecord(5, 4, 3, 2, 12032, 10001))
	if len(c.battle.petDepartures) != 1 || len(c.battle.arrivals) != 1 {
		t.Fatal("replacement did not start concurrent visual tracks")
	}
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	finishSceneFade(c, now)
	if c.battle.petTransition || len(c.battle.arrivals) != 0 || len(c.battle.transitionPackets) != 0 || len(c.battle.departures) != 0 || len(c.battle.petDepartures) != 0 || len(c.battle.presentedPets) != 0 {
		t.Fatal("old switch leaked into new battle")
	}
	*now = now.Add(time.Second)
	c.battleFrame(false)
}
