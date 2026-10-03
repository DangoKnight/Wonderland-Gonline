package role

// Layer order of the player draw: FUN_004465f0 with FUN_004478fc. Each
// table gives, for positions 1..7, the equipment slot drawn there (0 is
// the head). The base tables are at DAT_004c9530 + 8·k; every group has
// four variants chosen by the slot-2 item's flag byte (+8 each).
var orderTables = map[uint32][8]byte{
	0x4c9530: {0, 3, 1, 0, 2, 4, 5, 6}, 0x4c9538: {0, 3, 1, 0, 4, 2, 5, 6},
	0x4c9540: {0, 3, 1, 0, 5, 2, 4, 6}, 0x4c9548: {0, 3, 1, 0, 4, 5, 2, 6},
	0x4c9550: {0, 3, 6, 1, 0, 2, 4, 5}, 0x4c9558: {0, 3, 6, 1, 0, 4, 2, 5},
	0x4c9560: {0, 3, 6, 1, 0, 5, 2, 4}, 0x4c9568: {0, 3, 6, 1, 0, 4, 5, 2},
	0x4c9570: {0, 3, 1, 0, 6, 2, 4, 5}, 0x4c9578: {0, 3, 1, 0, 6, 4, 2, 5},
	0x4c9580: {0, 3, 1, 0, 6, 5, 2, 4}, 0x4c9588: {0, 3, 1, 0, 6, 4, 5, 2},
	0x4c9590: {0, 3, 1, 2, 6, 0, 4, 5}, 0x4c9598: {0, 3, 1, 4, 2, 6, 0, 5},
	0x4c95a0: {0, 3, 1, 5, 2, 6, 0, 4}, 0x4c95a8: {0, 3, 1, 4, 5, 2, 6, 0},
	0x4c95b0: {0, 3, 1, 0, 6, 2, 4, 5}, 0x4c95b8: {0, 3, 1, 0, 6, 4, 2, 5},
	0x4c95c0: {0, 3, 1, 0, 6, 5, 2, 4}, 0x4c95c8: {0, 3, 1, 0, 6, 4, 5, 2},
	0x4c95d0: {0, 6, 3, 1, 0, 2, 4, 5}, 0x4c95d8: {0, 6, 3, 1, 0, 4, 2, 5},
	0x4c95e0: {0, 6, 3, 1, 0, 5, 2, 4}, 0x4c95e8: {0, 6, 3, 1, 0, 4, 5, 2},
	0x4c95f0: {0, 3, 1, 0, 2, 4, 5, 6}, 0x4c95f8: {0, 3, 1, 0, 4, 2, 5, 6},
	0x4c9600: {0, 3, 1, 0, 5, 2, 4, 6}, 0x4c9608: {0, 3, 1, 0, 4, 5, 2, 6},
}

const (
	tableDefault    = 0x4c9530
	tableTwoHand    = 0x4c9550
	tableTwoHandAlt = 0x4c9570
	tableClass2     = 0x4c9590
	tableClass2Alt  = 0x4c95b0
	tableClass3Alt  = 0x4c95d0
	tableClass3     = 0x4c95f0
	variantStride   = 8
)

// Action bit sets: DAT_004478f4 (also DAT_00448a6c) for actions below
// 0x40, DAT_00448a74 and DAT_00448a80 for actions below 0x48.
var (
	twoHandActionSet = [8]byte{0x83, 0x83, 0x0c, 0x02, 0x00, 0xc0, 0xe0, 0x20}
	class2SetPlain   = [9]byte{0x44, 0x44, 0x00, 0x0c, 0xc0, 0x00, 0x11, 0x11, 0x03}
	class2SetFlagged = [9]byte{0x44, 0x44, 0x00, 0x0c, 0xc0, 0x00, 0x11, 0x00, 0x03}
)

func inSet(set []byte, limit int, a byte) bool {
	if int(a) >= limit {
		return false
	}
	return set[a>>3]>>(a&7)&1 != 0
}

// orderInput is what FUN_004465f0 reads: the body type (+8), action
// (+0x121) and frame (+0x11e), the slot-2 item's flag byte, the slot-6
// item's hand byte and the body sprite (the costume's or the armour's).
type orderInput struct {
	Body, Action byte
	Frame        int
	Flags, Hand  byte
	Sprite       int
}

// variant maps the slot-2 flag byte to a table variant. Flags 5..7 apply
// to body types 1 and 3, 9..11 to 2 and 4, 13..15 to all; other non-zero
// flags select nothing, and every position then draws slot 0.
func variant(flags, body byte) (int, bool) {
	switch {
	case flags == 0:
		return 0, true
	case flags >= 5 && flags <= 7:
		if body == 1 || body == 3 {
			return int(flags - 4), true
		}
		return 0, true
	case flags >= 9 && flags <= 11:
		if body == 2 || body == 4 {
			return int(flags - 8), true
		}
		return 0, true
	case flags >= 13 && flags <= 15:
		return int(flags - 12), true
	}
	return 0, false
}

// layerAt is FUN_004465f0 for one position (1..7).
func layerAt(in orderInput, pos int) byte {
	at := func(table uint32, v int) byte {
		return orderTables[table+uint32(v*variantStride)][pos]
	}
	class, _ := lookupRange(spriteClasses, in.Sprite)
	if class == classFixed {
		return at(tableTwoHand, 0)
	}
	v, ok := variant(in.Flags, in.Body)
	alt := inSet(twoHandActionSet[:], 0x40, in.Action)
	switch {
	case in.Hand == 1 || in.Hand == 2 || class == 1:
		if !ok {
			return 0
		}
		if alt {
			return at(tableTwoHandAlt, v)
		}
		return at(tableTwoHand, v)
	case in.Hand == 3 || class == 2:
		return class2Layer(in, pos, v, ok)
	case in.Hand == 4 || class == 3:
		if !ok {
			return 0
		}
		if alt {
			return at(tableClass3Alt, v)
		}
		if in.Flags == 0 && (in.Action == 0x28 || in.Action == 0x29) && in.Frame != 1 {
			return at(tableClass3Alt, 0)
		}
		return at(tableClass3, v)
	default:
		if !ok {
			return 0
		}
		return at(tableDefault, v)
	}
}

// class2Layer is FUN_004478fc: poses in the action sets, and some attack
// frames, use the second table.
func class2Layer(in orderInput, pos, v int, ok bool) byte {
	if !ok {
		return 0
	}
	at := func(table uint32) byte { return orderTables[table+uint32(v*variantStride)][pos] }
	if inSet(twoHandActionSet[:], 0x40, in.Action) {
		return at(tableTwoHandAlt)
	}
	set, frame24 := class2SetFlagged[:], in.Frame == 2
	if in.Flags == 0 {
		set, frame24 = class2SetPlain[:], in.Frame == 2 || in.Frame == 3
	}
	a := in.Action
	switch {
	case inSet(set, 0x48, a),
		(a == 0x20 || a == 0x21) && in.Frame == 3,
		(a == 0x22 || a == 0x23) && in.Frame != 0,
		(a == 0x24 || a == 0x25) && frame24,
		(a == 0x28 || a == 0x29) && in.Frame != 0:
		return at(tableClass2Alt)
	}
	return at(tableClass2)
}

// costumeCategory is FUN_00432c9c: 0 without a costume.
func costumeCategory(sprite int, id uint16, hand byte) byte {
	if id == 0 {
		return 0
	}
	if c, ok := lookupRange(costumeCategories, sprite); ok {
		return c
	}
	if hand >= 1 && hand <= 4 {
		return 6
	}
	return 0
}
