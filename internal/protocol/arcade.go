package protocol

// Native TSportManage kinds and AC71 payment/reply discriminators.
const (
	ArcadeEgg               = 6
	ArcadeSlots             = 8
	ArcadeSlots2            = 10
	ArcadeSlots3            = 19
	ArcadeEgg2              = 22
	ArcadePayPoints         = 1
	ArcadePayFunTokens      = 2
	ArcadePayPlayTokens     = 3
	ArcadeAccepted          = 1
	ArcadeRejected          = 2
	ArcadeEggRequestBytes   = 3
	ArcadeSlotsRequestBytes = 4
	ArcadePrizeCount        = 11
	ArcadeReelSymbols       = 16
)

// The "Little Mary" fruit machines (sport kinds 11, 12 and 14). Mario 1
// requests a play with its kind alone; Mario 2 and 3 add the payment mode.
// A play's outcome is reply 3 (prize, three reel positions, quantity:
// VMT +0x24, FUN_00149040) or reply 4 (the same with a lamp byte before
// the quantity: VMT +0x28, FUN_001488cc); reply 5 reports a full bag.
const (
	ArcadeMario              = 11
	ArcadeMario2             = 12
	ArcadeMario3             = 14
	ArcadeMarioRequestBytes  = 2
	ArcadeMarioJackpot       = 3
	ArcadeMarioPrize         = 4
	ArcadeMarioBagFull       = 5
	ArcadeMarioLampWin       = 1
	ArcadeMarioReelPositions = 8 // FUN_0014966c's tables hold positions 0-8
)
