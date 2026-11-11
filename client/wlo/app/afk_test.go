package app

import (
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/world"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func enableAFKTest(t *testing.T, c *Client) {
	t.Helper()
	c.connection = connectionRuntime{enabled: true, features: protocol.ConnectionAFK}
	c.mapReady = true
	c.stopMapLoading()
	if err := c.SetAFK(true); err != nil {
		t.Fatal(err)
	}
	if c.afk.active || !c.afk.pending {
		t.Fatal("enabled without server acknowledgment")
	}
	c.dispatch([]byte{253, 1, 1})
	if !c.afk.active || c.afk.pending {
		t.Fatal("AFK acknowledgment ignored")
	}
}

func TestAFKNegotiatedIndependentAndRefreshFence(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	if err := c.SetAFK(true); err == nil {
		t.Fatal("legacy server allowed AFK")
	}
	c.dispatch([]byte{253, 1, 1})
	if c.afk.active {
		t.Fatal("unsolicited server message enabled AFK")
	}
	enableAFKTest(t, c)
	w := NewWorkspace(c)
	defer w.Close()
	if err := w.Add(); err != nil {
		t.Fatal(err)
	}
	next := w.Sessions[1].Client
	if next.afk.active || next.afk.pending {
		t.Fatal("new session inherited AFK")
	}
	c.World.Peers = map[uint32]*world.Peer{2: {Player: world.Player{ID: 2}}}
	c.combat.round = &combatPlayback{}
	if err := c.SetAFK(false); err != nil {
		t.Fatal(err)
	}
	c.dispatch([]byte{253, 3})
	if !c.afk.active || !c.afk.resync || len(c.World.Peers) != 0 {
		t.Fatal("world revealed before stale peers cleared")
	}
	c.dispatch([]byte{7, 2, 0, 0, 0, 33, 39, 50, 0, 50, 0})
	if len(c.combat.round.deferred) != 0 {
		t.Fatal("peer refresh deferred past AFK end fence")
	}
	c.dispatch([]byte{253, 4})
	if c.afk.active || c.afk.pending || !c.afk.resync {
		t.Fatal("AFK did not wait for local render preparation")
	}
	c.afk.prepared = true
	c.Sprites.WaitNative()
	c.afkTick()
	if c.afk.resync {
		t.Fatal("prepared scene did not restore")
	}
}

func TestAFKBoundedLogSnapshotTravelAndPrompt(t *testing.T) {
	c, now, _ := enteredClient(t)
	_ = wire(t, c)
	enableAFKTest(t, c)
	for i := 0; i < afkLogLimit*2; i++ {
		c.afkLog("entry")
	}
	if len(c.afk.log) != afkLogLimit {
		t.Fatal("unbounded log")
	}
	state := protocol.AFKState{Map: 10017, X: 1042, Y: 1075, Units: []protocol.AFKUnit{{Owner: 10001, ID: 10001, Name: "Dango", Level: 7, HP: 50, MaxHP: 100, SP: 20, MaxSP: 40}}}
	p, err := protocol.EncodeAFKState(state)
	if err != nil {
		t.Fatal(err)
	}
	c.dispatch(p)
	if c.afk.state.Units[0].HP != 50 {
		t.Fatal("authoritative vital snapshot ignored")
	}
	state.Units[0].HP = 0
	state.Units[0].EXP = 50
	p, _ = protocol.EncodeAFKState(state)
	c.dispatch(p)
	if len(c.afk.log) != afkLogLimit || c.afk.state.Units[0].EXP != 50 {
		t.Fatal("vital event log failed")
	}
	if !c.afkIgnore([]byte{4, 2, 0, 0, 0}) || c.afkIgnore([]byte{22, 11}) || c.afkIgnore([]byte{51, 1}) {
		t.Fatal("unsafe packet classification")
	}
	// Normal map-ready processing must release hidden loading screens too.
	c.startMapLoading()
	c.mapReady = true
	c.afkTick()
	if c.mapLoading.active || c.afk.pending {
		t.Fatal("travel left AFK automation behind a loading gate")
	}
	*now = now.Add(time.Second)
	c.event.active = true
	c.afkTick()
	if !c.afk.pending {
		t.Fatal("unexpected dialogue did not request normal view")
	}
}

func TestAFKRemoteBattleAndRenderedLog(t *testing.T) {
	c, now, sent := enterBattle(t)
	c.Stats.HP, c.Stats.MaxHP = 100, 100
	c.InventoryState.Bag[0] = game.Item{ID: 34058, Count: 1}
	if !c.startRemote(inventory.RemoteOptions{AutoFight: true, SupplyFirst: 1, SupplyLast: 50}) {
		t.Fatal("could not start Remote")
	}
	enableAFKTest(t, c)
	before := len(sent())
	*now = now.Add(time.Second)
	c.Frame()
	if !c.afk.active || c.afk.pending {
		t.Fatal("AFK stopped during battle")
	}
	if !c.remote.active || len(sent()) <= before {
		t.Fatal("AFK stopped automatic battle choices")
	}
	// Both native and negotiated battle machinery continue without scene drawing.
	c.combat.enabled = true
	c.combat.round = &combatPlayback{steps: []protocol.CombatStep{{Packets: [][]byte{{52, 1}}, Delay: time.Second}}}
	c.battle.state.Actions = nil
	c.battle.playing = nil
	c.battle.arrivals = nil
	c.battle.petDepartures = nil
	c.battle.departures = nil
	c.battle.transitionPackets = nil
	c.battle.overkills = nil
	c.combatRoundTick(c.Now())
	if c.combat.round.index != 1 {
		t.Fatal("round chain stalled in AFK")
	}
	c.afk.state = protocol.AFKState{Map: 10017, X: 1042, Y: 1075, Units: []protocol.AFKUnit{
		{Owner: 10001, ID: 10001, Name: "Dango", Level: 7, HP: 181, MaxHP: 223, SP: 75, MaxSP: 109, EXP: 400, EXPNext: 800},
		{Owner: 10001, ID: 20001, Kind: protocol.AFKPet, Name: "Roca", Level: 6, HP: 130, MaxHP: 180, SP: 45, MaxSP: 60, EXP: 200, EXPNext: 700},
	}}
	c.afkLog("Roca: HP 130/180, SP 45/60")
	c.afkLog("Turn 2")
	c.drawAFK()
	if path := os.Getenv("AFK_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}

func TestAFKDisconnectAndReconnectClearMode(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	enableAFKTest(t, c)
	c.disconnected()
	if c.afk.active || c.afk.pending || c.afk.resync || c.AFKSupported() {
		t.Fatal("AFK survived disconnect")
	}
	if len(c.afk.log) == 0 {
		t.Fatal("connection problem was not recorded")
	}
	c.Servers.OnConnecting()
	if c.AFKSupported() || c.afk.active {
		t.Fatal("reconnect silently restored AFK")
	}
}
