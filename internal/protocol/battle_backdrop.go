package protocol

import "bytes"

const battleBackdropVersion = 1
const BattleBackdropCapabilityBytes = 8
const BattleBackdropLocationBytes = 14

// A separate opt-in, independent of combat transport. Native handshake echoes
// cannot enable local backdrop resolution.
func BattleBackdropCapability(sub byte) []byte {
	return []byte{CommandHandshake, sub, 'G', 'L', 'B', 'D', battleBackdropVersion, 0}
}

func IsBattleBackdropCapability(p []byte, sub byte) bool {
	return bytes.Equal(p, BattleBackdropCapability(sub))
}

type BattleBackdropLocation struct{ Map, X, Y uint16 }

func (l BattleBackdropLocation) Packet() []byte {
	return Builder(BattleBackdropCapability(HandshakeBackdropLocation)).U16(l.Map).U16(l.X).U16(l.Y)
}

func ParseBattleBackdropLocation(p []byte) (BattleBackdropLocation, bool) {
	if len(p) != BattleBackdropLocationBytes || !IsBattleBackdropCapability(p[:BattleBackdropCapabilityBytes], HandshakeBackdropLocation) {
		return BattleBackdropLocation{}, false
	}
	r := NewReader(p[BattleBackdropCapabilityBytes:])
	l := BattleBackdropLocation{r.U16(), r.U16(), r.U16()}
	return l, l.Map != 0
}
