package app

import (
	"testing"

	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/game"
)

func TestClientFixtureResourcesRetainSessionIsolation(t *testing.T) {
	fixture := newClientFixture(t)
	var first *Client
	t.Run("first", func(t *testing.T) {
		first = testClient(t, fixture)
		first.InventoryState.Bag[0] = game.Item{ID: 32003, Count: 4}
		first.SkillState.Learned[11016] = skills.Progress{Grade: 1}
		first.HotKeys.Bindings[1][1] = hud.Binding{Kind: 2, ID: 10001}
		first.Input.X = 17
	})
	// The first case's network cleanup must leave the group's assets usable.
	t.Run("second", func(t *testing.T) {
		second := testClient(t, fixture)
		if first.Pics != second.Pics || first.Sprites != second.Sprites || first.resources != second.resources {
			t.Fatal("group duplicated decoded assets")
		}
		if first.UI == second.UI || first.Input == second.Input || first.Net == second.Net || first.Screen == second.Screen {
			t.Fatal("group shared mutable session state")
		}
		if second.InventoryState.Bag[0].ID != 0 || second.SkillState.Learned[11016].Grade != 0 || second.HotKeys.Bindings[1][1].ID != 0 || second.Input.X == 17 {
			t.Fatal("case state leaked to its sibling")
		}
		if first.settingsPath == second.settingsPath || first.Assets.UserPath("save.dat") == second.Assets.UserPath("save.dat") {
			t.Fatal("case writable profiles shared")
		}
		second.Sprites.WaitNative()
	})
}
