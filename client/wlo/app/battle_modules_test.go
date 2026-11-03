package app

import (
	"bytes"
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
)

func TestBattleModulesOpenUnderNativeMovementAndEventLocks(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 12032, 10001))
	c.dispatch([]byte{20, 12})
	c.dispatch([]byte{6, 2, 1})
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 12032, Name: []byte("Robinson"), Amity: 60, Stats: world.Stats{Level: 1, HP: 75, MaxHP: 100, SP: 20, MaxSP: 40}}
	c.TeamState.BattlePet = 12032
	if c.Inventory.CanAct() || !c.canOpenModule() {
		t.Fatal("battle browsing shares the gameplay gate")
	}
	for _, tc := range []struct {
		button  int
		visible func() bool
	}{
		{mainInventoryButton, func() bool { return c.Inventory.Visible }},
		{mainSkillsButton, func() bool { return c.Skills.Visible }},
		{mainTeamButton, func() bool { return c.Team.Visible }},
		{mainSocialButton, func() bool { return c.Social.Visible }},
		{mainSettingsButton, func() bool { return c.Settings.Visible }},
	} {
		c.MainButtons.Buttons[tc.button].OnClick()
		if !tc.visible() {
			t.Fatalf("module %d stayed closed", tc.button)
		}
		if tc.button == mainTeamButton {
			if path := os.Getenv("BATTLE_TEAM_SNAPSHOT"); path != "" {
				c.Frame()
				savePNG(t, path, c)
			}
		}
		c.MainButtons.Buttons[tc.button].OnClick()
		if tc.visible() {
			t.Fatalf("module %d stayed open", tc.button)
		}
	}
	c.toggleCompound()
	if !c.Compound.Visible || c.Compound.CanAct() {
		t.Fatal("compound browsing or action gate")
	}
	c.toggleCompound()
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{33, 2}) {
		t.Fatal("opening modules sent commands beyond the Settings snapshot query", p)
	}
	c.battle.state.Active = false
	if c.canOpenModule() {
		t.Fatal("non-battle event lock bypassed")
	}
}

func TestBattleTeamSwitchUsesOnlyNativePetRequest(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[bundled], func(t *testing.T) {
			c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
			c.dispatch([]byte{20, 12})
			c.dispatch([]byte{6, 2, 1})
			c.InventoryState.Pets[0] = inventory.UsePet{ID: 14162, Amity: 60, Stats: world.Stats{HP: 100, MaxHP: 100}}
			c.InventoryState.Pets[1] = inventory.UsePet{ID: 12032, Amity: 60, Stats: world.Stats{HP: 75, MaxHP: 100}}
			c.TeamState.BattlePet = 14162
			if bundled {
				c.combat.enabled = true
				c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
				if err := c.sendCombatChoice([]byte{50, 1, 3, 2, 2, 2, 17, 39}); err != nil {
					t.Fatal(err)
				}
			}
			c.toggleTeam()
			c.Team.Refresh()
			if !c.Team.Modes[1].Enabled || c.Team.Dismiss[1].Enabled || c.Team.Names[1].Enabled {
				t.Fatal("pet switching shares unrelated Team permissions")
			}
			c.Team.SetMode(2, 2)
			c.Team.DismissPet(2)
			if len(sent()) != 0 || c.UI.Modal != nil {
				t.Fatal("battle allowed ride/dismiss")
			}
			c.Team.SetMode(2, 1)
			p := sent()
			if len(p) != 1 || !bytes.Equal(p[0], []byte{19, 1, 0, 47, 0, 0}) {
				t.Fatal("switch sent an action/defense", p)
			}
			if c.Team.Visible || c.battle.state.Ready || c.remote.battle.ready || c.combat.choiceToken != 0 || len(c.combat.choices) != 0 || c.TeamState.BattlePet != 14162 {
				t.Fatal("switch did not close command selection or changed roster optimistically")
			}
			c.toggleTeam()
			c.Team.SetMode(1, 0)
			if len(sent()) != 1 {
				t.Fatal("duplicate switch sent")
			}
			c.dispatch([]byte{19, 1, 0, 47, 0, 0})
			if c.TeamState.BattlePet != 12032 || c.remote.activePet != 12032 {
				t.Fatal("authoritative mode reply lost")
			}
			c.dispatch([]byte{50, 6, 4, 2, 0})
			c.dispatch([]byte{52, 1})
			if !c.canChangeBattlePet() {
				t.Fatal("next round did not restore pet selection")
			}
			c.hotbar.submitted = map[[2]byte]bool{{4, 2}: true}
			if c.canChangeBattlePet() {
				t.Fatal("pet change after player submitted")
			}
		})
	}
}

func TestBattleTimeoutClosesCommandsWithoutDefend(t *testing.T) {
	for _, bundled := range []bool{false, true} {
		t.Run(map[bool]string{false: "native", true: "chain"}[bundled], func(t *testing.T) {
			c, now, sent := enterBattle(t)
			if bundled {
				c.combat.enabled = true
				c.dispatch([]byte{254, 3, 9, 0, 0, 0, 4, 2})
			}
			c.battleFrame(false)
			*now = now.Add(22 * time.Second)
			c.battleTimerTick(*now)
			if !c.battle.timedOut || c.battle.state.Ready || c.remote.battle.ready || c.canChangeBattlePet() || c.combat.choiceToken != 0 {
				t.Fatal("timeout retained command selection")
			}
			c.chooseBattleCommand(battleCommandDefend)
			if len(sent()) != 0 {
				t.Fatal("timeout automatically defended or allowed a late command", sent())
			}
		})
	}
}
