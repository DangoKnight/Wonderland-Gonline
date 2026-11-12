package protocol

import "bytes"

const (
	ConnectionProtocolVersion         = 1
	ConnectionCapabilityBytes         = 12
	ConnectionCollectionTiming uint32 = 1 << 0
	ConnectionCombatRoundChain uint32 = 1 << 1
	ConnectionClientBackdrops  uint32 = 1 << 2
	// Native AC5:4 follows the complete map snapshot, including props and vehicles.
	ConnectionCompleteMapSnapshot uint32 = 1 << 3
	ConnectionObservedPickups     uint32 = 1 << 4
	ConnectionAFK                 uint32 = 1 << 5
	ConnectionAFKAggregation      uint32 = 1 << 6
	ConnectionAFKDelta            uint32 = 1 << 7
	ConnectionAgentAPI            uint32 = 1 << 8
	ConnectionKnownFeatures              = ConnectionCollectionTiming | ConnectionCombatRoundChain | ConnectionClientBackdrops | ConnectionCompleteMapSnapshot | ConnectionObservedPickups | ConnectionAFK | ConnectionAFKAggregation | ConnectionAFKDelta | ConnectionAgentAPI
)

// AgentAccessDeniedMessage is deliberately explicit about the operator's policy.
const AgentAccessDeniedMessage = "This server is specifically configured to disallow agentic AI and API-controlled gameplay. Please respect the server operator's choice and use a regular, manually controlled client."

func AgentAccessDenied() []byte {
	return Builder{CommandHandshake, HandshakeAgentAccessDenied}.Bytes([]byte(AgentAccessDeniedMessage))
}

func IsAgentAccessDenied(p []byte) bool { return bytes.Equal(p, AgentAccessDenied()) }

// One application hello after TCP connection establishment. Native framing and
// discovery stay intact so unknown peers continue speaking the Legacy protocol.
func ConnectionCapabilities(sub byte, flags uint32) []byte {
	return Builder{CommandHandshake, sub}.Bytes([]byte("GLON")).U16(ConnectionProtocolVersion).U32(flags)
}

func ParseConnectionCapabilities(p []byte, sub byte) (uint32, bool) {
	if len(p) != ConnectionCapabilityBytes || p[0] != CommandHandshake || p[1] != sub || !bytes.Equal(p[2:6], []byte("GLON")) {
		return 0, false
	}
	r := NewReader(p[6:])
	version, flags := r.U16(), r.U32()
	return flags, version == ConnectionProtocolVersion
}
