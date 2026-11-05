package assets

import (
	"fmt"
	"math"
	"slices"
	"wonderland-gonline/internal/game"
)

const (
	nativeItemCellWidthOffset  = 406
	nativeItemCellHeightOffset = 407
	nativeItemRecordBytes      = 451
	nativeItemHeaderRecords    = 1
	nativeItemVersionOffset    = 114
	nativeItemEncryptedVersion = 9
	nativeItemByteMask         = 0x9A
	nativeItemWordMask         = 0xEFC3
	nativeItemDwordMask        = 0x0B80F4B4
	nativeItemValueBias        = 9
	nativeItemNameOffset       = 1
	nativeItemNameBytes        = 14
	nativeItemTypeOffset       = 15
	nativeItemIDOffset         = 16
	nativeItemIconOffset       = 18
	nativeItemLargeIconOffset  = 20
	nativeItemSpriteOffset     = 22
	nativeItemSpriteCount      = 4
	nativeItemStatusOffset     = 30
	// Legacy converted-import mapping; native equipment level rules remain unresolved.
	nativeItemLegacyLevelOffset       = 34
	nativeItemValueOffset             = 36
	nativeItemEquipSlotOffset         = 46
	nativeItemDescriptionLengthOffset = 146
	nativeItemDescriptionOffset       = 147
	nativeItemDescriptionBytes        = 254
	nativeItemBuyPriceOffset          = 114
	nativeItemEquipLevelOffset        = 113 // Decoded Item.dat wearer minimum level (player and pet).
)

// Disk offsets traced from aLogin's FUN_003cdfa4. Its in-memory object has a
// four-byte prefix; these tables already subtract it. Unresolved fields remain
// positional data in the decoded record, rather than invented game properties.
var nativeItemByteOffsets = [...]int{
	15, 34, 35, 44, 45, 46, 47, 112, 113, 122, 129,
	401, 402, 403, 406, 407, 408, 415, 416, 417, 418, 419, 420,
}
var nativeItemWordOffsets = [...]int{
	16, 18, 20, 22, 24, 26, 28, 30, 32, 123, 134, 136, 138, 140, 142, 144,
	404, 409, 411, 413, 421, 423, 425, 427, 429,
}
var nativeItemDwordOffsets = [...]int{
	36, 40, 48, 52, 56, 60, 64, 68, 72, 76, 80, 84, 88, 92, 96, 100, 104, 108,
	114, 118, 125, 130, 431, 435, 439, 443, 447,
}

// NativeItem retains the entire decrypted disk record, including fields whose
// gameplay meaning is still unresolved. Definition is the existing server
// projection; client-only hardcoded item patches are not part of decryption.
type NativeItem struct {
	Definition  game.ItemDefinition
	Description string
	Icon        uint16
	LargeIcon   uint16
	Sprites     [nativeItemSpriteCount]uint16
	Record      [nativeItemRecordBytes]byte
}

const vehicleCapacityRankOffset = 45 // Native Item.dat +0x31, excluding its four-byte prefix.
const vehicleCapacityUnitsPerRank = 1800
const vehicleCapacityRankPower = 1.3

// VehicleCapacity follows FUN_00262974/FUN_0044d5d0: round(rank^1.3)*1800.
func (i NativeItem) VehicleCapacity() uint32 {
	if i.Definition.Type != game.VehicleType {
		return 0
	}
	return uint32(math.RoundToEven(math.Pow(float64(i.Record[vehicleCapacityRankOffset]), vehicleCapacityRankPower))) * vehicleCapacityUnitsPerRank
}

// VehicleCondition is shared by inventory and the mounted-vehicle HUD.
type VehicleCondition struct{ Health, Maximum, Fuel uint32 }

func (i NativeItem) VehicleCondition(item game.Item) VehicleCondition {
	if i.Definition.Type != game.VehicleType || item.Empty() {
		return VehicleCondition{}
	}
	capacity := i.VehicleCapacity()
	fuel := uint32(item.VehicleFuel())
	if capacity == 0 {
		capacity, fuel = game.VehicleWreckDamage, 0
	}
	health := capacity - min(capacity, item.VehicleWear())
	// Precise native wear and the existing server damage percentage both
	// constrain remaining health; neither display may restore spent health.
	health = min(health, uint32(uint64(capacity)*uint64(game.VehicleWreckDamage-min(item.Damage, game.VehicleWreckDamage))/game.VehicleWreckDamage))
	if item.ID == game.DisposableRaftItemID {
		health = capacity
	}
	return VehicleCondition{Health: health, Maximum: capacity, Fuel: min(fuel, capacity)}
}

