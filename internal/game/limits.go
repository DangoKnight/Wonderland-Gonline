package game

import "math"

// Native game limits and record sizes; these are independent of numeric wire commands.
const (
	MaxTentToolAttachments        = 10
	NotepadItemID          uint16 = 34038
	RemoteControlItemID    uint16 = 34058
	ItemTypeAppliance      byte   = 29
	ItemTypeTentDecoration byte   = 30
	MaxGold                       = 999_999
	MaxBankGold                   = math.MaxUint32 // Native AC45 balance range; no lower reference cap is established.
	MaxItemStack                  = 50
	ItemMetadataBytes             = 26
	StatPointsPerLevel            = 3
	SkillProficiencyScale         = 10_000
	SkillGradeExpScale            = 100
	MinSkillGrade                 = 1
	DefaultSkillGrade             = 10 // Compatibility limit without catalog metadata.
	StarterStuntClientID          = 15003
	VehicleWreckDamage            = 100
	RaftItemID                    = 48010
	DisposableRaftItemID          = 48016
)

// Account mall balances use the legacy signed 32-bit point range.
const MaxMallPoints = math.MaxInt32

// Character rebirth and jobs remain unported; roster fields explicitly say none.
const (
	RosterNotReborn           = 0
	RosterNoJob               = 0
	CharacterRosterFixedBytes = 54
)

const MaxNativeGuildID = 65535
