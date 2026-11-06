package app

import (
	"bytes"
	"os"
	"testing"
	"time"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/hud"
)

func overkillClient(t *testing.T, participants ...[]byte) (*Client, *time.Time) {
	t.Helper()
	c, now, _ := enterBattle(t, participants...)
	c.battleFrame(false)
	c.battle.assets = &clientbattle.Assets{Motions: map[uint16]clientbattle.Motion{
		10001: {Events: []clientbattle.MotionEvent{{Trigger: 150, Percent: 100}}},
		50001: {Events: []clientbattle.MotionEvent{{Trigger: 1000, Percent: 100}}},
	}}
	return c, now
}

func TestBattleOrdinaryKnockoutRemainsOnField(t *testing.T) {
	c, now := overkillClient(t)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 100, 0, 0, 0, 1})
	c.battleFrame(false)
	*now = now.Add(time.Second)
	c.battleFrame(false)
	f := c.battle.shown[clientbattle.Cell{X: 2, Y: 2}]
	if f.HP != 0 || f.Removed || f.Overkilled || len(c.battle.overkills) != 0 {
		t.Fatal("ordinary knockout vanished", f)
	}
}

func TestBattleOverkillFliesAfterContactBeforeRoundEnds(t *testing.T) {
	c, now := overkillClient(t)
	c.dispatch(remoteRecord(5, 7, 2, 3, 20002, 0))
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1})
	// A different fighter's longer action must not hold up this removal.
	c.dispatch([]byte{50, 1, 17, 0, 2, 3, 81, 195, 0, 1, 4, 2, 1, 0, 1, 25, 1, 0, 0, 0, 1})
	c.dispatch([]byte{53, 3, 2, 2})
	c.battleFrame(false)
	cell := clientbattle.Cell{X: 2, Y: 2}
	if !c.battle.overkills[cell].IsZero() || c.battle.shown[cell].Removed || c.battle.shown[cell].HP != 100 {
		t.Fatal("overkill skipped contact")
	}
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	at := c.battle.overkills[cell]
	if at.IsZero() || c.battle.shown[cell].HP != 0 || c.battle.shown[cell].Removed {
		t.Fatal("flight failed to start at contact")
	}
	*now = at.Add(299 * time.Millisecond)
	c.battleFrame(true)
	if c.battle.shown[cell].Removed {
		t.Fatal("flight ended early")
	}
	*now = at.Add(300 * time.Millisecond)
	c.battleFrame(false)
	if !c.battle.shown[cell].Removed || !c.battle.state.Active || len(c.battle.playing) == 0 {
		t.Fatal("overkill waited for the round to finish")
	}
	c.dispatch([]byte{53, 3, 2, 2})
	c.battleFrame(false)
	if len(c.battle.overkills) != 0 || !c.battle.shown[cell].Removed {
		t.Fatal("duplicate overkill replayed")
	}
}

func TestBattleOverkillCannotRemoveReplacement(t *testing.T) {
	c, now := overkillClient(t)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1})
	c.dispatch([]byte{53, 3, 2, 2})
	c.battleFrame(false)
	// A reinforcement may reuse the exact same template and grid cell.
	p := remoteRecord(5, 7, 2, 2, 20001, 0)
	p[2] = 2
	c.dispatch(p)
	*now = now.Add(time.Second)
	c.battleFrame(false)
	f := c.battle.shown[clientbattle.Cell{X: 2, Y: 2}]
	if f.HP != 100 || f.Removed || f.Overkilled || len(c.battle.overkills) != 0 {
		t.Fatal("old damage/removal reached replacement", f)
	}
}

func TestBattleOverkillFinishesBeforeSceneClose(t *testing.T) {
	c, now := overkillClient(t)
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1})
	c.dispatch([]byte{53, 3, 2, 2})
	c.dispatch([]byte{11, 0, 17, 39, 0, 0, 0, 0})
	c.battleFrame(false)
	*now = now.Add(150 * time.Millisecond)
	c.battleFrame(false)
	if !c.battle.closing || c.sceneFade.mode != sceneFadeNone {
		t.Fatal("close skipped overkill")
	}
	*now = now.Add(300 * time.Millisecond)
	if c.battleFrame(false) || c.sceneFade.mode != sceneFadeLeaveBattle || c.battle.state.Active {
		t.Fatal("flight did not release scene")
	}
}

func TestBattleOverkillCannotBeRevivalTarget(t *testing.T) {
	c, _ := overkillClient(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	c.dispatch([]byte{51, 1, 3, 2, 25, 0, 0, 0, 0})
	var actor, target remoteFighter
	for _, f := range c.remote.battle.fighters {
		if f.x == 4 && f.y == 2 {
			actor = f
		}
		if f.x == 3 && f.y == 2 {
			target = f
		}
	}
	binding := hud.Binding{Kind: 2, ID: 11053}
	if !c.hotbarTargetAllowed(binding, actor, target) {
		t.Fatal("ordinary knockout cannot be revived")
	}
	c.dispatch([]byte{53, 3, 3, 2})
	if c.hotbarTargetAllowed(binding, actor, target) {
		t.Fatal("overkilled unit remains a revival target")
	}
}

func TestBattleOverkillNativeFlightCapture(t *testing.T) {
	path := os.Getenv("BATTLE_OVERKILL_SNAPSHOT")
	if path == "" {
		t.Skip("set BATTLE_OVERKILL_SNAPSHOT")
	}
	c, now, _ := enterBattle(t)
	c.Frame()
	c.dispatch([]byte{50, 1, 17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 151, 0, 0, 0, 1})
	c.dispatch([]byte{53, 3, 2, 2})
	c.Frame()
	m := c.battle.assets.Motions[10001]
	*now = now.Add(m.EventAt(m.Events[0]))
	c.Frame()
	*now = now.Add(150 * time.Millisecond)
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame()
	savePNG(t, path, c)
}

func TestBattleOverkillDoesNotBlockLivingTargetHitbox(t *testing.T) {
	c, now := overkillClient(t)
	sent := wire(t, c)
	p := remoteRecord(5, 7, 2, 3, 20002, 0)
	p[2] = 2
	c.dispatch(p)
	c.dispatch([]byte{53, 3, 2, 2})
	c.battleFrame(false)
	*now = now.Add(300 * time.Millisecond)
	c.battleFrame(false)
	c.chooseBattleCommand(battleCommandAttack)
	c.GroundClick(220, 385) // Overlapping hit boxes of rows 2 and 3.
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{50, 1, 4, 2, 2, 3, 17, 39}) {
		t.Fatalf("vanished unit blocked living target: %v", got)
	}
}