// ParseNativeItems reads version-nine Item.dat: one plaintext 451-byte header,
// followed by encrypted records. Integer arithmetic intentionally wraps at its
// field width, including signed status values stored as two's-complement words.
// First duplicate ID wins, consistent with the server's converted lookup.
func ParseNativeItems(data []byte) (map[uint16]NativeItem, error) {
	if len(data) < (nativeItemHeaderRecords+1)*nativeItemRecordBytes || len(data)%nativeItemRecordBytes != 0 {
		return nil, fmt.Errorf("native item catalog: invalid record length")
	}
	if version := le.Uint32(data[nativeItemVersionOffset:]); version != nativeItemEncryptedVersion {
		return nil, fmt.Errorf("native item catalog: unsupported version %d", version)
	}
	out := make(map[uint16]NativeItem)
	for offset := nativeItemHeaderRecords * nativeItemRecordBytes; offset < len(data); offset += nativeItemRecordBytes {
		var item NativeItem
		copy(item.Record[:], data[offset:offset+nativeItemRecordBytes])
		b := item.Record[:]
		if int(b[0]) > nativeItemNameBytes || int(b[nativeItemDescriptionLengthOffset]) > nativeItemDescriptionBytes {
			return nil, fmt.Errorf("native item catalog: invalid text length at record %d", offset/nativeItemRecordBytes)
		}
		slices.Reverse(b[nativeItemNameOffset : nativeItemNameOffset+nativeItemNameBytes])
		slices.Reverse(b[nativeItemDescriptionOffset : nativeItemDescriptionOffset+nativeItemDescriptionBytes])
		for _, at := range nativeItemByteOffsets {
			b[at] = (b[at] ^ nativeItemByteMask) - nativeItemValueBias
		}
		for _, at := range nativeItemWordOffsets {
			le.PutUint16(b[at:], (le.Uint16(b[at:])^nativeItemWordMask)-nativeItemValueBias)
		}
		for _, at := range nativeItemDwordOffsets {
			le.PutUint32(b[at:], (le.Uint32(b[at:])^nativeItemDwordMask)-nativeItemValueBias)
		}
		id := le.Uint16(b[nativeItemIDOffset:])
		if id == 0 {
			continue
		}
		name, err := decodeItemText(b[nativeItemNameOffset : nativeItemNameOffset+int(b[0])])
		if err != nil {
			return nil, fmt.Errorf("native item %d name: %w", id, err)
		}
		description, err := decodeItemText(b[nativeItemDescriptionOffset : nativeItemDescriptionOffset+int(b[nativeItemDescriptionLengthOffset])])
		if err != nil {
			return nil, fmt.Errorf("native item %d description: %w", id, err)
		}
		item.Definition = game.ItemDefinition{CellWidth: b[nativeItemCellWidthOffset], CellHeight: b[nativeItemCellHeightOffset], ID: id, Name: name, Type: b[nativeItemTypeOffset], EquipSlot: uint16(b[nativeItemEquipSlotOffset]), Level: uint16(b[nativeItemLegacyLevelOffset]), Status: [2]uint16{le.Uint16(b[nativeItemStatusOffset:]), le.Uint16(b[nativeItemStatusOffset+2:])}, Values: [2]int32{int32(le.Uint32(b[nativeItemValueOffset:])), int32(le.Uint32(b[nativeItemValueOffset+4:]))}}
		item.Description = description
		item.InitializeEquipLevel()
		item.Icon = le.Uint16(b[nativeItemIconOffset:])
		item.LargeIcon = le.Uint16(b[nativeItemLargeIconOffset:])
		for i := range item.Sprites {
			item.Sprites[i] = le.Uint16(b[nativeItemSpriteOffset+i*2:])
		}
		if _, exists := out[id]; !exists {
			out[id] = item
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("native item catalog: no item definitions")
	}
	return out, nil
}

// InitializeEquipLevel projects the native equipment requirement at import or
// explicit database migration time. It is distinct from the legacy Level field.
func (item *NativeItem) InitializeEquipLevel() {
	item.Definition.EquipLevel = uint16(item.Record[nativeItemEquipLevelOffset])
}

// BuyPrice is the native shop price stored at Item.dat +0x72.
func (item *NativeItem) BuyPrice() uint32 {
	return le.Uint32(item.Record[nativeItemBuyPriceOffset:])
}

// InitializeInventoryDimensions seeds projections made before dimensions were
// named. This is an import/migration helper; gameplay reads typed SQL fields.
func (item *NativeItem) InitializeInventoryDimensions() {
	if item.Definition.CellWidth == 0 && item.Definition.CellHeight == 0 {
		item.Definition.CellWidth = item.Record[nativeItemCellWidthOffset]
		item.Definition.CellHeight = item.Record[nativeItemCellHeightOffset]
	}
}
