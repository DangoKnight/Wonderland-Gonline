package app

import (
	"bytes"
	"testing"
	"time"
	"wonderland-gonline/internal/clientassets"
)

func TestConnectionHelloSelectsAllFeaturesOnce(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.resources.battleBackdrops = map[uint16]clientassets.GroundBackdrops{60001: {Default: 91}}
	for _, p := range [][]byte{{1, 250, 71, 76, 79, 78, 1, 0, 7, 0, 0, 0}, {1, 248, 1}, {1, 248, 71, 76, 79, 78, 2, 0, 7, 0, 0, 0}} {
		c.dispatch(p)
	}
	if c.connection.offered || c.connection.enabled {
		t.Fatal("invalid hello enabled connection")
	}
	offer := []byte{1, 248, 71, 76, 79, 78, 1, 0, 7, 0, 0, 0}
	c.dispatch(offer)
	c.dispatch(offer)
	if !c.connection.offered || c.connection.enabled || c.combat.enabled || c.backdrops.enabled || c.gonlineCollectionTiming {
		t.Fatal("features enabled before confirmation")
	}
	deadline := time.Now().Add(time.Second)
	for len(sent()) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if packets := sent(); len(packets) != 1 || !bytes.Equal(packets[0], []byte{1, 249, 71, 76, 79, 78, 1, 0, 7, 0, 0, 0}) {
		t.Fatal("expected one combined acceptance", packets)
	}
	c.dispatch([]byte{1, 250, 71, 76, 79, 78, 1, 0, 5, 0, 0, 0})
	if c.connection.enabled {
		t.Fatal("mismatched confirmation accepted")
	}
	c.dispatch([]byte{1, 250, 71, 76, 79, 78, 1, 0, 7, 0, 0, 0})
	if !c.connection.enabled || !c.combat.enabled || !c.backdrops.enabled || !c.gonlineCollectionTiming {
		t.Fatal("shared features not enabled")
	}
	c.dispatch(offer)
	c.dispatch([]byte{1, 240, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0})
	c.dispatch([]byte{1, 244, 71, 76, 66, 68, 1, 0})
	if len(sent()) != 1 {
		t.Fatal("per-feature handshakes continued after connection hello")
	}
	c.dispatch([]byte{1, 247, 71, 76, 66, 68, 1, 0, 97, 234, 154, 8, 115, 5})
	c.dispatch(remoteRecord(250, 2, 4, 2, 10001, 0))
	if c.battle.state.Background != 91 {
		t.Fatal("shared mode did not enable local backdrop resolution")
	}
	c.Servers.OnConnecting()
	if c.connection.offered || c.connection.enabled || c.combat.enabled || c.backdrops.enabled || c.gonlineCollectionTiming {
		t.Fatal("connection features survived reconnect")
	}
}

func TestConnectionHelloRespectsConfigAndMissingAssets(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	c.resources.battleBackdrops = nil
	// Server has chains disabled and the client cannot resolve backdrops.
	c.dispatch([]byte{1, 248, 71, 76, 79, 78, 1, 0, 13, 0, 0, 0}) // Includes an unknown future bit, ignored by intersection.
	if c.connection.features != 1 {
		t.Fatal("unavailable features selected", c.connection.features)
	}
	c.dispatch([]byte{1, 250, 71, 76, 79, 78, 1, 0, 1, 0, 0, 0})
	if !c.connection.enabled || !c.gonlineCollectionTiming || c.combat.enabled || c.backdrops.enabled {
		t.Fatal("feature subset not respected")
	}
	c.dispatch([]byte{1, 242, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0})
	c.dispatch([]byte{1, 246, 71, 76, 66, 68, 1, 0})
	if c.combat.enabled || c.backdrops.enabled {
		t.Fatal("legacy feature confirmation changed negotiated mode")
	}
}
