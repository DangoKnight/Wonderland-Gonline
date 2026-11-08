package app

import (
	"testing"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func TestRemoteAndBattleAutoStopIndependently(t *testing.T) {
	for _, stopRemoteFirst := range []bool{false, true} {
		name := "auto_first"
		if stopRemoteFirst {
			name = "remote_first"
		}
		t.Run(name, func(t *testing.T) {
			c, _, sent := enterBattle(t)
			c.Stats.HP, c.Stats.MaxHP = 100, 100
			c.InventoryState.Bag[0] = game.Item{ID: 34058, Count: 1}
			o := inventory.RemoteOptions{AutoFight: true, SupplyFirst: 1, SupplyLast: 50}
			if !c.startRemote(o) {
				t.Fatal("Remote could not start in combat")
			}
			if !c.battle.auto || c.battle.autoForm == nil || !c.battle.autoForm.Visible {
				t.Fatal("Remote did not enable the battle AUTO indicator")
			}
			cancel := c.battle.autoForm.Children[0].(*seui.FixedButton)
			if stopRemoteFirst {
				c.stopRemote()
				if !c.battle.auto {
					t.Fatal("stopping Remote disabled AUTO")
				}
				c.battleAutoTick()
				if len(sent()) != 1 {
					t.Fatal("AUTO did not take over")
				}
				cancel.OnClick()
			} else {
				c.battleAutoTick()
				if len(sent()) != 0 {
					t.Fatal("AUTO competed with Remote for the same turn")
				}
				cancel.OnClick()
				if !c.remote.active || !c.remote.options.AutoFight {
					t.Fatal("AUTO cancellation disabled Remote")
				}
				c.remoteTick()
				if len(sent()) != 1 {
					t.Fatal("Remote stopped submitting when AUTO was cancelled")
				}
				if !c.battle.auto {
					t.Fatal("Remote did not re-enable AUTO for its decision")
				}
				c.stopRemote()
				cancel.OnClick()
			}
			// Already submitted commands remain authoritative; manual control
			// resumes at the next action-selection phase.
			c.dispatch([]byte{50, 6, 4, 2, 0})
			c.dispatch([]byte{52, 1})
			c.battleFrame(false)
			if c.remote.active || c.battle.auto || c.battle.autoForm.Visible || c.battle.menu == nil || !c.battle.menu.Visible {
				t.Fatal("both stopped but manual controls unavailable")
			}
			if len(sent()) != 1 {
				t.Fatal("automatic command after both controls stopped")
			}
		})
	}
}

func TestRemoteRearmsAutoAtDecisionStage(t *testing.T) {
	for _, alreadyDeciding := range []bool{false, true} {
		name := "next_decision"
		if alreadyDeciding {
			name = "current_decision"
		}
		t.Run(name, func(t *testing.T) {
			c, _, sent := enterBattle(t)
			c.remote.active, c.remote.options.AutoFight = true, true
			c.battle.auto = true
			c.battleAutoCancel()
			cancel := c.battle.autoForm.Children[0].(*seui.FixedButton)
			if !alreadyDeciding {
				c.battle.state.Ready, c.remote.battle.ready = false, false
			}
			cancel.OnClick()
			c.battleAutoTick()
			if !alreadyDeciding {
				if c.battle.auto || c.battle.autoForm.Visible {
					t.Fatal("AUTO re-enabled outside decision stage")
				}
				c.dispatch([]byte{50, 6, 4, 2, 0})
				c.dispatch([]byte{52, 1})
				c.battleAutoTick()
			}
			if !c.battle.auto || !c.battle.autoForm.Visible || !c.remote.active {
				t.Fatal("Remote did not re-arm AUTO at decision stage")
			}
			if len(sent()) != 0 {
				t.Fatal("re-arming competed with Remote submissions")
			}
			c.stopRemote()
			cancel.OnClick()
			c.battleAutoTick()
			if c.battle.auto || c.battle.autoForm.Visible {
				t.Fatal("stopped Remote re-armed AUTO")
			}
		})
	}
}

func TestRemoteEnablesAutoOnBattleEntry(t *testing.T) {
	c, _, o := remoteClient(t)
	o.AutoFight = true
	if !c.startRemote(o) {
		t.Fatal("Remote start")
	}
	if c.battle.auto {
		t.Fatal("AUTO enabled outside combat")
	}
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	if !c.battle.auto || c.battle.autoForm == nil || !c.battle.autoForm.Visible {
		t.Fatal("battle entry did not show AUTO")
	}
	c.battle.autoForm.Children[0].(*seui.FixedButton).OnClick()
	c.dispatch(remoteRecord(5, 7, 2, 2, 20001, 0))
	if c.battle.auto {
		t.Fatal("roster updates re-enabled cancelled AUTO")
	}
}
