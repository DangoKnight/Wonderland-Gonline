package app

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/clientassets"
)

func TestClientBackdropNegotiationAndFormation(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.resources.battleBackdrops = map[uint16]clientassets.GroundBackdrops{60001: {Default: 80, Regions: []clientassets.GroundBackdropRegion{{Selector: 91, Left: 1899, Top: 1165, Right: 2844, Bottom: 1442}}}}
	c.dispatch([]byte{1, 246, 71, 76, 66, 68, 1, 0})
	if c.backdrops.enabled {
		t.Fatal("unsolicited confirmation accepted")
	}
	c.dispatch([]byte{1, 244, 1})
	if c.backdrops.offered {
		t.Fatal("generic echo activated backdrops")
	}
	c.dispatch([]byte{1, 244, 71, 76, 66, 68, 1, 0})
	if !c.backdrops.offered || c.backdrops.enabled {
		t.Fatal("wrong negotiation state")
	}
	deadline := time.Now().Add(time.Second)
	for len(sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if p := sent(); len(p) != 1 || !bytes.Equal(p[0], []byte{1, 245, 71, 76, 66, 68, 1, 0}) {
		t.Fatal(p)
	}
	c.dispatch([]byte{1, 246, 71, 76, 66, 68, 1, 0})
	if !c.backdrops.enabled || c.combat.enabled {
		t.Fatal("backdrops incorrectly depend on combat chains")
	}
	c.dispatch([]byte{1, 247, 71, 76, 66, 68, 1, 0, 97, 234, 154, 8, 115, 5})
	formation := remoteRecord(250, 2, 4, 2, 10001, 0)
	formation[2] = 1
	c.dispatch(formation)
	if c.battle.state.Background != 91 || c.backdrops.pending {
		t.Fatal("client did not resolve shared entry coordinates", c.battle.state.Background)
	}
	// A subsequent formation cannot reuse the previous battle's context.
	formation[2] = 50
	c.dispatch(formation)
	if c.battle.state.Background != 50 {
		t.Fatal("stale backdrop context reused")
	}
	c.Servers.OnConnecting()
	if c.backdrops.enabled || c.backdrops.offered || c.backdrops.pending {
		t.Fatal("backdrop state leaked between servers")
	}
	// Legacy server packets retain their own selector even with local metadata.
	c.dispatch([]byte{1, 247, 71, 76, 66, 68, 1, 0, 97, 234, 154, 8, 115, 5})
	c.dispatch(formation)
	if c.battle.state.Background != 50 {
		t.Fatal("legacy server selector overridden")
	}
}

func TestClientBackdropOfferWithoutAssets(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.resources.battleBackdrops = nil
	c.dispatch([]byte{1, 244, 71, 76, 66, 68, 1, 0})
	if c.backdrops.offered {
		t.Fatal("client accepted local resolution without metadata")
	}
}

func TestClientBackdropContextWaitsForRoundPlayback(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.resources.battleBackdrops = map[uint16]clientassets.GroundBackdrops{60001: {Default: 91}}
	c.backdrops.enabled = true
	round := &combatPlayback{}
	c.combat.round = round
	c.dispatch([]byte{1, 247, 71, 76, 66, 68, 1, 0, 97, 234, 154, 8, 115, 5})
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	if c.backdrops.pending || len(round.deferred) != 2 {
		t.Fatal("entry context bypassed round playback ordering")
	}
	c.combat.round = nil
	for _, p := range round.deferred {
		c.dispatch(p)
	}
	if c.battle.state.Background != 91 || c.backdrops.pending {
		t.Fatal("deferred context/formation ordering lost")
	}
}
