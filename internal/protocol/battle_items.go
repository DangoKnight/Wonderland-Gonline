package protocol

const (
	BattleItemRequestBytes = 12
	// The native client's send path appends a 40-byte compatibility trailer.
	// Captured from legacy aLogin -> Gonline on 2026-11-03. It is not authority
	// for ownership, inventory, targets or effects; validate the leading fields.
	BattleItemLegacyTrailerBytes = 40
	BattleItemLegacyRequestBytes = BattleItemRequestBytes + BattleItemLegacyTrailerBytes
)

// BattleItemChoice uses the native request layout. Gonline leaves the opaque
// random/compatibility trailer at zero; the server never treats it as authority.
func BattleItemChoice(sx, sy, tx, ty byte, item uint16, slot byte) []byte {
	return Builder{CommandBattleAction, BattleActionItemRequest, sx, sy, tx, ty}.U16(item).U16(0).U8(slot).U8(0)
}
