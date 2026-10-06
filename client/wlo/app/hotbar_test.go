package app

import (
	"bytes"
	"os"
	"testing"
	"wonderland-gonline/client/wlo/hud"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
)

func TestHotbarDragPagesPersistenceAndRemoval(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.Skills.Show()
	a := c.Skills.List.Abs()
	c.Input.X, c.Input.Y = a.X+75, a.Y+10
	c.Frame()
	if c.Input.Hovered != c.Skills.List {
		t.Fatal("skill list hover")
	}
	c.UI.MouseDown(seui.ButtonLeft, 0, c.Input.X, c.Input.Y)
	if c.hotbar.drag == nil || c.hotbar.drag.binding.ID != 10001 {
		t.Fatal("Basic Attack did not begin dragging")
	}
	target := c.HotKeys.Slots[1].Abs()
	c.Input.X, c.Input.Y = target.X+10, target.Y+10
	c.Frame()
	if !c.dropHotbar(c.Input.X, c.Input.Y) {
		t.Fatal("drop not consumed")
	}
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{40, 1, 2, 0x11, 0x27, 1, 1}) {
		t.Fatalf("native assignment %x", got)
	}
	if c.HotKeys.Slots[1].Image < 0 {
		t.Fatal("missing mounted icon")
	}
	saved, err := hud.LoadBindings(c.hotbar.path)
	if err != nil || saved[1][1].ID != 10001 {
		t.Fatal("binding not saved", err)
	}
	c.loadHotbar()
	if c.HotKeys.Bindings[1][1].ID != 10001 {
		t.Fatal("re-entry lost binding")
	}
	c.Skills.Hide()
	if out := os.Getenv("HOTBAR_SNAPSHOT"); out != "" {
		c.Frame()
		savePNG(t, out+".vertical.png", c)
	}
	inventoryClick(t, c, c.HotKeys.Next)
	if c.HotKeys.Page != 2 || c.HotKeys.Slots[1].Image != -1 {
		t.Fatal("page failed to refresh")
	}
	c.assignBinding(2, 8, hud.Binding{Kind: 2, ID: 60021})
	inventoryClick(t, c, c.HotKeys.Minimize)
	if !c.HotKeys.Horizontal || c.HotKeys.Width != 274 || c.HotKeys.Slots[8].Image < 0 {
		t.Fatal("horizontal layout lost binding")
	}
	c.HotKeys.Clear(8)
	if c.HotKeys.Bindings[2][8].ID != 0 || c.HotKeys.Slots[8].Image != -1 {
		t.Fatal("clear retained shortcut")
	}
	saved, err = hud.LoadBindings(c.hotbar.path)
	if err != nil || saved[2][8].ID != 0 || saved[1][1].ID != 10001 {
		t.Fatal("clear lost other page", err)
	}
	if out := os.Getenv("HOTBAR_SNAPSHOT"); out != "" {
		c.Frame()
		savePNG(t, out+".horizontal.png", c)
	}
}
func TestHotbarNativeListsAtomicAndCharacterIsolation(t *testing.T) {
	c, _, _ := enteredClient(t)
	wire(t, c)
	c.dispatch([]byte{40, 1, 2, 0x11, 0x27, 1, 1, 2, 0x75, 0xea, 3, 8})
	if c.HotKeys.Bindings[1][1].ID != 10001 || c.HotKeys.Bindings[3][8].ID != 60021 {
		t.Fatal("native list")
	}
	before := c.HotKeys.Bindings
	c.dispatch([]byte{40, 1, 2, 0x08, 0x2b, 1, 1, 2, 0x75, 0xea, 4, 8})
	c.dispatch([]byte{40, 1, 1, 0, 0}) // Legacy alchemy failure is not a binding list.
	if c.HotKeys.Bindings != before {
		t.Fatal("malformed list partially changed bindings")
	}
	oldPath := c.hotbar.path
	c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
	if c.HotKeys.Bindings[1][1].ID != 0 || c.hotbar.path == oldPath {
		t.Fatal("bindings leaked between characters")
	}
}
func TestHotbarBattleActionsGoldenAndStaleGuards(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.HotKeys.Bindings[1][1] = hud.Binding{Kind: 2, ID: 10001}
	c.HotKeys.Bindings[1][2] = hud.Binding{Kind: 2, ID: 60021}
	player := remoteRecord(250, 2, 4, 2, 10001, 0)
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	for _, p := range [][]byte{player, enemy, {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	if !c.HotbarKey(0x70, 0) || c.hotbar.target == nil || len(sent()) != 0 {
		t.Fatal("F1 did not wait for target")
	}
	inventoryClick(t, c, c.hotbar.target.Children[1])
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}) {
		t.Fatalf("basic attack %x", got)
	}
	c.HotbarKey(0x71, 0)
	if len(sent()) != 1 || c.hotbar.target != nil {
		t.Fatal("duplicate turn submitted")
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.HotbarKey(0x71, 0)
	got = sent()
	if len(got) != 2 || !bytes.Equal(got[1], []byte{50, 4, 4, 2, 4, 2, 0x75, 0xea}) {
		t.Fatalf("Defense %x", got)
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.SkillState.Learned[11016] = skills.Progress{Grade: 1}
	c.HotKeys.Bindings[1][3] = hud.Binding{Kind: 2, ID: 11016}
	c.HotbarKey(0x72, 0)
	if c.hotbar.target == nil {
		t.Fatal("learned skill not usable")
	}
	c.remote.battle.ready = false
	inventoryClick(t, c, c.hotbar.target.Children[1])
	if len(sent()) != 2 {
		t.Fatal("expired prompt sent action")
	}
	c.remote.battle.ready = true
	c.remote.battle.fighters[0].sp = 0
	c.HotbarKey(0x72, 0)
	if c.hotbar.target != nil || len(sent()) != 2 {
		t.Fatal("insufficient SP allowed action")
	}
	c.remote.battle.fighters[0].sp = 100
	delete(c.SkillState.Learned, 11016)
	c.HotbarKey(0x72, 0)
	if c.hotbar.target != nil || len(sent()) != 2 {
		t.Fatal("unlearned skill used")
	}
	c.HotbarKey(0x70, 0)
	if c.hotbar.target == nil {
		t.Fatal("basic target prompt")
	}
	if !c.HotbarKey(27, 0) || c.hotbar.target != nil || c.UI.Modal != nil {
		t.Fatal("Escape target cleanup")
	}
}
func TestLogoutScreenshotConfirmationAndCancel(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.Settings.Show()
	baseline := len(sent())
	c.settingsAction("logout")
	f := c.logoutPrompt
	if f == nil || c.UI.Modal != f || f.Width != 280 || f.Height != 200 {
		t.Fatal("native logout confirmation missing")
	}
	c.Frame()
	if out := os.Getenv("HOTBAR_SNAPSHOT"); out != "" {
		savePNG(t, out+".logout.png", c)
	}
	inventoryClick(t, c, f.Children[2]) // Cancel
	if c.logoutPrompt != nil || c.UI.Modal != nil || c.World == nil || c.Exit || len(sent()) != baseline {
		t.Fatal("Cancel logged out")
	}
	c.settingsAction("logout")
	if !c.SettingsKey(27) || c.UI.Modal != nil || c.logoutPrompt != nil {
		t.Fatal("Escape logged out/left shade")
	}
	c.settingsAction("logout")
	inventoryClick(t, c, c.logoutPrompt.Children[1]) // Confirm
	if c.World != nil || c.G.InGame || !c.Servers.Visible || c.Exit || c.logoutPrompt != nil {
		t.Fatal("logout confirmation did not return to server selection")
	}
}

func TestHotbarOwnedPetAndReleasedTarget(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.mapReady = true
	sent := wire(t, c)
	c.InventoryState.Pets[2] = inventory.UsePet{ID: 14156}
	enemy := remoteRecord(5, 7, 2, 2, 20001, 0)
	enemy[2] = 2
	for _, p := range [][]byte{remoteRecord(250, 2, 4, 2, 10001, 0), remoteRecord(5, 4, 3, 2, 14156, 10001), enemy, {50, 6, 4, 2, 0}, {52, 1}} {
		c.dispatch(p)
	}
	b := c.skillBinding(10001, 3)
	c.useBinding(b)
	if c.hotbar.target == nil {
		t.Fatal("owned pet could not select action")
	}
	inventoryClick(t, c, c.hotbar.target.Children[1])
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{50, 1, 3, 2, 2, 2, 0x11, 0x27}) {
		t.Fatalf("pet action %x", got)
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	c.useBinding(b)
	c.InventoryState.Pets[2] = inventory.UsePet{}
	inventoryClick(t, c, c.hotbar.target.Children[1])
	if len(sent()) != 1 {
		t.Fatal("released pet submitted an action")
	}
}

func TestHotbarSupportTargetsRespectSideAndRevival(t *testing.T) {
	c, _, _ := enteredClient(t)
	actor := remoteFighter{id: c.World.Player.ID, side: 1, x: 4, y: 2, hp: 100}
	fallen := remoteFighter{id: 10002, side: 1, x: 4, y: 3}
	enemy := remoteFighter{id: 20001, side: 2, x: 2, y: 2, hp: 100}
	c.remote.battle.fighters = []remoteFighter{actor, fallen, enemy}
	binding := hud.Binding{Kind: 2, ID: 11016}
	definition := c.SkillState.Catalog.Definitions[binding.ID]
	definition.EffectLayer = 5 // Native status removal, not revival.
	c.SkillState.Catalog.Definitions[binding.ID] = definition
	c.openHotbarTargets(binding, actor)
	if len(c.hotbar.target.Children) != 3 { // Title, living ally, Cancel.
		t.Fatal("status removal offered an enemy or defeated ally")
	}
	definition.EffectLayer = 8 // Native revival category.
	c.SkillState.Catalog.Definitions[binding.ID] = definition
	c.openHotbarTargets(binding, actor)
	if len(c.hotbar.target.Children) != 4 { // Title, both allies, Cancel.
		t.Fatal("revival omitted defeated ally or offered an enemy")
	}
	c.closeHotbarTarget()
}
