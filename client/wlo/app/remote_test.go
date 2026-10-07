package app

import (
	"bytes"
	"encoding/binary"
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
)

func remoteClient(t *testing.T) (*Client, *time.Time, inventory.RemoteOptions) {
	t.Helper()
	c, now, _ := enteredClient(t)
	c.mapReady = true
	c.Stats.HP, c.Stats.MaxHP, c.Stats.SP, c.Stats.MaxSP = 100, 100, 100, 100
	c.InventoryState.Bag[0] = game.Item{ID: 34058, Count: 1}
	return c, now, inventory.RemoteOptions{SupplyFirst: 1, SupplyLast: 50, Thresholds: [4]int{50, 50, 50, 50}, PlayerDeaths: 3, PetDeaths: 3, Information: true}
}

func TestRemoteConfirmWalkingAndInformation(t *testing.T) {
	c, now, _ := remoteClient(t)
	c.Inventory.OpenRemote(1)
	d := c.Inventory.Remote
	inventoryClick(t, c, d.Checks[1])
	inventoryClick(t, c, d.Start)
	if !c.remote.active || d.Visible || !c.remote.button.Visible {
		t.Fatal("Confirm did not start/hide/show shortcut")
	}
	if c.remote.button.AnimImage < 0 {
		t.Fatalf("missing shortcut artwork: controller icon %d", c.items[34058].Icon)
	}
	c.remoteTick()
	if !c.World.Walking() {
		t.Fatal("auto walk did not begin")
	}
	direction := c.remote.direction
	*now = now.Add(time.Second)
	c.remoteTick()
	if c.remote.direction != direction {
		t.Fatal("active path replaced")
	}
	c.Frame()
	if out := os.Getenv("REMOTE_SNAPSHOT"); out != "" {
		savePNG(t, out+".info.png", c)
	}
	c.remote.options.Information = false
	c.Frame()
	if !c.remote.button.Visible {
		t.Fatal("information toggle hid shortcut")
	}
	if out := os.Getenv("REMOTE_SNAPSHOT"); out != "" {
		savePNG(t, out+".hidden.png", c)
	}
	inventoryClick(t, c, c.remote.button)
	if !d.Visible {
		t.Fatal("shortcut failed")
	}
	inventoryClick(t, c, d.Stop)
	if c.remote.active || c.World.Walking() || c.remote.button.Visible {
		t.Fatal("Stop failed")
	}
}

func TestRemoteSupplyReceiptsAndTimeout(t *testing.T) {
	c, now, o := remoteClient(t)
	sent := wire(t, c)
	c.items[32011] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 8, Status: [2]uint16{25}, Values: [2]int32{200}}}
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 3}
	c.Stats.HP = 49
	o.AutoSupply = true
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 15, 2, 1, 0, 0}) {
		t.Fatal("supply request", got)
	}
	if c.InventoryState.Bag[1].Count != 3 || c.Stats.HP != 49 {
		t.Fatal("optimistic consumption")
	}
	// Removing the stack alone is not enough: wait until the vitals/use ACK.
	c.dispatch([]byte{23, 9, 2, 1})
	*now = now.Add(time.Second)
	c.remoteTick()
	if len(sent()) != 1 || c.remote.pending == nil {
		t.Fatal("request repeated before ACK")
	}
	c.Stats.HP = 100
	c.dispatch([]byte{23, 15})
	c.remoteTick()
	if c.remote.pending != nil || len(sent()) != 1 {
		t.Fatal("receipt not resolved")
	}
	c.Stats.HP = 49
	*now = now.Add(time.Second)
	c.remoteTick()
	if len(sent()) != 2 {
		t.Fatal("next recovery")
	}
	*now = now.Add(5 * time.Second)
	c.remoteTick()
	if c.remote.active || len(sent()) != 2 || c.remote.pending != nil {
		t.Fatal("missing receipt replayed or remained active")
	}
}

