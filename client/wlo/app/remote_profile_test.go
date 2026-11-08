package app

import (
	"os"
	"path/filepath"
	"testing"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/internal/game"
)

func remoteProfileOptions() inventory.RemoteOptions {
	o := inventory.RemoteOptions{AutoFight: true, AutoMove: true, AutoSupply: true, LeaveNoSupplies: true, LeavePlayerDeaths: true, LeavePetDeaths: true, LeaveAfter: true, AutoUnequip: true, AutoDiscard: true, Information: true, SupplyFirst: 5, SupplyLast: 40, PlayerDeaths: 4, PetDeaths: 7, LeaveMinutes: 90, Thresholds: [4]int{10, 25, 75, 100}, Discard: [5]uint16{100, 200, 300}}
	for i := range o.Advanced {
		a := &o.Advanced[i]
		a.Enabled = [6]bool{true, true, true, true, true, true}
		a.HP, a.SP, a.EnemyHP, a.EnemyAbove = 123+i, 45+i, 678+i, i == 1
		for j := range a.Assignments {
			a.Assignments[j] = inventory.RemoteAssignment{ID: uint16(11000 + j), Item: j > 4, Target: byte(j%4 + 1)}
		}
	}
	return o
}

func TestRemoteProfileRoundTripAndInvalidPreservation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles", "remote-10001.json")
	o := remoteProfileOptions()
	if err := saveRemoteProfile(path, o); err != nil {
		t.Fatal(err)
	}
	got, found, err := readRemoteProfile(path)
	if err != nil || !found || got != o {
		t.Fatal("Remote configuration did not round-trip", got, err)
	}
	o.LeaveMinutes++
	if err := saveRemoteProfile(path, o); err != nil {
		t.Fatal("atomic replacement failed", err)
	}
	if replaced, found, err := readRemoteProfile(path); err != nil || !found || replaced != o {
		t.Fatal("replacement did not save latest settings", err)
	}
	bad := o
	bad.SupplyFirst = 0
	if err := saveRemoteProfile(path, bad); err == nil {
		t.Fatal("invalid profile saved")
	}
	got, _, err = readRemoteProfile(path)
	if err != nil || got != o {
		t.Fatal("failed save replaced valid profile")
	}
	for _, contents := range []string{`{`, `{"version":99,"options":{}}`, `{"version":1,"options":{"SupplyFirst":1,"SupplyLast":50,"Thresholds":[101,0,0,0]}}`} {
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		if _, found, err := readRemoteProfile(path); err == nil || found {
			t.Fatal("corrupt/unsupported profile accepted", contents)
		}
	}
	if _, found, err := readRemoteProfile(filepath.Join(t.TempDir(), "missing")); found || err != nil {
		t.Fatal("missing profile should use defaults", err)
	}
}

func TestRemoteProfileRestoresControlsWithoutStarting(t *testing.T) {
	c, _, _ := remoteClient(t)
	c.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	o := remoteProfileOptions()
	if !c.startRemote(o) {
		t.Fatal("confirmed settings did not save/start")
	}
	c.remote.playerDeaths, c.remote.petDeaths = 99, 98
	c.remote.sequence = [2]int{2, 3}
	c.Inventory.ResetRemote()
	c.resetRemoteRuntime()
	c.loadRemoteProfile()
	if c.remote.active || c.remote.playerDeaths != 0 || c.remote.petDeaths != 0 || c.remote.sequence != [2]int{} {
		t.Fatal("profile restored live automation state")
	}
	c.Inventory.OpenRemote(1)
	d := c.Inventory.Remote
	if string(d.Fields[0].Text) != "5" || string(d.Fields[1].Text) != "40" || d.Thresholds[3].Pos != 100 {
		t.Fatal("saved main controls not restored")
	}
	inventoryClick(t, c, d.Start)
	if !c.remote.active || c.remote.options != o {
		t.Fatal("restored controls changed confirmed configuration", c.remote.options)
	}
	firstPath := c.remoteProfilePath()
	c.dispatch(selfPacket(10002, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Other"))
	if c.remoteProfilePath() == firstPath || c.remote.active || c.remote.options == o {
		t.Fatal("profile leaked to another character")
	}
	c.InventoryState.Bag[0] = game.Item{ID: 34058, Count: 1}
	c.Inventory.OpenRemote(1)
	if string(c.Inventory.Remote.Fields[0].Text) != "1" {
		t.Fatal("new character inherited old controls")
	}
	c.dispatch(selfPacket(10001, 2, 10017, 1042, 1075, 0, 444444444, 444444444, nil, "Original"))
	if c.remote.active || c.remote.options != o {
		t.Fatal("original character profile not restored")
	}
}

func TestRemoteProfileSaveFailureDoesNotStart(t *testing.T) {
	c, _, o := remoteClient(t)
	blocked := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocked, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	c.settingsPath = filepath.Join(blocked, "settings.json")
	if c.startRemote(o) || c.remote.active {
		t.Fatal("automation started despite failed configuration save")
	}
	raw, err := os.ReadFile(blocked)
	if err != nil || string(raw) != "preserve" {
		t.Fatal("failed save damaged existing file", err)
	}
}
