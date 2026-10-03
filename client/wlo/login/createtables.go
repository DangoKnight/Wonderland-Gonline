package login

import "image"

// Authored tables of character creation (WLRI build).

// Portrait pictures of the carousel, by character 1..14: the centre x and
// the y the picture's bottom sits 15 pixels below (FUN_00215d38).
var rolePicAt = [rolePicCount + 1]image.Point{
	{},
	{0xb2, 0x1dd}, {0x293, 0x1d1}, {0x394, 0x1e4}, {0x457, 0x1c7},
	{0x399, 0x1c8}, {0x2cf, 0x1c6}, {0x13b, 0x1c8}, {0x335, 0x1d6},
	{0x19d, 0x1c9}, {0x260, 0x1d0}, {0x81, 0x1c8}, {0x402, 0x1c6},
	{0x1fa, 0x1c8}, {0xf8, 0x1c7},
}

// rolePicStops is DAT_004bd460: the left edge the carousel brings the
// current entry's picture to, by entry 1..14.
var rolePicStops = [rolePicCount + 1]int{0, 79, 51, 68, 68, 57, 50, 55, 48, 64, 62, 78, 69, 50, 50}

// creationRoles is FUN_0021838c's switch, by character 1..14: body type,
// head, where the selected picture is drawn (bottom centre), the
// description picture and its position.
var creationRoles = [rolePicCount + 1]struct {
	Type, Head byte
	Art        image.Point
	Info       string
	InfoAt     image.Point
}{
	{},
	{2, 0, image.Pt(0x69, 0x1b3), "RoleInfo_2_1", image.Pt(0x87, 0x100)},
	{2, 1, image.Pt(0xe1, 0x1d5), "RoleInfo_2_2", image.Pt(0x31, 0xf6)},
	{1, 0, image.Pt(0x81, 0x1ac), "RoleInfo_1_1", image.Pt(0xa8, 0x125)},
	{4, 2, image.Pt(0xf0, 0x1da), "RoleInfo_4_3", image.Pt(0x30, 0xf5)},
	{4, 1, image.Pt(0x69, 0x1d5), "RoleInfo_4_2", image.Pt(0x8b, 199)},
	{3, 0, image.Pt(0xda, 0x1e5), "RoleInfo_3_1", image.Pt(0x2e, 0xcf)},
	{3, 1, image.Pt(0xe6, 0x1da), "RoleInfo_3_2", image.Pt(0x36, 0xdb)},
	{4, 4, image.Pt(0x6d, 0x1db), "RoleInfo_4_5", image.Pt(0x8a, 0xc3)},
	{4, 5, image.Pt(0x78, 0x1d5), "RoleInfo_4_6", image.Pt(0xa0, 0xd2)},
	{4, 0, image.Pt(0xf5, 0x1db), "RoleInfo_4_1", image.Pt(0x32, 0xeb)},
	{4, 6, image.Pt(0x68, 0x1d5), "RoleInfo_4_7", image.Pt(0x87, 0xdd)},
	{3, 2, image.Pt(0xda, 0x1e5), "RoleInfo_3_3", image.Pt(0x39, 0xd6)},
	{3, 3, image.Pt(0x73, 0x1e5), "RoleInfo_3_4", image.Pt(0xac, 0xbe)},
	{4, 7, image.Pt(0x72, 0x1d5), "RoleInfo_4_8", image.Pt(0xaf, 200)},
}

// RoleIndex is FUN_00484f20: a body type and head's row in the starter
// tables, or 0.
func RoleIndex(body, head byte) byte {
	switch body {
	case 1:
		if head == 0 {
			return 3
		}
	case 2:
		switch head {
		case 0:
			return 1
		case 1:
			return 2
		}
	case 3:
		switch head {
		case 0:
			return 6
		case 1:
			return 7
		case 2:
			return 0xc
		case 3:
			return 0xe
		}
	case 4:
		if int(head) < len(roleIndexBody4) {
			return roleIndexBody4[head]
		}
	}
	return 0
}

var roleIndexBody4 = [...]byte{10, 5, 4, 0xd, 8, 9, 0xb, 0xf}

// starterItems is the table at PTR_DAT_004ca774 (0x4b5ab4): six item IDs
// per role index (FUN_00215c24 equips them for the preview).
var starterItems = [16][6]uint16{
	{},
	{22003, 21002, 0, 0, 24002, 0},
	{22001, 21003, 0, 0, 24003, 0},
	{22004, 21001, 0, 0, 24001, 0},
	{0, 21008, 0, 0, 24008, 0},
	{0, 21007, 0, 23002, 24007, 0},
	{0, 21004, 0, 0, 24004, 0},
	{0, 21005, 0, 0, 24005, 0},
	{22002, 21010, 10003, 0, 24010, 0},
	{0, 21013, 0, 0, 24013, 0},
	{22005, 21006, 0, 23001, 24006, 0},
	{22006, 21011, 10004, 0, 24011, 0},
	{0, 21012, 0, 0, 24012, 0},
	{22007, 21009, 10002, 0, 24009, 0},
	{22009, 21014, 18002, 0, 24014, 0},
	{22008, 21015, 0, 0, 24015, 0},
}

// statBonus is the table at PTR_DAT_004c9f28 (0x4b5b94): the bonus shown
// after each attribute, STR, CON, INT, WIS, AGI, per role index.
var statBonus = [16][5]uint16{
	{},
	{0, 0, 1, 1, 1},
	{0, 0, 1, 0, 2},
	{0, 0, 2, 0, 1},
	{0, 0, 3, 0, 0},
	{2, 0, 0, 0, 1},
	{1, 2, 0, 0, 0},
	{2, 1, 0, 0, 0},
	{0, 0, 1, 2, 0},
	{0, 0, 2, 1, 0},
	{1, 2, 0, 0, 0},
	{0, 0, 2, 0, 1},
	{0, 0, 0, 2, 1},
	{0, 0, 0, 0, 0},
	{1, 1, 1, 0, 0},
	{0, 0, 1, 1, 1},
}

// statHints are FUN_002198a8's tooltips, STR..AGI.
var statHints = [6]string{
	"",
	"Strength influences basic physical damage and physical skills damage",
	"Constitution increases maximum health (HP) and basic defense against physical damage",
	"Intelligence determines magical skills damage and sealing skills chance",
	"Wisdom increases maximum mana (SP), defense against magic attacks, and unlocks some support skills",
	"Agility determines movement order in battle and ability to perform combo-attacks with other characters",
}
