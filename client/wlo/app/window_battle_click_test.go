package app

import (
	"bytes"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/seui"
)

func TestBattleRapidTargetClicksChoosePlayerAndPet(t *testing.T) {
	for _, chain := range []bool{false, true} {
		name := "legacy"
		if chain {
			name = "round-chain"
		}
		t.Run(name, func(t *testing.T) {
			c, now, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			c.InventoryState.Pets[0].ID = 14162
			if chain {
				c.combat.enabled = true
				c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
			}
			g := &Game{C: c}
			c.Input.Hovered = nil
			g.leftPress(shiftLeft, 230, 370)
			c.UI.MouseUp(seui.ButtonLeft, 0, 230, 370)
			*now = now.Add(50 * time.Millisecond)
			g.leftPress(shiftLeft, 230, 370)
			c.UI.MouseUp(seui.ButtonLeft, 0, 230, 370)

			want := [][]byte{
				{50, 1, 4, 2, 2, 2, 17, 39},
				{50, 1, 3, 2, 2, 2, 17, 39},
			}
			if chain {
				want = [][]byte{{254, 2, 9, 0, 0, 0, 2, 50, 1, 4, 2, 2, 2, 17, 39, 50, 1, 3, 2, 2, 2, 17, 39}}
			}
			got := sent()
			if len(got) != len(want) {
				t.Fatalf("rapid clicks sent %v; want %v", got, want)
			}
			for i := range want {
				if !bytes.Equal(got[i], want[i]) {
					t.Fatalf("request %d = %v; want %v", i, got[i], want[i])
				}
			}
			*now = now.Add(50 * time.Millisecond)
			g.leftPress(shiftLeft, 230, 370)
			if len(sent()) != len(want) {
				t.Fatal("extra click submitted a duplicate action")
			}
		})
	}
}

func TestBattleDoubleClickOnControlDoesNotChooseTarget(t *testing.T) {
	c, now, sent := enterBattle(t)
	g := &Game{C: c}
	control := seui.NewGrBasic(c.Env, nil)
	doubles := 0
	control.OnDblTag = func(int) { doubles++ }
	c.Input.Hovered = control
	g.leftPress(shiftLeft, 230, 370)
	*now = now.Add(50 * time.Millisecond)
	g.leftPress(shiftLeft, 230, 370)
	if doubles != 1 || len(sent()) != 0 {
		t.Fatalf("UI double click count %d, battle requests %v", doubles, sent())
	}
}
