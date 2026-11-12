package server

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/server/transport"
)

func TestAgentPolicyDefaultDenialAndBypassRefusal(t *testing.T) {
	s, cs, ws := worldFixture(t)
	c := cs[0]
	ctx := context.Background()
	if err := s.discoveryCommand(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if c.connectionOffered&protocol.ConnectionAgentAPI != 0 {
		t.Fatal("agent API offered by default")
	}
	ws[0].Reset()
	if err := s.acceptConnectionProtocol(c, protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, protocol.ConnectionAgentAPI)); err != nil {
		t.Fatal(err)
	}
	if c.connectionNegotiated || !c.agentRequested || !contains(ws[0].packets(t), protocol.AgentAccessDenied()) {
		t.Fatal("agent session not explicitly refused")
	}
	before := c.character.Clone()
	ws[0].Reset()
	if err := s.dispatch(ctx, c, []byte{6, 1, 0, 1, 0, 1, 0}); err != nil {
		t.Fatal(err)
	}
	if c.character.X != before.X || c.character.Y != before.Y || !contains(ws[0].packets(t), protocol.AgentAccessDenied()) {
		t.Fatal("declared agent bypassed policy")
	}
}

func TestAgentPolicyRefusalArrivesBeforeDisconnect(t *testing.T) {
	s, cs, _ := worldFixture(t)
	c := cs[0]
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	output := transport.New(serverSide, nil)
	defer output.Close()
	c.conn = output
	if err := s.acceptConnectionProtocol(c, protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, protocol.ConnectionAgentAPI)); err != nil {
		t.Fatal(err)
	}
	_ = clientSide.SetReadDeadline(time.Now().Add(time.Second))
	packet, err := protocol.Read(clientSide)
	if err != nil || !bytes.Equal(packet, protocol.AgentAccessDenied()) {
		t.Fatal("policy message was lost on disconnect", packet, err)
	}
	if _, err := protocol.Read(clientSide); err == nil {
		t.Fatal("refused connection remained open")
	}
}

func TestAgentPolicyPersistenceAndImmediateRevocation(t *testing.T) {
	s, cs, ws := worldFixture(t)
	c := cs[0]
	ctx := context.Background()
	if err := s.LoadRuntimeSettings(ctx); err != nil {
		t.Fatal(err)
	}
	settings, _ := s.Store.Settings(ctx)
	if settings["agent_api_enabled"] != "false" {
		t.Fatal("missing default-deny seed")
	}
	v := s.RuntimeSettings()
	v.AgentAPIEnabled = true
	if err := s.UpdateRuntimeSettings(ctx, v); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.EnsureAgentPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.LoadRuntimeSettings(ctx); err != nil || !s.RuntimeSettings().AgentAPIEnabled {
		t.Fatal("opt-in was not preserved", err)
	}
	if err := s.discoveryCommand(ctx, c, nil); err != nil {
		t.Fatal(err)
	}
	if c.connectionOffered&protocol.ConnectionAgentAPI == 0 {
		t.Fatal("opt-in was not offered")
	}
	if err := s.acceptConnectionProtocol(c, protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, protocol.ConnectionAgentAPI)); err != nil {
		t.Fatal(err)
	}
	if !c.connectionNegotiated {
		t.Fatal("permitted agent not negotiated")
	}
	s.sessions[c.info.ID] = c
	s.sessions[cs[1].info.ID] = cs[1]
	for _, w := range ws {
		w.Reset()
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	v.AgentAPIEnabled = false
	if err := s.UpdateRuntimeSettings(canceled, v); err == nil || !s.RuntimeSettings().AgentAPIEnabled || ws[0].Len() != 0 {
		t.Fatal("failed policy write revoked a live session")
	}
	if err := s.UpdateRuntimeSettings(ctx, v); err != nil {
		t.Fatal(err)
	}
	if !contains(ws[0].packets(t), protocol.AgentAccessDenied()) || ws[1].Len() != 0 {
		t.Fatal("revocation failed or affected manual client")
	}
	if err := s.Store.SaveSettings(ctx, map[string]string{"agent_api_enabled": "yes"}); err == nil {
		t.Fatal("invalid opt-in accepted")
	}
}

func TestAgentIdentityCannotBeDowngraded(t *testing.T) {
	s, cs, ws := worldFixture(t)
	c := cs[0]
	s.agentAPIEnabled = true
	if err := s.offerConnectionProtocol(c); err != nil {
		t.Fatal(err)
	}
	if err := s.acceptConnectionProtocol(c, protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, protocol.ConnectionAgentAPI)); err != nil {
		t.Fatal(err)
	}
	ws[0].Reset()
	// An allowed server rejects role downgrade as a protocol error; it must
	// not falsely claim that its operator disallows all agent gameplay.
	if err := s.acceptConnectionProtocol(c, protocol.ConnectionCapabilities(protocol.HandshakeConnectionAccept, 0)); !errors.Is(err, protocol.ErrMalformed) {
		t.Fatal("agent identity downgrade was not rejected as a protocol error", err)
	}
	if !c.agentRequested || c.connectionFeatures&protocol.ConnectionAgentAPI == 0 || contains(ws[0].packets(t), protocol.AgentAccessDenied()) {
		t.Fatal("agent identity downgraded to manual")
	}
}
