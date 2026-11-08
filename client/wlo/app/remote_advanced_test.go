package app

import (
	"bytes"
	"image"
	"maps"
	"os"
	"slices"
	"testing"
	clientbattle "wonderland-gonline/client/wlo/battle"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/skills"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func TestRemoteAdvancedActionIconStableOnHoverAndPress(t *testing.T) {
	c, _, _ := remoteClient(t)
	c.Inventory.OpenRemote(1)
	inventoryClick(t, c, c.Inventory.Remote.Settings)
	p := c.Inventory.Remote.Plus
	b := p.Assignments[0]
	for i := 0; i < 3; i++ {
		inventoryClick(t, c, b) // Attack, Defend, Flee.
		if b.Image < 0 {
			t.Fatal("action artwork missing")
		}
		at := b.Abs()
		r := image.Rect(at.X, at.Y, at.X+b.Width, at.Y+b.Height)
		paint := func(state byte) []uint16 {
			c.Screen.Fill(r, 0)
			b.State = state
			b.Paint()
			var pixels []uint16
			for y := r.Min.Y; y < r.Max.Y; y++ {
				pixels = append(pixels, c.Screen.Pix[y*c.Screen.W+r.Min.X:y*c.Screen.W+r.Max.X]...)
			}
			return pixels
		}
		idle := paint(0)
		visible := false
		for _, pixel := range idle {
			if pixel != 0 {
				visible = true
				break
			}
		}
		if !visible {
			t.Fatal("idle action invisible")
		}
		if !slices.Equal(idle, paint(1)) || !slices.Equal(idle, paint(2)) {
			t.Fatal("hover or press changed action artwork")
		}
	}
}

func TestRemoteAdvancedNativeCellClickCycleAndClear(t *testing.T) {
	c, _, _ := remoteClient(t)
	c.Inventory.OpenRemote(1)
	d := c.Inventory.Remote
	inventoryClick(t, c, d.Settings)
	p := d.Plus
	for tab := 0; tab < 2; tab++ {
		inventoryClick(t, c, p.Tabs[tab])
		for i, b := range p.Assignments {
			for _, icon := range []string{"Icon_sk60022s", "Icon_sk60021s", "Icon_sk60041s", "Icon_sk60022s"} {
				inventoryClick(t, c, b)
				if b.Image < 0 || b.Image != c.Env.Pics.Find(icon) {
					t.Fatalf("tab %d cell %d cycle: got %d want %s", tab, i, b.Image, icon)
				}
			}
			c.Input.Hovered = b
			x, y := p.Left+b.Left+1, p.Top+b.Top+1
			c.UI.MouseDown(seui.ButtonRight, 0, x, y)
			c.UI.MouseUp(seui.ButtonRight, 0, x, y)
			if b.Image != -1 || b.HasHint {
				t.Fatal("right-click did not clear assignment", tab, i)
			}
			inventoryClick(t, c, b)
			if b.Image != c.Env.Pics.Find("Icon_sk60022s") {
				t.Fatal("cleared field did not restart at Attack")
			}
		}
	}
	for _, child := range p.Children {
		if child.Base().Left == 51 && child.Base().Top == 453 {
			inventoryClick(t, c, child)
			break
		}
	}
	inventoryClick(t, c, d.Start)
	for _, a := range c.remote.options.Advanced {
		for _, field := range a.Assignments {
			if field.ID != 10001 || field.Item {
				t.Fatal("native click cycle did not save correct skill", field)
			}
		}
	}
}

func TestRemoteAdvancedBuiltinDefendAndFleePackets(t *testing.T) {
	c, _, sent := enterBattle(t)
	actor := c.remote.battle.fighters[0]
	for _, id := range []uint16{skills.Defense, skills.Flee} {
		c.remote.options.Advanced[0] = inventory.RemoteAdvanced{Enabled: [6]bool{inventory.RemoteDefault: true}, Assignments: [inventory.RemoteAssignmentCount]inventory.RemoteAssignment{{ID: id}}}
		if !c.remoteAdvancedAction(actor) {
			t.Fatal("built-in action not executable", id)
		}
	}
	p := sent()
	want := [][]byte{{50, 4, 4, 2, 4, 2, 0x75, 0xea}, {50, 5, 4, 2, 4, 2, 0x89, 0xea}}
	if len(p) != len(want) {
		t.Fatal("built-in choices", p)
	}
	for i := range want {
		if !bytes.Equal(p[i], want[i]) {
			t.Fatal("native built-in packet", p)
		}
	}
}

