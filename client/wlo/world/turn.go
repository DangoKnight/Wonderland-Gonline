package world

// actionBases is FUN_00427150: the first action of an action's family,
// [first, last, base] by range. Walking (0..7) and standing (8..15) turn
// through eight facings; most other families have two.
var actionBases = [][3]int{
	{0x00, 0x07, 0x00}, {0x08, 0x0f, 0x08}, {0x10, 0x11, 0x10}, {0x12, 0x19, 0x12},
	{0x1a, 0x1b, 0x1a}, {0x1c, 0x1d, 0x1c}, {0x1e, 0x1f, 0x1e}, {0x20, 0x21, 0x20},
	{0x22, 0x23, 0x22}, {0x24, 0x25, 0x24}, {0x26, 0x27, 0x26}, {0x28, 0x29, 0x28},
	{0x2a, 0x2b, 0x2a}, {0x2c, 0x2d, 0x2c}, {0x2e, 0x35, 0x2e}, {0x36, 0x3d, 0x36},
	{0x3e, 0x3f, 0x3e}, {0x40, 0x41, 0x40}, {0x42, 0x49, 0x42}, {99, 99, 99},
}

// ActionBase is FUN_00427150; unknown actions give 0.
func ActionBase(action int) int {
	for _, r := range actionBases {
		if action >= r[0] && action <= r[1] {
			return r[2]
		}
	}
	return 0
}

// TurnAction is FUN_00432638: the action of a character at (x, y) turned
// toward (tx, ty), in its current family.
func TurnAction(action, x, y, tx, ty int) int {
	base := ActionBase(action)
	return Facing(x, y, tx, ty, (action-base)%directionsCount) + base
}

// eventDirections are the points kinds 11 and 14 of FUN_00304fd0 turn a
// character toward for directions 1..8 (jump tables at 0x306c43 and
// 0x306ec5): ten pixels up, then clockwise.
var eventDirections = [8][2]int{{0, -10}, {10, -10}, {10, 0}, {10, 10}, {0, 10}, {-10, 10}, {-10, 0}, {-10, -10}}

// DirectionPoint is the point an event direction (1..8) names from (x, y);
// false for any other value, which the original leaves unset.
func DirectionPoint(x, y, d int) (int, int, bool) {
	if d < 1 || d > len(eventDirections) {
		return 0, 0, false
	}
	o := eventDirections[d-1]
	return x + o[0], y + o[1], true
}

// actionFamily is the size of an action's family: 8 facings or 2.
func actionFamily(action int) int {
	for _, r := range actionBases {
		if action >= r[0] && action <= r[1] {
			return r[1] - r[0] + 1
		}
	}
	return 1
}

// PoseAction is FUN_00430474's facing for a held pose: the pose family's
// action that keeps the current facing (eight facings, or the left and
// right of a two-facing family, the left half being facings 0..3).
func PoseAction(pose, current int) int {
	base := ActionBase(current)
	facing := current - base
	if actionFamily(current) == 2 {
		facing *= directionsCount / 2
	}
	facing %= directionsCount
	switch actionFamily(pose) {
	case directionsCount:
		return pose + facing
	case 2:
		if facing >= directionsCount/2 {
			return pose + 1
		}
	}
	return pose
}
