package protocol

import "bytes"

// Native clients ignore unknown AC1 subcommands. A versioned signature prevents
// generic handshake echoes or server names from selecting Gonline timing.
const collectionTimingVersion = 1

func CollectionTimingCapability() []byte {
	return []byte{CommandHandshake, HandshakeCollectionTiming, 'G', 'L', 'C', 'T', collectionTimingVersion, 0}
}

func IsCollectionTimingCapability(p []byte) bool {
	return bytes.Equal(p, CollectionTimingCapability())
}