func TestRemoteAdvancedSettingsConfirmAndCancel(t *testing.T) {
	c, _, _ := remoteClient(t)
	c.Inventory.OpenRemote(1)
	d := c.Inventory.Remote
	inventoryClick(t, c, d.Settings)
	p := d.Plus
	if !p.Visible || len(c.Inventory.RemoteChoices(false)) == 0 {
		t.Fatal("advanced settings unavailable")
	}
	if !c.Inventory.AssignRemoteSkill(skills.BasicAttack, false, p.Left+163, p.Top+92) || p.Assignments[0].Image < 0 {
		t.Fatal("skill drag assignment or native artwork unavailable")
	}
	p.Fields[0].SetText([]byte("60"))
	inventoryClick(t, c, p.Checks[inventory.RemoteHealHP])
	if string(p.Fields[0].Text) != "60" {
		t.Fatal("checkbox lost typed threshold")
	}
	inventoryClick(t, c, p.Tabs[1])
	p.Fields[0].SetText([]byte("25"))
	inventoryClick(t, c, p.Checks[inventory.RemoteHealHP])
	if path := os.Getenv("REMOTE_ADVANCED_SNAPSHOT"); path != "" {
		c.Frame()
		savePNG(t, path, c)
	}
	// Native confirmation button; changes remain local until the main Confirm.
	for _, child := range p.Children {
		if child.Base().Left == 51 && child.Base().Top == 453 {
			inventoryClick(t, c, child)
			break
		}
	}
	inventoryClick(t, c, d.Start)
	if !c.remote.active || c.remote.options.Advanced[0].HP != 60 || c.remote.options.Advanced[1].HP != 25 {
		t.Fatal("independent advanced settings not confirmed", c.remote.options.Advanced)
	}
	c.stopRemote()
	c.Inventory.OpenRemote(1)
	inventoryClick(t, c, d.Settings)
	p.Fields[0].SetText([]byte("999"))
	p.Hide()
	inventoryClick(t, c, d.Settings)
	if string(p.Fields[0].Text) != "60" {
		t.Fatal("closing committed unconfirmed settings")
	}
}

func TestRemoteAdvancedRecoverySequenceAndReservations(t *testing.T) {
	c, _, sent := enterBattle(t)
	actor := c.remote.battle.fighters[0]
	autoTestDefinition(c, autoTestAttackSkill, skills.Magical, 10)
	c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
	c.items = maps.Clone(c.items)
	c.items[32003] = assets.NativeItem{Definition: game.ItemDefinition{ID: 32003, Type: 23, EquipSlot: 8, Status: [2]uint16{25}, Values: [2]int32{180}}}
	c.InventoryState.Bag[0] = game.Item{ID: 32003, Count: 1}
	a := inventory.RemoteAdvanced{Enabled: [6]bool{inventory.RemoteHealHP: true, inventory.RemoteSequence: true}}
	a.HP = 60
	a.Assignments[5] = inventory.RemoteAssignment{ID: 32003, Item: true, Target: inventory.RemoteTargetPlayer}
	a.Assignments[2] = inventory.RemoteAssignment{ID: autoTestAttackSkill, Target: inventory.RemoteTargetEnemy}
	c.remote.options.Advanced[0] = a
	for i := range c.remote.battle.fighters {
		if c.remote.battle.fighters[i].id == actor.id {
			c.remote.battle.fighters[i].hp = 20
		}
	}
	if !c.remoteAdvancedAction(actor) {
		t.Fatal("recovery did not precede sequence")
	}
	p := sent()
	if len(p) != 1 || len(p[0]) < 12 || !bytes.Equal(p[0][:6], []byte{50, 2, 4, 2, 4, 2}) {
		t.Fatal("native item choice", p)
	}
	if c.remote.sequence[0] != 0 || c.InventoryState.Bag[0].Count != 1 {
		t.Fatal("recovery advanced sequence or consumed bag optimistically")
	}
	if !c.remoteAdvancedAction(actor) || c.remote.sequence[0] != 1 {
		t.Fatal("reserved last item was reused instead of sequence")
	}
	p = sent()
	if len(p) != 2 || p[1][1] != 1 {
		t.Fatal("sequence skill", p)
	}
	if c.remoteAdvancedAction(actor) {
		t.Fatal("one-shot sequence repeated")
	}
	// Full health and waiting targets cannot receive an automated supply.
	c.remote.reservedItems = nil
	c.remote.sequence[0] = 0
	for i := range c.remote.battle.fighters {
		if c.remote.battle.fighters[i].id == actor.id {
			c.remote.battle.fighters[i].hp = c.remote.battle.fighters[i].maxHP
		}
	}
	f := c.battle.state.At(clientbattle.Cell{X: actor.x, Y: actor.y})
	f.Waiting = true
	a.Enabled[inventory.RemoteSequence] = false
	c.remote.options.Advanced[0] = a
	if c.remoteAdvancedAction(actor) {
		t.Fatal("invalid recovery target accepted")
	}
}