func TestRemotePetSupplyAndCandidateSafety(t *testing.T) {
	c, _, o := remoteClient(t)
	sent := wire(t, c)
	c.InventoryState.Pets[1] = inventory.UsePet{ID: 123, Stats: world.Stats{HP: 49, MaxHP: 100, SP: 100, MaxSP: 100}}
	c.remote.activePet = 123
	c.items[32011] = assets.NativeItem{Definition: game.ItemDefinition{Status: [2]uint16{25}, Values: [2]int32{200}}}
	c.items[32012] = assets.NativeItem{Definition: game.ItemDefinition{Status: [2]uint16{25, 26}, Values: [2]int32{200, 50}}}
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 3, Locked: true}
	c.InventoryState.Bag[2] = game.Item{ID: 32012, Count: 3} // harmful SP effect: skip
	c.InventoryState.Bag[3] = game.Item{ID: 32011, Count: 3}
	o.AutoSupply = true
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 15, 4, 1, 2, 0}) {
		t.Fatal("safe pet recovery", got)
	}
}

func TestRemoteDiscardSafeguards(t *testing.T) {
	c, now, o := remoteClient(t)
	sent := wire(t, c)
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 3, Locked: true}
	c.InventoryState.Bag[2] = game.Item{ID: 32012, Count: 4}
	c.InventoryState.Bag[3] = game.Item{ID: 32011, Count: 2}
	o.Discard = [5]uint16{34058, 32011}
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	if len(sent()) != 0 {
		t.Fatal("discard toggle ignored")
	}
	c.remote.options.AutoDiscard = true
	*now = now.Add(time.Second)
	c.remoteTick()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 124, 4, 2, 0}) {
		t.Fatal("discard safeguards", got)
	}
	if c.InventoryState.Bag[3].Count != 2 {
		t.Fatal("optimistic deletion")
	}
	*now = now.Add(time.Second)
	c.remoteTick()
	if len(sent()) != 1 {
		t.Fatal("duplicate discard")
	}
	c.dispatch([]byte{23, 9, 4, 2})
	c.remoteTick()
	if c.remote.pending != nil || len(sent()) != 1 {
		t.Fatal("discard receipt")
	}
}

func TestRemoteUnequipSpaceAndOwnership(t *testing.T) {
	c, _, o := remoteClient(t)
	sent := wire(t, c)
	c.items[1001] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1, CellWidth: 2, CellHeight: 2}}
	c.InventoryState.Equipment[0] = game.Item{ID: 1001, Count: 1, Damage: 200}
	o.AutoUnequip = true
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 12, 1, 2}) {
		t.Fatal("unequip", got)
	}
	if c.InventoryState.Equipment[0].ID != 1001 || !c.InventoryState.Bag[1].Empty() {
		t.Fatal("optimistic unequip")
	}
	c.stopRemote()
	c.remote.pending = nil
	for i := 1; i < len(c.InventoryState.Bag); i++ {
		c.InventoryState.Bag[i] = game.Item{ID: 32011, Count: 50}
	}
	if !c.startRemote(o) {
		t.Fatal("restart")
	}
	c.remoteTick()
	if len(sent()) != 1 {
		t.Fatal("full bag accepted unequip")
	}
}

