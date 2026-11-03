package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
)

func TestBattleVitalsAtContact(t *testing.T) {
	for _, test := range []struct {
		name                string
		cell                clientbattle.Cell
		stat, mode          byte
		start, amount, want byte
	}{
		{"damage", clientbattle.Cell{X: 2, Y: 2}, 25, 1, 100, 25, 75},
		{"critical", clientbattle.Cell{X: 4, Y: 2}, 25, 2, 100, 25, 75},
		{"healing", clientbattle.Cell{X: 4, Y: 2}, 25, 3, 40, 25, 65},
		{"SP", clientbattle.Cell{X: 3, Y: 2}, 26, 1, 20, 7, 13},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 12178, 10001))
			c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Stats: world.Stats{HP: 100, MaxHP: 100, SP: 20, MaxSP: 20}}
			c.TeamState.BattlePet = 12032
			c.dispatch([]byte{51, 1, test.cell.X, test.cell.Y, test.stat, test.start, 0, 0, 0})
			c.battleFrame(false)
			c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10001: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
			start := *now
			// Independent native packet layout: a single landed HP/SP result.
			c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, test.cell.X, test.cell.Y, 1, 0, 1, test.stat, test.amount, 0, 0, 0, test.mode})
			// A following absolute sync must not bypass the pending hit cue.
			c.dispatch([]byte{51, 1, test.cell.X, test.cell.Y, test.stat, test.want, 0, 0, 0})
			read := func() uint32 {
				f := c.battle.displayed(*c.battle.state.At(test.cell))
				if test.stat == 26 {
					return uint32(f.SP)
				}
				return f.HP
			}
			c.battleFrame(false)
			*now = start.Add(149 * time.Millisecond)
			c.battleFrame(false)
			if got := read(); got != uint32(test.start) {
				t.Fatalf("before contact: %d", got)
			}
			*now = start.Add(150 * time.Millisecond)
			c.battleFrame(false)
			if got := read(); got != uint32(test.want) {
				t.Fatalf("at contact: %d", got)
			}
			if test.cell.X == 4 && c.MainStatus.Stats.HP != uint32(test.want) {
				t.Fatal("player HUD missed contact")
			}
			if test.cell.X == 3 {
				pet, ok := c.MainStatus.Pet.Read()
				if !ok || pet.Stats.SP != uint16(test.want) {
					t.Fatal("pet HUD missed contact", pet)
				}
			}
			c.battleFrame(false)
			if got := read(); got != uint32(test.want) {
				t.Fatalf("duplicate cue: %d", got)
			}
		})
	}
}

func TestBattleMultiHitDefeatAtFinalContact(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10001: {
		DamageMode: 2, Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 33}, {Trigger: 300, Percent: 33}, {Trigger: 450, Percent: 34}},
	}}}
	start := *now
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1})
	c.dispatch([]byte{53, 1, 2, 2})
	c.battleFrame(false)
	for i, want := range []uint32{67, 34, 0} {
		*now = start.Add(time.Duration(i+1) * 150 * time.Millisecond)
		c.battleFrame(false)
		if hp := c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP; hp != want {
			t.Fatalf("hit %d HP %d, want %d", i+1, hp, want)
		}
	}
}

func TestBattleCaptureAndMissAtContact(t *testing.T) {
	for _, capture := range []bool{false, true} {
		c, now, _ := enterBattle(t)
		c.battleFrame(false)
		c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{10008: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}}}}
		result := byte(0) // Native miss.
		if capture {
			result = 1
		}
		c.dispatch([]byte{50, 1, 17, 0, 4, 2, 24, 39, 0, 1, 2, 2, result, 0, 1, 0, 0, 0, 0, 0, 1})
		c.battleFrame(false)
		cell := clientbattle.Cell{X: 2, Y: 2}
		if c.battle.shown[cell].HP != 100 || c.battle.shown[cell].Removed {
			t.Fatal("capture removed target before contact")
		}
		*now = now.Add(150 * time.Millisecond)
		c.battleFrame(false)
		if c.battle.shown[cell].Removed != capture {
			t.Fatal("capture/miss result not shown at contact")
		}
		if !capture && c.battle.shown[cell].HP != 100 {
			t.Fatal("miss changed HP")
		}
	}
}

func TestBattleNewFormationCancelsDeparture(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(false)
	c.dispatch([]byte{11, 1, 4, 2, 0})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	c.battleFrame(false)
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	finishSceneFade(c, now)
	c.battleFrame(false)
	if c.battle.closing || len(c.battle.departures) != 0 || c.battle.shown[clientbattle.Cell{X: 4, Y: 2}].Removed {
		t.Fatal("new battle retained departure")
	}
}

func TestBattleDepartureWaitsForFinalAttack(t *testing.T) {
	for _, victory := range []bool{false, true} {
		name := "flee"
		if victory {
			name = "victory"
		}
		t.Run(name, func(t *testing.T) {
			c, now, sent := enterBattle(t)
			c.battleFrame(false)
			start := *now
			if victory {
				c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1})
				c.dispatch([]byte{53, 1, 2, 2})
				c.dispatch([]byte{11, 12, 1})
			}
			c.dispatch([]byte{11, 1, 4, 2, 0})
			c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
			if !c.battle.closing || c.remote.battle.active || c.battle.state.CanChoose(clientbattle.Cell{X: 4, Y: 2}) {
				t.Fatal("exit failed to disable commands")
			}
			c.battleFrame(false)
			cell := clientbattle.Cell{X: 4, Y: 2}
			if c.battle.shown[cell].Removed || c.sceneFade.mode != sceneFadeNone || c.battle.timerOn {
				t.Fatal("departure skipped")
			}
			if victory {
				if !c.battle.departures[cell].IsZero() || c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 100 {
					t.Fatal("last attack skipped")
				}
				*now = start.Add(time.Minute)
				c.battleFrame(false)
				if c.battle.shown[clientbattle.Cell{X: 2, Y: 2}].HP != 0 {
					t.Fatal("final blow lost")
				}
			}
			at := c.battle.departures[cell]
			if at.IsZero() {
				t.Fatal("departure never started")
			}
			motion := c.battleDepartureMotion()
			*now = at.Add(motion.Duration() / 4)
			c.battleFrame(true)
			if dir := os.Getenv("BATTLE_DEPARTURE_SNAPSHOT"); dir != "" {
				c.Sprites.WaitNative()
				c.battleFrame(true)
				savePNG(t, filepath.Join(dir, name+".png"), c)
			}
			if !c.battle.state.Active || c.sceneFade.mode != sceneFadeNone || motion.PositionAt(cell, clientbattle.Cell{}, now.Sub(at)).X <= cell.Position().X {
				t.Fatal("outward motion missing")
			}
			c.WalkKeys(true, false, false, false)
			c.chooseBattleCommand(battleCommandAttack)
			if c.World.Walking() || len(sent()) != 0 {
				t.Fatal("exit allowed input")
			}
			*now = at.Add(motion.Duration())
			if c.battleFrame(false) || c.battle.state.Active || c.battle.closing || c.sceneFade.mode != sceneFadeLeaveBattle || c.MainStatus.Stats != c.Stats {
				t.Fatal("departure did not finish into map fade")
			}
		})
	}
}