func TestRemoteAdvancedPreferredSkillAndFallback(t *testing.T) {
	c, _, sent := enterBattle(t)
	autoTestDefinition(c, autoTestAttackSkill, skills.Magical, 10)
	c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
	c.remote.options.Advanced[0] = inventory.RemoteAdvanced{Enabled: [6]bool{inventory.RemoteDefault: true}, Assignments: [9]inventory.RemoteAssignment{{ID: autoTestAttackSkill}}}
	actor := c.remote.battle.fighters[0]
	if !c.remoteAdvancedAction(actor) {
		t.Fatal("preferred skill not selected")
	}
	packets := sent()
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{50, 1, 4, 2, 2, 2, byte(autoTestAttackSkill & 255), byte(autoTestAttackSkill >> 8)}) {
		t.Fatal("native skill choice", packets)
	}
	actor.sp = 0
	if c.remoteAdvancedAction(actor) {
		t.Fatal("unaffordable skill selected")
	}
}

func TestRemoteAdvancedIndependentPlayerAndPet(t *testing.T) {
	c, _, sent := enterBattle(t, remoteRecord(5, 4, 3, 2, 14162, 10001))
	autoTestDefinition(c, autoTestAttackSkill, skills.Magical, 6)
	autoTestDefinition(c, autoTestPetAttackSkill, skills.Physical, 9)
	c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
	c.InventoryState.Pets[0] = inventory.UsePet{ID: 14162, Skills: [3]game.PetSkill{{ID: autoTestPetAttackSkill, Grade: 1}}}
	c.remote.options.AutoFight, c.remote.active = true, true
	for i, id := range []uint16{autoTestAttackSkill, autoTestPetAttackSkill} {
		c.remote.options.Advanced[i] = inventory.RemoteAdvanced{Enabled: [6]bool{inventory.RemoteDefault: true}, Assignments: [inventory.RemoteAssignmentCount]inventory.RemoteAssignment{{ID: id}}}
	}
	c.remoteAttack()
	p := sent()
	want := [][]byte{{50, 1, 4, 2, 2, 2, 0xff, 0x2a}, {50, 1, 3, 2, 2, 2, 0, 0x2b}}
	if len(p) != len(want) {
		t.Fatal("missing independent choices", p)
	}
	for i := range want {
		if !bytes.Equal(p[i], want[i]) {
			t.Fatal("mixed player/pet skill", p)
		}
	}
}

func TestRemoteAdvancedConditionalSPAndRevive(t *testing.T) {
	for _, scenario := range []string{"enemy_hp", "sp", "revive", "party_death_attack"} {
		t.Run(scenario, func(t *testing.T) {
			c, _, sent := enterBattle(t, remoteRecord(5, 2, 4, 3, 10002, 0))
			layer := byte(skills.Magical)
			if scenario == "sp" {
				layer = 6
			}
			if scenario == "revive" {
				layer = nativeReviveLayer
			}
			autoTestDefinition(c, autoTestAttackSkill, layer, 1)
			c.SkillState.Learned[autoTestAttackSkill] = skills.Progress{Grade: 1}
			a := inventory.RemoteAdvanced{EnemyHP: 50, SP: 50}
			slot, condition, target := 8, inventory.RemoteEnemyHP, byte(inventory.RemoteTargetEnemy)
			if scenario == "sp" {
				slot, condition, target = 6, inventory.RemoteHealSP, inventory.RemoteTargetPlayer
			}
			if scenario == "revive" || scenario == "party_death_attack" {
				slot, condition, target = 7, inventory.RemoteEmergency, inventory.RemoteTargetParty
			}
			a.Enabled[condition] = true
			a.Assignments[slot] = inventory.RemoteAssignment{ID: autoTestAttackSkill, Target: target}
			for i := range c.remote.battle.fighters {
				f := &c.remote.battle.fighters[i]
				if scenario == "enemy_hp" && f.kind == remoteFighterMonster {
					f.hp = 20
				}
				if scenario == "sp" && f.id == c.World.Player.ID {
					f.sp = 10
				}
				if (scenario == "revive" || scenario == "party_death_attack") && f.id == 10002 {
					f.hp = 0
				}
			}
			c.remote.options.Advanced[0] = a
			if !c.remoteAdvancedAction(c.remote.battle.fighters[0]) {
				t.Fatal("conditional action not selected")
			}
			p := sent()
			wantCell := []byte{2, 2}
			if scenario == "sp" {
				wantCell = []byte{4, 2}
			}
			if scenario == "revive" {
				wantCell = []byte{4, 3}
			}
			if len(p) != 1 || !bytes.Equal(p[0][4:6], wantCell) {
				t.Fatal("conditional target", p)
			}
		})
	}
}
