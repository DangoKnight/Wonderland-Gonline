package app

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/protocol"
)

func TestAgentRequiresExplicitServerPermission(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	c.AgentEnableSessionHost()
	c.agent.serverAttached.Store(true)
	if _, err := c.AgentSubmit([]byte(`{"command":"walk.to","arguments":{"x":1100,"y":1075}}`)); err == nil {
		t.Fatal("unpermitted API command queued")
	}
	if err := c.Net.Send([]byte{6, 1, 0, 1, 0, 1, 0}); err == nil || len(sent()) != 0 {
		t.Fatal("raw network path bypassed permission")
	}
	if _, ok := c.AgentState()["player"]; ok {
		t.Fatal("unpermitted state leaked gameplay")
	}
	if _, err := c.AgentNativeStream(0); err == nil {
		t.Fatal("unpermitted presentation stream exposed")
	}
	offer := protocol.ConnectionCapabilities(protocol.HandshakeConnectionOffer, protocol.ConnectionCollectionTiming)
	c.dispatch(offer)
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 249, 71, 76, 79, 78, 1, 0, 1, 1, 0, 0}) {
		t.Fatal("agent did not declare itself on denying server", got)
	}
	if c.AgentState()["error"] != protocol.AgentAccessDeniedMessage {
		t.Fatal("policy message missing")
	}
	c.dispatch(protocol.AgentAccessDenied())
	if c.AgentAccessError() == nil {
		t.Fatal("denial did not persist")
	}
}

func TestAgentPermissionConfirmationAndReconnect(t *testing.T) {
	c, _, _ := enteredClient(t)
	_ = wire(t, c)
	c.AgentEnableSessionHost()
	c.agent.serverAttached.Store(true)
	flags := uint32(protocol.ConnectionAgentAPI | protocol.ConnectionCollectionTiming)
	c.dispatch(protocol.ConnectionCapabilities(protocol.HandshakeConnectionOffer, flags))
	if c.AgentAccessError() == nil {
		t.Fatal("offer alone granted access")
	}
	c.dispatch(protocol.ConnectionCapabilities(protocol.HandshakeConnectionEnabled, flags))
	if c.AgentAccessError() != nil {
		t.Fatal("server confirmation did not grant API access")
	}
	if _, err := c.AgentSubmit([]byte(`{"command":"walk.to","arguments":{"x":1100,"y":1075}}`)); err != nil {
		t.Fatal(err)
	}
	c.Servers.OnConnecting()
	if c.AgentAccessError() == nil {
		t.Fatal("authorization survived reconnect")
	}
}

func TestNativeReplicaRetainsAgentNegotiatedFeatures(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.Net.SetPresentationOnly()
	c.replica = &nativeReplica{}
	flags := uint32(protocol.ConnectionAgentAPI | protocol.ConnectionCompleteMapSnapshot)
	c.dispatch(protocol.ConnectionCapabilities(protocol.HandshakeConnectionOffer, flags))
	c.dispatch(protocol.ConnectionCapabilities(protocol.HandshakeConnectionEnabled, flags))
	if !c.connection.enabled || c.connection.features != flags {
		t.Fatal("native replica lost hosted protocol features")
	}
}