// Raw native fighter records, independent of production constants/builders.
func remoteRecord(code, kind, x, y byte, id, owner uint32) []byte {
	p := []byte{11, code}
	if code == 250 {
		p = append(p, 0, 0)
	}
	r := make([]byte, 32)
	r[0], r[1], r[12], r[13] = 1, kind, x, y
	binary.LittleEndian.PutUint32(r[2:], id)
	binary.LittleEndian.PutUint32(r[8:], owner)
	binary.LittleEndian.PutUint32(r[14:], 100)
	binary.LittleEndian.PutUint16(r[18:], 100)
	binary.LittleEndian.PutUint32(r[20:], 100)
	binary.LittleEndian.PutUint16(r[24:], 100)
	return append(p, r...)
}
func TestRemoteBattleRoundActionsAndDeathCounts(t *testing.T) {
	c, now, o := remoteClient(t)
	sent := wire(t, c)
	o.AutoFight = true
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	for _, p := range [][]byte{
		remoteRecord(250, 2, 4, 2, 10001, 0), remoteRecord(5, 4, 3, 2, 123, 10001), remoteRecord(5, 2, 4, 3, 10002, 0), remoteRecord(5, 4, 3, 3, 124, 10002), remoteRecord(5, 7, 2, 2, 20001, 0), {50, 6, 4, 2, 0}, {52, 1},
	} {
		c.dispatch(p)
	}
	c.remoteTick()
	got := sent()
	if len(got) != 2 || !bytes.Equal(got[0], []byte{50, 1, 4, 2, 2, 2, 0x11, 0x27}) || !bytes.Equal(got[1], []byte{50, 1, 3, 2, 2, 2, 0x11, 0x27}) {
		t.Fatal("round actors", got)
	}
	c.dispatch([]byte{52, 1})
	*now = now.Add(time.Second)
	c.remoteTick()
	if len(sent()) != 2 {
		t.Fatal("duplicate ready repeated actions")
	}
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	*now = now.Add(time.Second)
	c.remoteTick()
	if len(sent()) != 4 {
		t.Fatal("next round missing")
	}
	c.dispatch([]byte{51, 1, 3, 2, 25, 0, 0, 0, 0})
	c.dispatch([]byte{53, 3, 3, 2})
	c.dispatch([]byte{53, 3, 3, 2})
	if c.remote.petDeaths != 1 {
		t.Fatal("pet death count", c.remote.petDeaths)
	}
	c.dispatch([]byte{53, 3, 2, 2})
	c.dispatch([]byte{50, 6, 4, 2, 0})
	c.dispatch([]byte{52, 1})
	*now = now.Add(time.Second)
	c.remoteTick()
	if !c.remote.active || len(sent()) != 4 {
		t.Fatal("finished monster roster should wait for battle exit")
	}
	c.dispatch([]byte{11, 0, 0x11, 0x27, 0, 0, 0, 0})
	if c.remote.battle.active {
		t.Fatal("battle exit not reset")
	}
}

func TestRemoteLeaveAndSessionBoundaries(t *testing.T) {
	for _, condition := range []string{"timer", "death", "no supplies", "map", "controller"} {
		t.Run(condition, func(t *testing.T) {
			c, now, o := remoteClient(t)
			wire(t, c)
			switch condition {
			case "timer":
				o.LeaveAfter = true
				o.LeaveMinutes = 1
			case "death":
				o.LeavePlayerDeaths = true
				o.PlayerDeaths = 1
			case "no supplies":
				o.AutoSupply = true
				o.LeaveNoSupplies = true
				c.Stats.HP = 49
			}
			if !c.startRemote(o) {
				t.Fatal("start")
			}
			switch condition {
			case "timer":
				*now = now.Add(time.Minute)
			case "death":
				c.remote.playerDeaths = 1
			case "map":
				c.World.Player.Map++
			case "controller":
				c.InventoryState.Bag[0] = game.Item{}
			}
			c.remoteTick()
			if c.remote.active {
				t.Fatal("condition didn't stop automation")
			}
			if condition != "map" && condition != "controller" && !c.Lost.Visible {
				t.Fatal("enabled leave didn't disconnect")
			}
		})
	}
	c, _, o := remoteClient(t)
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remote.playerDeaths = 99
	c.remote.petDeaths = 99
	c.remoteTick()
	if !c.remote.active {
		t.Fatal("disabled death condition disconnected")
	}
	c.Inventory.ResetRemote()
	if c.remote.active {
		t.Fatal("session reset didn't stop")
	}
}

func TestRemoteRejectInvalidOptionsAndPackets(t *testing.T) {
	c, _, o := remoteClient(t)
	o.SupplyFirst = 0
	if c.startRemote(o) {
		t.Fatal("invalid slot")
	}
	o.SupplyFirst = 1
	o.Thresholds[0] = 101
	if c.startRemote(o) {
		t.Fatal("invalid threshold")
	}
	p := remoteRecord(250, 2, 4, 2, 10002, 0)
	if c.remoteBattlePacket(p) || c.remote.battle.active {
		t.Fatal("foreign formation")
	}
	p = remoteRecord(250, 2, 5, 2, 10001, 0)
	if c.remoteBattlePacket(p) || c.remote.battle.active {
		t.Fatal("invalid cell")
	}
}

