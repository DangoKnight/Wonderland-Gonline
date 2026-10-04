package protocol

// Launcher status is an unframed reply on port 6416, separate from game packets.
const (
	StatusReplyOpcode     = 201
	StatusReplySubtype    = 0
	StatusDefaultCluster  = 1
	StatusLegacyServerID  = 1
	StatusDefaultServerID = 101
	StatusLoadOffline     = 0
	StatusLoadGreen       = 1
	StatusLoadYellow      = 2
	StatusLoadRed         = 3
	// Native clients store status IDs in a 10000-entry table, with zero unused.
	StatusMaxServerID      = 9999
	StatusMaxServerRecords = 100
)
