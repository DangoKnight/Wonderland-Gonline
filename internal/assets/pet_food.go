package assets

import (
	"encoding/binary"
	"wonderland-gonline/internal/game"
)

type PetFood struct{ Targets []uint32 }

// Decoded Item.dat type-20 food target, corroborated by all 24 native food
// descriptions. The four-byte in-memory prefix is excluded from this offset.
const nativePetFoodTargetOffset = 413

func (i NativeItem) PetFoodTarget() uint32 {
	if i.Definition.Type != game.ItemTypePetStatFood {
		return 0
	}
	return uint32(binary.LittleEndian.Uint16(i.Record[nativePetFoodTargetOffset:]))
}
