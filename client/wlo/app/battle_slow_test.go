package app

import (
	"image"
	"os"
	"testing"
	"time"

	clientbattle "wonderland-gonline/client/wlo/battle"
)

func TestBattleSlowNativeArtwork(t *testing.T) {
	c, now, _ := enterBattle(t)
	c.battleFrame(true)
	*now = now.Add(10 * time.Second)
	c.battleFrame(true)
	i := c.Pics.Find("E119")
	w, h := c.Pics.Size(i)
	if w != 32 || h != 128 {
		t.Fatal("native turtle artwork missing", w, h)
	}
	f := *c.battle.state.At(clientbattle.Cell{X: 4, Y: 2})
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.drawBattleSlow(f, &battleBody{height: 80}, f.Position(), *now)
	visible := false
	for _, pixel := range c.Screen.Pix {
		visible = visible || pixel != 0
	}
	if !visible {
		t.Fatal("native turtle was not drawn")
	}
	if path := os.Getenv("BATTLE_SLOW_SNAPSHOT"); path != "" {
		c.battleFrame(true)
		savePNG(t, path, c)
	}
}

func TestBattleSlowNativeCountdownAndReceipts(t *testing.T) {
	for _, kind := range []byte{0, 2} {
		c, now, _ := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001), remoteRecord(5, 2, 4, 3, 10002, 0))
		c.battle.state.Kind = kind
		c.battleTimerTick(*now)
		start := *now
		f := *c.battle.state.At(clientbattle.Cell{X: 4, Y: 3})
		threshold := time.Duration(c.battle.state.TurnLimit()-10) * time.Second
		if c.battleSlowFrame(f, start.Add(threshold-time.Millisecond)) != -1 || c.battleSlowFrame(f, start.Add(threshold)) != 0 || c.battleSlowFrame(f, start.Add(threshold+250*time.Millisecond)) != 1 {
			t.Fatal("native turtle threshold/animation", kind)
		}
		c.dispatch([]byte{53, 5, 4, 2})
		// Our choice must not hide a teammate's turtle.
		if c.battleSlowFrame(f, start.Add(threshold)) < 0 {
			t.Fatal("own submission hid teammate turtle")
		}
		c.dispatch([]byte{53, 5, 4, 3})
		if c.battleSlowFrame(*c.battle.state.At(f.Cell), start.Add(threshold)) != -1 {
			t.Fatal("submitted teammate still has turtle")
		}
		pet := *c.battle.state.At(clientbattle.Cell{X: 3, Y: 2})
		if c.battleSlowFrame(pet, start.Add(threshold)) < 0 {
			t.Fatal("unsubmitted pet missing turtle")
		}
		for _, invalid := range []string{"dead", "waiting", "removed", "submitted", "monster", "playback", "expired"} {
			copy := pet
			at := start.Add(threshold)
			switch invalid {
			case "dead":
				copy.HP = 0
			case "waiting":
				copy.Waiting = true
			case "removed":
				copy.Removed = true
			case "submitted":
				copy.Submitted = true
			case "monster":
				copy.Kind = clientbattle.Monster
			case "playback":
				c.battle.state.Actions = []clientbattle.Action{{Actor: pet.Cell}}
			case "expired":
				at = start.Add(time.Duration(c.battle.state.TurnLimit()+1) * time.Second)
			}
			if c.battleSlowFrame(copy, at) != -1 {
				t.Fatal("ineligible turtle", invalid)
			}
			c.battle.state.Actions = nil
		}
		c.dispatch([]byte{50, 6, 4, 2, 0})
		c.dispatch([]byte{52, 1})
		*now = start.Add(threshold)
		c.battleTimerTick(*now)
		if c.battleSlowFrame(*c.battle.state.At(pet.Cell), *now) != -1 {
			t.Fatal("turtle survived next round")
		}
	}
}
