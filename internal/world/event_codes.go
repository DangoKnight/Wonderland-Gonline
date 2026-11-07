package world

// EVE namespaces use explicit values from the decoded legacy event data.
// WireCode names mean the action is recognized numerically but remains unported.

// Compatibility headers for stationary field actors. Moving field combat actors
// are identified from their movement and battle operation, regardless of header.
// The precise trigger distinction between kinds 8 and 10 remains unresolved.
const (
	EventKindFieldBattleCompatibility8 = 8
	EventKindFieldBattle               = 10
	ActorFlagFieldEncounter            = 5
	BattleActionStart                  = 1
)

// Action codes (DialogPtr).
const (
	ActionPlayer            = 1
	ActionActor             = 2
	ActionCompanion         = 3
	ActionBattle            = 4
	ActionQuestMark         = 5
	ActionBattleAlternate   = 6
	ActionServiceOrTeleport = 7
	ActionMovie             = 8
	ActionMinigame          = 9
	ActionWireCode10        = 10
	ActionLearnSkill        = 11
	ActionTransform         = 12
	ActionMinimapMarker     = 13
	ActionGather            = 14
	ActionEffect            = 15
	ActionWireCode16        = 16
	ActionWireCode17        = 17
)

// Action 7 operands below this boundary identify NPC services, not maps.
const ServiceTeleportMinimumMapID = 1000

const (
	MovieActionPlay          = 1
	MovieActionPlayAlternate = 2
)

// Mission operations have no client presentation of their own.
const (
	ActionMissionMarker   = 10
	ActionMissionVariable = 17
	MissionVariableAdd    = 1
	MissionVariableAbsent = 2
)

// Branch condition and callback kinds.
const (
	ConditionAlways             = 0
	ConditionEmptyOperands      = 1
	ConditionSubject            = 2
	ConditionProp               = 3
	ConditionBattleResult       = 4
	ConditionQuest              = 5
	ConditionAlwaysAlternate    = 6
	ConditionChoiceResult       = 7
	ConditionMinigameResult     = 8
	ConditionSkillGrade         = 10
	ConditionTeamSize           = 13
	ConditionWaterGathering     = 14
	ConditionFreeBagSlots       = 15
	ConditionPlayerEquipment    = 16
	ConditionCompanionEquipment = 17
	ConditionMissionVariable    = 19
	ConditionCompanionRebirth   = 21
)

// EVE condition 16 selectors are one higher than the native equipment slot:
// 2=head, 3=body, 4=hand, 5=arm, 6=feet, 7=special.
const (
	PlayerEquipmentFirstSelector = 2
	PlayerEquipmentLastSelector  = 7
)

// EVE condition 15 becomes native inventory condition 21. Its selector
// distinguishes a free-cell count from positive/negative item-capacity gates.
const (
	BagCapacityFreeCells     = 1
	BagCapacityCanReceive    = 2
	BagCapacityCannotReceive = 3
)

const (
	SubjectBodyOrGender = 3
	SubjectElement      = 4
	SubjectRebornJob    = 5
	GenderMale          = 7
	GenderFemale        = 8
)

// Other scopes of these legacy companion-condition selectors remain unresolved.
const (
	CompanionConditionPrisonPresence = 3
	CompanionConditionCaptureAbsence = 4
)

// Condition comparison operators.
const (
	CompareLess           = 1
	CompareGreater        = 2
	CompareLessOrEqual    = 3
	CompareGreaterOrEqual = 4
	CompareEqual          = 5
	CompareNotEqual       = 6
)

// Region kinds are independent of branch condition kinds.
const (
	RegionEntry          = 1
	RegionDoor           = 2
	RegionArea           = 3
	TriggerEntry         = 0
	MinDialogueID        = 10_000
	MaxTalkIDHighByte    = 255
	MaxDialogueWordID    = 65_000
	MinMonsterTemplateID = 10_000
)

// ActionPlayer D1 operands.
const (
	PlayerActionReward          = 1
	PlayerActionSpeech          = 2
	PlayerActionSceneTransition = 3
	PlayerActionChoice          = 4
	PlayerActionRecordPoint     = 5
	PlayerActionAnimation       = 6
	PlayerActionEffect          = 7
)

// EVE player-effect expressions use the same E101..E124 strips as AC32:1.
const (
	PlayerEffectExpressionMode     = 1
	PlayerExpressionOffset         = 100
	PlayerExpressionMaximumOperand = 24
)

// Reward kinds and quest checkpoint modes are separate EVE operand namespaces.
const (
	PlayerRewardItem              = 1
	PlayerRewardItemCompatibility = 7 // Native item-delivery alternate accepted by the existing interpreter.
	PlayerRewardGold              = 2
	PlayerRewardExperience        = 5
	PlayerExperienceValue         = 1
	QuestMarkAdvance              = 1
	QuestMarkComplete             = 2
)

// Companion actions 3/4 are temporary quest escorts. They require an authored
// quest_companion policy; recognizing these operands does not enable them globally.
const (
	CompanionRecruit                = 1
	CompanionDismiss                = 2
	CompanionQuestJoin              = 3
	CompanionQuestLeave             = 4
	CompanionQuestJoinCompatibility = 1
	ActorVisibilityCompatibility    = 65535
)

// EVE transformation modes distinguish permanent character conversion from a
// temporary NPC shape. Only SQL-scoped disguise actions are enabled here.
const (
	TransformNPCDisguise  = 2
	TransformRestoreShape = 3
)

// ActionActor D2 operands.
const (
	ActorActionSpeech             = 0
	ActorActionSpeechAlternate    = 1
	ActorActionHide               = 2
	ActorActionShow               = 3
	ActorActionAnimation          = 4
	ActorActionPropState          = 5
	ActorActionChoice             = 6
	ActorActionPose               = 7
	ActorActionExpression         = 8
	ActorActionAnimationAlternate = ActorActionExpression // Retained compatibility name.
	ActorActionPath               = 9
	ActorActionTurn               = 10
	ActorActionWireCode10         = ActorActionTurn // Retained compatibility name.
	ActorActionConstellation      = 11
)

// EVE entry rectangles use cells of this many pixels.
const RegionCellPixels = 20

// Service operands for ActionServiceOrTeleport (D1 below the map ID range).
const (
	ServiceWeaponShop         = 1
	ServicePropsShop          = 2
	ServiceArmorShop          = 3
	ServicePropsKeeper        = 4
	ServicePetHotel           = 5
	ServiceBank               = 6
	ServiceClinic             = 7
	ServiceStockKeeper        = 9
	ServiceRebornJobSelection = 10
	ServiceCompanionRebirth   = 27
)
