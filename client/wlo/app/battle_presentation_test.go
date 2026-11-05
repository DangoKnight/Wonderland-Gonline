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
		if c.battle.shown[cell].Removed {
			t.Fatal("capture removed target before its motion finished")
		}
		if !capture && c.battle.shown[cell].HP != 100 {
			t.Fatal("miss changed HP")
		}
		*now = now.Add(2 * time.Second)
		c.battleFrame(false)
		if c.battle.shown[cell].Removed != capture {
			t.Fatal("capture/miss final visibility")
		}
	}
}

func TestBananaCaptureRemainsVisibleDuringNativeDrag(t *testing.T) {
	enemy := remoteRecord(5, 7, 2, 2, 17000, 0) // Extracted Banana Monster template.
	enemy[2] = 2
	c, now, _ := enterBattle(t, enemy)
	c.battleFrame(false)
	m := c.battle.assets.Motions[battleCatchSkill]
	if m.Reaction != 60001 {
		t.Fatal("native capture reaction changed", m.Reaction)
	}
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 24, 39, 0, 1, 2, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1})
	c.battleFrame(false)
	start := *now
	*now = start.Add(m.EventAt(clientbattle.MotionEvent{Trigger: capturePullFrameTrigger}) + 600*time.Millisecond)
	c.battleFrame(true)
	cell := clientbattle.Cell{X: 2, Y: 2}
	if c.battle.shown[cell].Removed || !c.battle.state.At(cell).Removed {
		t.Fatal("capture visibility/authority separated incorrectly")
	}
	found := false
	for _, p := range c.battle.playing {
		if !p.captureRemoval {
			continue
		}
		found = true
		if p.captureDrag == nil {
			t.Fatal("drag not attached to capturer")
		}
		pull := p.captureDrag.pullStart()
		for _, at := range []time.Time{start, start.Add(pull.Sub(start) / 2), pull} {
			if p.captureDrag.position(cell, at) != cell.Position() {
				t.Fatal("monster moved before capturer reached it")
			}
		}
		if !p.at.Equal(pull) {
			t.Fatal("capture reaction starts before physical contact")
		}
		contact := p.captureDrag.capturerPosition(pull)
		spacing := cell.Position().Sub(contact)
		if spacing.X < -40 || spacing.X > 40 || spacing.Y < -40 || spacing.Y > 40 {
			t.Fatalf("capturer did not reach monster before pull: spacing %v", spacing)
		}
		for _, elapsed := range []time.Duration{100 * time.Millisecond, time.Second, 2 * time.Second} {
			at := pull.Add(elapsed)
			capturer := p.captureDrag.capturerPosition(at)
			if p.captureDrag.position(cell, at).Sub(capturer) != spacing {
				t.Fatal("captured monster fell behind capturer")
			}
		}
		reaction := c.battle.assets.Motions[p.action.Skill].Departure()
		elapsed := now.Sub(p.at)
		if reaction.PositionAt(cell, clientbattle.Cell{}, elapsed) == cell.Position() {
			t.Fatal("target not dragged")
		}
		body := c.battleBody(c.battle.shown[cell])
		action, frame := reaction.PoseAt(cell, clientbattle.Cell{}, elapsed, body.moveKind)
		if body.npc == nil {
			t.Fatal("missing Banana sprite")
		}
		body.npc.Hold(frame, false)
		if w, h := body.npc.FrameSize(action); w == 0 || h == 0 {
			t.Fatalf("missing drag pose %d", action)
		}
	}
	if !found {
		t.Fatal("missing capture drag track")
	}
	*now = start.Add(20 * time.Second)
	c.battleFrame(false)
	if !c.battle.shown[cell].Removed {
		t.Fatal("capture target not removed after drag")
	}
}

func TestBattleCaptureFailureSharesPullAndReturnsMonster(t *testing.T) {
	enemy := remoteRecord(5, 7, 2, 2, 17000, 0)
	enemy[2] = 2
	c, now, _ := enterBattle(t, enemy)
	c.battleFrame(false)
	start := *now
	// Independent wire fixture: 10009 is failed capture, with a landed reaction.
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 25, 39, 0, 1, 2, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1})
	c.battleFrame(false)
	var drag *battleCaptureDrag
	for _, p := range c.battle.playing {
		if p.captureDrag != nil {
			drag = p.captureDrag
			if p.captureRemoval {
				t.Fatal("failed capture removes monster")
			}
		}
	}
	if drag == nil {
		t.Fatal("failed capture uses independent reaction path")
	}
	pull := drag.pullStart()
	contact := drag.capturerPosition(pull)
	spacing := drag.target.Position().Sub(contact)
	facing := drag.target.Position().Sub(drag.actor.Position())
	if spacing.X*facing.X+spacing.Y*facing.Y <= 0 {
		t.Fatal("monster trails capturer instead of leading")
	}
	if spacing.X < -40 || spacing.X > 40 {
		t.Fatalf("distant contact: %v", spacing)
	}
	for _, elapsed := range []time.Duration{0, 500 * time.Millisecond, time.Second, 2 * time.Second} {
		at := pull.Add(elapsed)
		*now = at
		c.battleFrame(true)
		actor := drag.capturerPosition(at)
		if drag.position(drag.target, at).Sub(actor) != spacing {
			t.Fatal("failed capture pull diverged")
		}
		if c.battle.shown[drag.target].Removed {
			t.Fatal("monster hidden during failed capture")
		}
	}
	end := start.Add(drag.motion.ReturnStart() + drag.returnDuration)
	if drag.position(drag.target, end) != drag.target.Position() {
		t.Fatal("monster did not return after failed capture")
	}
	*now = end.Add(time.Second)
	c.battleFrame(true)
	if c.battle.shown[drag.target].Removed || c.battle.state.At(drag.target).Removed {
		t.Fatal("failed capture removed monster")
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
