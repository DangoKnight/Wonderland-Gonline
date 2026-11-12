package protocol

import (
	"bytes"
	"testing"
)

func TestConnectionCapabilityGoldens(t *testing.T) {
	for _, sub := range []byte{248, 249, 250} {
		want := []byte{1, sub, 71, 76, 79, 78, 1, 0, 255, 0, 0, 0}
		// Keep the original non-agent wire golden independent of later features.
		if !bytes.Equal(ConnectionCapabilities(sub, ConnectionKnownFeatures&^ConnectionAgentAPI), want) {
			t.Fatal("connection wire changed", sub)
		}
		if got, ok := ParseConnectionCapabilities(want, sub); !ok || got != 255 {
			t.Fatal(got, ok)
		}
		for i := range len(want) {
			if _, ok := ParseConnectionCapabilities(want[:i], sub); ok {
				t.Fatal("accepted truncated hello", i)
			}
		}
		for _, bad := range [][]byte{{1, sub, 1}, {1, sub, 71, 76, 79, 78, 2, 0, 7, 0, 0, 0}, {1, sub, 71, 76, 66, 82, 1, 0, 7, 0, 0, 0}, append(append([]byte(nil), want...), 0)} {
			if _, ok := ParseConnectionCapabilities(bad, sub); ok {
				t.Fatal("accepted incompatible hello", bad)
			}
		}
	}
}

func TestAgentAccessProtocolGoldens(t *testing.T) {
	want := []byte{1, 249, 71, 76, 79, 78, 1, 0, 0, 1, 0, 0}
	if !bytes.Equal(ConnectionCapabilities(HandshakeConnectionAccept, ConnectionAgentAPI), want) {
		t.Fatal("agent declaration changed")
	}
	denial := append([]byte{1, 251}, []byte("This server is specifically configured to disallow agentic AI and API-controlled gameplay. Please respect the server operator's choice and use a regular, manually controlled client.")...)
	if !bytes.Equal(AgentAccessDenied(), denial) || !IsAgentAccessDenied(denial) || IsAgentAccessDenied(denial[:2]) {
		t.Fatal("policy refusal layout changed")
	}
}