func TestRemoteDiscardSelectionDoesNotConsume(t *testing.T) {
	c, _, _ := remoteClient(t)
	sent := wire(t, c)
	c.Inventory.OpenRemote(1)
	c.Inventory.Show()
	d := c.Inventory.Remote
	cell := d.DiscardCells[0].Abs()
	drag := func(slot int) {
		s := c.Inventory.Slots[slot]
		at := s.Abs()
		s.LeftDown(0, at.X+10, at.Y+10)
		s.LeftUpOutside(0, cell.X+10, cell.Y+10)
	}
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 3}
	drag(1)
	if d.Discard[0] != 32011 || len(sent()) != 0 || c.InventoryState.Bag[1].Count != 3 {
		t.Fatal("selection consumed/sent item")
	}
	drag(0)
	if d.Discard[0] != 32011 {
		t.Fatal("controller selected")
	}
	c.InventoryState.Bag[2] = game.Item{ID: 32012, Count: 1, Locked: true}
	drag(2)
	if d.Discard[0] != 32011 {
		t.Fatal("locked item selected")
	}
	d.DiscardCells[0].RightUp(0, cell.X+10, cell.Y+10)
	if d.Discard[0] != 0 {
		t.Fatal("right click did not clear selection")
	}
}

func TestRemoteWalkingPausesForServerLocks(t *testing.T) {
	for _, pause := range []string{"loading", "held", "event", "recruitment", "dead", "battle"} {
		t.Run(pause, func(t *testing.T) {
			c, _, o := remoteClient(t)
			o.AutoMove = true
			if !c.startRemote(o) {
				t.Fatal("start")
			}
			switch pause {
			case "loading":
				c.mapReady = false
			case "held":
				c.held = true
			case "event":
				c.event.active = true
			case "recruitment":
				c.petAnnouncement = true
			case "dead":
				c.Stats.HP = 0
			case "battle":
				c.remote.battle.active = true
			}
			c.remoteTick()
			if c.World.Walking() {
				t.Fatal("walking during", pause)
			}
		})
	}
}

func TestRemotePetUnequipReceipt(t *testing.T) {
	c, now, o := remoteClient(t)
	sent := wire(t, c)
	c.InventoryState.Pets[1] = inventory.UsePet{ID: 123}
	c.InventoryState.Pets[1].Equipment[0] = game.Item{ID: 1001, Count: 1, Damage: 200}
	c.items[1001] = assets.NativeItem{Definition: game.ItemDefinition{EquipSlot: 1}}
	c.remote.activePet = 123
	o.AutoUnequip = true
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	got := sent()
	if len(got) != 1 || !bytes.Equal(got[0], []byte{23, 18, 2, 1, 2}) {
		t.Fatal("pet unequip", got)
	}
	c.dispatch([]byte{23, 22, 2, 1, 2})
	*now = now.Add(time.Second)
	c.remoteTick()
	if c.remote.pending != nil || !c.InventoryState.Pets[1].Equipment[0].Empty() || c.InventoryState.Bag[1].ID != 1001 || len(sent()) != 1 {
		t.Fatal("pet receipt")
	}
}

func TestRemoteStoppedRequestStillResolves(t *testing.T) {
	c, now, o := remoteClient(t)
	sent := wire(t, c)
	c.InventoryState.Bag[1] = game.Item{ID: 32011, Count: 2}
	o.AutoDiscard = true
	o.Discard[0] = 32011
	if !c.startRemote(o) {
		t.Fatal("start")
	}
	c.remoteTick()
	c.stopRemote()
	if c.startRemote(o) {
		t.Fatal("restarted before outstanding deletion receipt")
	}
	c.dispatch([]byte{23, 9, 2, 2})
	*now = now.Add(time.Second)
	c.remoteTick()
	if c.remote.pending != nil || !c.startRemote(o) || len(sent()) != 1 {
		t.Fatal("stopped request prevented subsequent start")
	}
}
