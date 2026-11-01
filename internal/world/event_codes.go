package world

// EVE namespaces use explicit values from the decoded legacy event data.
// WireCode names mean the action is recognized numerically but remains unported.

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
	ConditionCompanionEquipment = 17
	ConditionMissionVariable    = 19
	ConditionCompanionRebirth   = 21
)

const (
	SubjectBodyOrGender = 3
	SubjectElement      = 4
	SubjectRebornJob    = 5
	GenderMale          = 7
	GenderFemale        = 8
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
	ActorActionAnimationAlternate = 8
	ActorActionPath               = 9
	ActorActionWireCode10         = 10
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
	ServiceClinic             = 6
	ServiceClinicAlternate    = 7
	ServiceStockKeeper        = 9
	ServiceRebornJobSelection = 10
	ServiceCompanionRebirth   = 27
)
