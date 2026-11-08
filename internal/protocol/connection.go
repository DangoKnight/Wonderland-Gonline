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
	ConnectionKnownFeatures              = ConnectionCollectionTiming | ConnectionCombatRoundChain | ConnectionClientBackdrops | ConnectionCompleteMapSnapshot | ConnectionObservedPickups
)

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
