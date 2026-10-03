package login

import "encoding/binary"

// Table sizes of the original's global arrays.
const (
	nameTableEntries = 10000 // PTR_DAT_004c9dc8; searched up to index 9999
	statusEntries    = 10000 // PTR_DAT_004ca06c; larger IDs are ignored
	regionSlots      = 100   // the form's +0x180 and +0x310 list arrays
	maxRegion        = 99    // region numbers 1..99 in SERVER.INI
	serversPerRegion = 100   // server lines read after a region line
	hiddenRegion     = 0x5f  // region 95 is kept out of the list
	statusIDsPerFlag = 100   // status ID = flag*100 + index + 1
	maxSignal        = 3     // icon_ServerSignal rows 0..3
)

// Globals are the shared PTR_DAT_ state the login forms read and write.
type Globals struct {
	// Mode is PTR_DAT_004c9ca4 +0x23d: 0 normal (sounds play); 1 and 2
	// skip the server list, 2 also the account form.
	Mode byte
	// InGame is DAT_00828128 +4; connecting is refused while it is set.
	InGame bool
	// NetActive is PTR_DAT_004ca328, set when a socket is opened.
	NetActive bool

	Names       [nameTableEntries][]byte // region and server names
	Status      [statusEntries]byte      // signal per status ID
	ServerIndex [statusEntries]uint16    // status ID per global server row
	FlagRegion  map[int]int              // PTR_DAT_004ca768: flag → region
	Offset      byte                     // PTR_DAT_004caa18 +0x577c
	HiddenName  []byte                   // PTR_DAT_004c9ca4 +0x260
	HiddenAddr  []byte                   // PTR_DAT_004c9ca4 +0x264

	statusBuf []byte // DAT_00839d58, a partial status record

	// Server description from packet 1/9 (FUN_004a7028).
	ServerWord   uint16 // DAT_00839d62
	ServerFlag   bool   // DAT_00839d68
	ServerText   []byte // DAT_00839d64
	LoginStarted bool   // DAT_008265f4 +0x5716

	// CharacterActive is PTR_DAT_004ca2f8 +4: set by 1/3 and on entering
	// the world, cleared when creation is cancelled.
	CharacterActive bool
	// PlayerID is the player object's ID (DAT_008265f4 +4); 0 until the
	// world is entered. The name filter exempts some IDs.
	PlayerID int32
}

func NewGlobals() *Globals { return &Globals{FlagRegion: map[int]int{}} }

// statusRecordBytes is one status record: u16 ID and a signal.
const statusRecordBytes = 3

// ReadStatus is the record loop of ClientSocket3Read (0x4a6940). Every
// three bytes are an ID and a signal; the reply's header parses as an ID
// past the table and is dropped.
func (g *Globals) ReadStatus(data []byte) {
	buf := append(g.statusBuf, data...)
	for len(buf) >= statusRecordBytes {
		id := int(binary.LittleEndian.Uint16(buf))
		if id < statusEntries {
			g.Status[id] = buf[2]
		}
		buf = buf[statusRecordBytes:]
	}
	g.statusBuf = append([]byte(nil), buf...)
}
