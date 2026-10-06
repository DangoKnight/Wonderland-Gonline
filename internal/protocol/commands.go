package protocol

// Wire command and subcommand values are explicit: changing declaration order must
// never change the bytes sent to the client. Names describe the existing handlers.
// WireCode names preserve values whose full meaning is not established.
const (
	CommandDiscovery          = 0
	CommandHandshake          = 1
	CommandChat               = 2
	CommandMapLoad            = 3
	CommandAppearance         = 4
	CommandCharacterState     = 5
	CommandMovement           = 6
	CommandPosition           = 7
	CommandStats              = 8
	CommandCharacterCreation  = 9
	CommandPresence           = 10
	CommandSocialRelations    = 10 // Inbound AC10; presence packets use the same wire opcode.
	CommandBattleState        = 11
	CommandMapAcknowledgment  = 12
	CommandTeam               = 13
	CommandFriends            = 14
	CommandStoryConstellation = 15 // Outbound star collection, distinct from pet commands.
	CommandPetControl         = 15
	CommandDirectSettings     = 16
	CommandBattlePet          = 19
	CommandEvent              = 20
	CommandScene              = 22
	CommandInventory          = 23
	CommandQuest              = 24
	CommandTrade              = 25
	CommandGold               = 26
	CommandShop               = 27
	CommandLegacyStorage      = 29
	CommandStorage            = 30
	CommandNPCService         = 31
	CommandPose               = 32
	CommandSettings           = 33
	CommandMallCheckout       = 34
	CommandCharacterSelection = 35
	CommandEquipmentRepair    = 36
	CommandAlchemy            = 40 // Legacy four-byte synthesis compatibility.
	CommandHotbar             = 40 // Native AC40:1 hotbar bindings (seven bytes).
	CommandBank               = 45
	CommandBattleAction       = 50
	CommandBattleReady        = 52
	CommandMonsterBook        = 53 // Outbound discovery; AC53:9 carries a template ID.
	CommandBattleEffect       = 53
	CommandDiscoveryChannels  = 54
	CommandMinigame           = 57
	CommandWire62             = 62
	CommandLogin              = 63
	CommandPetFeeding         = 67
	CommandPetTraining        = 68
	CommandPetRebirth         = 69
	CommandTerritory          = 70
	CommandArcadeGame         = 71 // Native paid arcade requests and outcomes (AC71).
	CommandMall               = 75
	CommandSceneReady         = 89
	CommandSceneReadyReply    = 90
	CommandSceneReadyAck      = 92
	CommandPackContents       = 91
	CommandLuckyDraw          = 104
	CommandMovie              = 186
)

// Discovery subcommands (AC0).
const (
	// Native AC0:19 displays "Repeated Login" when the rejected socket closes.
	DiscoveryAlreadyLoggedIn           = 19
	DiscoveryCharacterCreationRejected = 30
	// Unresolved wire meaning; retain the numeric code until verified.
	DiscoveryWireCode32 = 32
)

// Handshake subcommands (AC1).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	HandshakeWireCode3 = 3
	// Unresolved wire meaning; retain the numeric code until verified.
	HandshakeWireCode6         = 6
	HandshakeServerDescription = 9
	HandshakeWorldReady        = 11
)

// Chat subcommands (AC2). Each carries the speaker's ID and the text both
// ways, except that a whisper request carries the target's ID; the client
// shows 2/n on its chat channel n (FUN_0048ac28 via the receive cases at
// 0x2df0b7) and puts 2/16 on the notice board.
const (
	ChatSystemMessage  = 0
	ChatWorldMessage   = 1
	ChatMapMessage     = 2 // the Local channel
	ChatWhisperMessage = 3
	ChatGMMessage      = 4
	ChatTeamMessage    = 5
	ChatGuildMessage   = 6
	ChatAllyMessage    = 7
	ChatHeadBanner     = 16
)

// CharacterState subcommands (AC5).
const (
	CharacterStateEquipmentSnapshot = 0
	CharacterStateRepairEffect      = 5
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode3     = 3
	CharacterStateRefresh       = 4
	CharacterStateSpriteRefresh = 8
	// Character ID followed by native model code (distinct from skill-book AC5:12).
	CharacterStateModelTransform   = 12
	CharacterStateSkillProficiency = 11
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode12 = 12
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode14 = 14
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode15 = 15
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode16 = 16
	CharacterStateRestEffect = 18
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode21 = 21
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode24 = 24
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterStateWireCode30 = 30
)

// Movement subcommands (AC6).
const (
	MovementMove         = 1
	MovementStop         = 2 // Incoming current-position report when native movement stops.
	MovementMovementLock = 2
	// AC6:1 needs command, subcommand, direction and two uint16 coordinates.
	// Native clients append additional movement metadata, ignored by AC06.Recv1.
	MovementRequestMinBytes    = 7
	MovementNativeRequestBytes = 15 // Destination/stop report plus eight native verification bytes.
	MovementDirectionMax       = 7
	// FUN_00427150 recognizes packed pose/facing values in native stop reports.
	MovementStopPoseMax    = 73
	MovementStopWirePose99 = 99 // Recognized native pose; display meaning unresolved.
)

// Stats subcommands (AC8).
const (
	StatsStatUpdate = 1
	// Absolute stat value mode used by Equip.SendStat.
	StatsValueAbsolute = 1
	// Unresolved wire meaning; retain the numeric code until verified.
	StatsWireCode2 = 2
	// Unresolved wire meaning; retain the numeric code until verified.
	StatsWireCode3 = 3
)

// CharacterCreation subcommands (AC9).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	CharacterCreationWireCode1 = 1
	CharacterCreationCreate    = 2
	CharacterCreationCheckName = 3
)

// Presence subcommands (AC10).
const (
	PresenceOnline = 3
)

// BattleState subcommands (AC11).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	BattleStateWireCode0     = 0
	BattleStateExit          = 1
	BattleStateChallenge     = 2
	BattleChallengePlayer    = 3
	BattleStateParticipation = 4
	BattleStateParticipant   = 5
	BattleStateInitialize    = 10
	BattleStateFinish        = 12
	BattleStateFormation     = 250
)

// Team subcommands (AC13).
const (
	TeamRequest            = 1
	TeamReply              = 3
	TeamLeave              = 4
	TeamFormation          = 5
	TeamRoster             = 6
	TeamKick               = 9
	TeamTransferLeadership = 10
)

// Social relation operations (inbound AC10) and status notifications (outbound).
const (
	SocialAddFriend     = 1
	SocialFriendReply   = 2
	SocialFriendList    = 3
	SocialRemoveFriend  = 4
	SocialQueryStatus   = 6
	SocialFriendDecline = 0
	SocialFriendAccept  = 1
	SocialFriendStatus  = 1
	SocialFriendOnline  = 1
)

// Native text mail shares AC14 with friendship operations.
const (
	FriendsTextMail            = 1
	FriendsMailSent            = 9
	FriendsMailSentSuccess     = 0
	TextMailRequestHeaderBytes = 7 // Command, subcommand, type and uint32 recipient.
)

// Friends subcommands (AC14).
const (
	FriendsRequest  = 2
	FriendsAccept   = 3
	FriendsRemove   = 4
	FriendsList     = 5
	FriendsAccepted = 7
	// Unresolved wire meaning; retain the numeric code until verified.
	FriendsWireCode8         = 8
	FriendsAcceptanceReply   = 9
	FriendsAcceptanceSuccess = 0
	FriendsTabs              = 11
	// Unresolved wire meaning; retain the numeric code until verified.
	FriendsWireCode13 = 13
)

// PetControl subcommands (AC15).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	PetControlWireCode1 = 1
	PetControlPetSlot   = 2
	// Unresolved wire meaning; retain the numeric code until verified.
	PetControlWireCode4    = 4
	PetControlRename       = 6
	PetControlBoardVehicle = 7
	// Unresolved wire meaning; retain the numeric code until verified.
	PetControlWireCode8             = 8
	PetControlBoardVehicleAlternate = 9
	PetControlDismountVehicle       = 10
	// Unresolved wire meaning; retain the numeric code until verified.
	PetControlWireCode11      = 11
	PetControlVehicleNoOp     = 13
	PetControlPlaceVehicle    = 14
	PetControlRemoveVehicle   = 15
	PetControlMount           = 16
	PetControlUnmount         = 17
	PetControlVehiclePosition = 18
	// Unresolved wire meaning; retain the numeric code until verified.
	PetControlWireCode20 = 20
)

// BattlePet subcommands (AC19).
const (
	BattlePetSelect          = 1
	BattlePetRest            = 2
	BattlePetSelectAlternate = 4
	BattlePetRestAlternate   = 5
	// Unresolved wire meaning; retain the numeric code until verified.
	BattlePetWireCode7 = 7
)

// Event subcommands (AC20).
const (
	EventActorClick   = 1
	EventRegion       = 4
	EventAcknowledge  = 6
	EventClose        = 7
	EventResume       = 8
	EventChoice       = 9
	EventStepComplete = 10
	EventBattleBegin  = 12
	// Unresolved wire meaning; retain the numeric code until verified.
	EventWireCode33 = 33
)

// Scene subcommands (AC22).
const (
	SceneActorState    = 1
	SceneActorWalk     = 2 // Ambient NPC movement, distinct from scripted AC22:11 paths.
	SceneActorPosition = 4
	// Unresolved wire meaning; retain the numeric code until verified.
	SceneWireCode7 = 7
	// Unresolved wire meaning; retain the numeric code until verified.
	SceneWireCode8 = 8
	// Unresolved wire meaning; retain the numeric code until verified.
	SceneWireCode9     = 9
	SceneActorHide     = 10 // Explicit actor despawn; native S.Monkey rescue uses FFFF flags.
	SceneActorMovement = 11
	SceneActorAction   = 12
)

// Inventory subcommands (AC23).
const (
	InventoryPickup           = 2
	InventoryDrop             = 3
	InventoryGroundItems      = 4
	InventoryItems            = 5
	InventoryCompoundResult   = 8
	InventoryRemove           = 9
	InventoryMove             = 10
	InventoryEquip            = 11
	InventoryUnequip          = 12
	InventoryCompoundSuccess  = 13
	InventoryCompound         = 14
	InventoryItemUse          = 15
	InventoryEquipmentChanged = 16
	InventoryPetEquip         = 17
	InventoryPetUnequip       = 18
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode22 = 22
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode23 = 23
	InventoryDropped    = 26
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode32 = 32
	InventoryMessage    = 57
	InventoryOpenPack   = 75
	// AC23.Recv77 requests the player-stall list, including during native login.
	InventoryStallListRequest  = 77
	InventoryStallList         = 4
	InventoryStallListComplete = 102
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode76        = 76
	InventoryUse               = 96
	InventoryCompoundAnimation = 122
	InventoryFishingStopped    = 122
	InventoryFishingStarted    = 123
	InventoryAcquisitionNotice = 51
	InventorySceneComplete     = 102
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode112 = 112
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode122         = 122
	InventoryDestroy             = 124
	InventoryOpenPackAlternate   = 128
	InventoryPotentialPill       = 126
	InventoryPotentialPillResult = 213
	PotentialPillRequestBytes    = 4
	PotentialPillResultBytes     = 5
	PotentialPillRejected        = 0
	PotentialPillProcessed       = 1
	PotentialPillPlayerTarget    = 0
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode132 = 132
	InventorySceneBegin  = 138
	// Unresolved wire meaning; retain the numeric code until verified.
	InventoryWireCode212 = 212
)

// Quest subcommands (AC24).
const (
	QuestMarkState   = 1
	QuestMark        = 4
	QuestFlag        = 5
	QuestActiveMarks = 6
	// Unresolved wire meaning; retain the numeric code until verified.
	QuestWireCode7 = 7
)

// Trade subcommands (AC25).
const (
	TradeRequest   = 1
	TradeReply     = 2
	TradeItemOffer = 3
	TradeGoldOffer = 4
	TradeOfferNoOp = 5
	TradeConfirm   = 6
	TradeCancel    = 7
)

// Bank/ATM operations (AC45). PIN and inter-character transfer remain unavailable.
const (
	BankBalance             = 8
	BankDeposit             = 9
	BankWithdraw            = 10
	BankSetPIN              = 11
	BankTransfer            = 12
	BankBalanceRequestBytes = 2
	BankAmountRequestBytes  = 6
)

// Gold subcommands (AC26).
const (
	GoldBalance = 4
)

// Shop subcommands (AC27).
const (
	ShopSell            = 2
	ShopPropsWindow     = 3
	ShopEquipmentWindow = 4
)

// LegacyStorage subcommands (AC29).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	LegacyStorageWireCode6 = 6
)

// Storage subcommands (AC30).
const (
	StorageItems            = 1
	StorageDepositComplete  = 6
	StorageWithdrawComplete = 7
	StorageOpen             = 8
)

// NPCService subcommands (AC31).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode1 = 1
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode2 = 2
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode3 = 3
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode4 = 4
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode6 = 6
	// Unresolved wire meaning; retain the numeric code until verified.
	NPCServiceWireCode7 = 7
)

// Pose subcommands (AC32).
const (
	PoseBroadcast = 2
)

// Settings subcommands (AC33).
const (
	SettingsPK           = 1 // C->S: desired PK permission.
	SettingsError        = 1 // S->C: rejection reason.
	SettingsJoinBattle   = 2 // C->S with value; no value requests a snapshot.
	SettingsSnapshot     = 2
	SettingsTrade        = 4
	SettingsPartyInvites = 5
	SettingsChannels     = 3
)

// CharacterSelection subcommands (AC35).
const (
	CharacterSelectionDelete      = 2
	CharacterSelectionReady       = 11
	CharacterSelectionRecordPoint = 12
)

// BattleAction subcommands (AC50).
const (
	BattleActionAnimation = 1
	BattleActionTurn      = 6
)

// BattleReady subcommands (AC52).
const (
	BattleReadyReady = 1
)

// BattleEffect subcommands (AC53).
const (
	BattleEffectDefeated = 3
	BattleEffectEscape   = 5
)

// DiscoveryChannels subcommands (AC54).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	DiscoveryChannelsWireCode201 = 201
)

// Minigame subcommands (AC57).
const (
	MinigameStart  = 1
	MinigameResult = 1
	MinigameEnd    = 2
)

// Wire62 subcommands (AC62).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	Wire62WireCode53 = 53
)

// Login subcommands (AC63).
const (
	LoginReturn          = 0
	LoginRoster          = 1
	LoginSelectAlternate = 2
	// LoginCancelCreation is sent when character creation is cancelled on
	// its first step (client send case 0x2d8dcd).
	LoginCancelCreation = 3
	LoginAuthenticate   = 4
)

// Territory subcommands (AC70).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	TerritoryWireCode1 = 1
)

// Wire75 subcommands (AC75).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	MallSettings = 8
)

// Movie subcommands (AC186).
const (
	// Unresolved wire meaning; retain the numeric code until verified.
	MovieWireCode12 = 12
)

// Character state menu requests (AC5).
const (
	CharacterStateRefreshRequest = 7
	CharacterStateTeleport       = 17
)

// Direct setting commands (AC16).
const (
	DirectSettingsPK         = 1
	DirectSettingsTradeBlock = 2
	DirectSettingsJoinBlock  = 3
	DirectSettingsWalkMode   = 4
)

// AC15:10 has different meanings by direction.
const PetControlVehicleMount = 10

// AC25:2 reply values.
const (
	TradeReplyAccepted  = 1
	TradeReplyRejected  = 2
	TradeReplyCompleted = 4
)

// Direction-specific or context-specific requests and responses.
const (
	MapAcknowledgmentMapLoaded = 1
	EventFrame                 = 1
	EventPortal                = 8
	EventHold                  = 9
	PoseEmote                  = 1         // Temporary expression; independent of the held AC32:2 pose.
	PoseSet                    = PoseEmote // Historical name retained for compatibility.
	PoseStop                   = 3
	ClinicRestConfirm          = 1
	ClinicRestOffer            = 2
	PetHotelWithdraw           = 2
	PetHotelDeposit            = 3
	PetHotelTransfer           = 4
	PetHotelOpen               = 7
	StorageWithdraw            = 1
	StorageDeposit             = 2
	StorageReorder             = 3
	StorageDepositToSlot       = 4
	StorageWithdrawToSlot      = 5
)

// Native cart balance handshake (AC34:1 -> AC35:4).
const (
	MallCheckoutBalanceRequest    = 1
	CharacterSelectionMallBalance = 4
	MallCheckoutRequestBytes      = 3
	MallCheckoutReservedBytes     = 8
	// The reference accepts both modes but always resumes with ordinary IM points.
	// Their separate native meanings remain unresolved.
	MallCheckoutModeWireCode0 = 0
	MallCheckoutModeWireCode1 = 1
)

// Item mall requests and responses use distinct names when directions differ.
const (
	InventoryMallBalance   = 25
	InventoryMallBuy       = 26
	InventoryFishingStart  = 53
	InventoryFishingStop   = 54 // Same request code as mall refresh; disambiguated by active cast.
	InventoryMallCatalog   = 54
	MallBuyPoints          = 1
	MallPointsCatalog      = 1
	MallRefresh            = 2
	MallPointsBalance      = 3
	MallForge              = 3
	MallGameCategory       = 4
	MallCartReceipt        = 4
	MallBuyBonus           = 5
	MallStatus             = 7
	MallBonusBalance       = 9
	MallBonusCatalog       = 10
	MallPurchaseFailed     = 0
	MallPurchaseSucceeded  = 1
	MallCartMaxRows        = 7
	MallCartRowBytes       = 6
	MallCatalogRecordBytes = 10
	MallNoDiscount         = 100
	MallBadgeNew           = 1
	MallBadgeHot           = 2
	MallBadgeLimited       = 3
	MallCategoryHot        = 1
	MallCategoryArmory     = 2
	MallCategoryWeaponry   = 3
	MallCategoryGrocery    = 4
	MallCategoryFurniture  = 5
	MallCategoryGames      = 6
	MallCategoryForging    = 7
)

const MallCatalogHeaderBytes = 4
const MallStatusEnabled = 1

// Missing reference-native request envelopes. Request and response opcodes may
// differ; unresolved matrix rows retain explicit compatibility names.
const (
	CommandGesture                = 18
	CommandNativeMall             = 21
	CommandTitle                  = 44
	CommandRebornJob              = 66
	CommandBath                   = 87
	CommandFishing                = 90 // Inbound; outbound AC90 also acknowledges scene readiness.
	CommandHeartbeatSync          = 183
	CommandMallClaimState         = 225
	CommandMallMatrixRequest      = 226
	CommandMallMatrix             = 238
	TeamMallRefresh               = 238
	TeamMallConfirmed             = 42
	NativeMallWindow              = 1
	NativeMallBuy                 = 2
	NativeMallBalance             = 3
	NativeMallWindowSlots         = 21
	HeartbeatSyncRequest          = 17
	HeartbeatSyncStatus           = 11
	HeartbeatSyncStatusWireValue9 = 9
	HeartbeatSyncStatusWireValue2 = 2
	MoviePlaybackAcknowledgment   = 9
	NativeDefaultEntityID         = 0
	NativeDefaultGestureID        = 1
	NativeDefaultMovieID          = 1
	NativeActive                  = 1
	MallMatrixQuery               = 255
	MallMatrixRows                = 183
	MallMatrixRowWireID27         = 27
	MallMatrixRowWireID29         = 29
	MallMatrixRowWireID24         = 24
	MallClaimSnapshot             = 252
	MallClaimReservedBytes        = 10
	TitleBroadcast                = 1
	FishingStart                  = 1
	FishingCatch                  = 2
	FishingStop                   = 3
	NativeInactive                = 0
)

const NativeDefaultRebornJob = 1
const RebornJobMetadata = 11

// LoginSelect is retained for older callers; AC63:0 returns to the account form.
// Deprecated: use LoginReturn. Character selection uses LoginSelectAlternate.
const LoginSelect = LoginReturn

const (
	LoginClientVersionMin     = 1000
	LoginClientVersionMax     = 2000
	LoginReturnPacketBytes    = 2
	LoginSelectionPacketBytes = 3
	LoginCancelPacketBytes    = 2
)

// AC23:14 supports precisely two slot operands. The result packet includes the
// reference's 28 reserved zero bytes, distinct from ordinary inventory metadata.
const (
	CompoundIngredientSlots     = 2
	CompoundRequestBytes        = 5
	CompoundResultReservedBytes = 28
)

// Native AC91 previews use a cache hint; AC23:75/128 carry a UInt16 bag slot.
const (
	PackContentsRequest      = 1
	PackContentsResponse     = 2
	PackContentsVersion      = 1
	PackContentsRequestBytes = 5
	PackOpenRequestBytes     = 4
)

// Equipment repair (AC36). The reference echoes the caller's subcommand.
const (
	RepairRequestBytes = 3
	RepairSucceeded    = 1
	RepairFailed       = 0
)

// Legacy AC40 echoes the request subcommand; its handler defines no distinct
// subcommand operations. The payload names two one-based bag slots.
const (
	AlchemyRequestBytes = 4
	AlchemyFailed       = 0
	AlchemySucceeded    = 1
)

// Native hotbar assignment: command, subcommand, kind, ID:u16, page, slot.
// FUN_002aa92c sends it after updating the local hotbar. There is no required
// acknowledgement; inbound AC40:1 is a list of the same five-byte records.
const (
	HotbarAssign       = 1
	HotbarRequestBytes = 7
	HotbarItem         = 1
	HotbarSkill        = 2
	HotbarPages        = 3
	HotbarSlotsPerPage = 8
	HotbarKindOffset   = 2
	HotbarIDOffset     = 3
	HotbarPageOffset   = 5
	HotbarSlotOffset   = 6
)

// Native AC75:6 results from MallForgingManager.
const (
	MallForgeResult              = 6
	MallForgeProgressSucceeded   = 1
	MallForgeRollFailed          = 2
	MallForgeInsufficientPoints  = 4
	MallForgeInsufficientScrolls = 5
	MallForgeUpgradeSucceeded    = 6
	MallForgeRejected            = 8
	MallForgeRequestBytes        = 3
)

// AC75:4 opens the chosen arcade category with the legacy zero seed.
const (
	MallGameRequestBytes = 3
	MallGameSeed         = 0
)

// Battle target result precedes the guard flag in AC50:1 hit records.
// Native client FUN_00398dac stores it at target-record offset 6;
// FUN_003990ac treats 0 as miss and 1 as landed. This is separate from
// the final digit mode byte (where 2 means critical damage).
const (
	BattleHitLanded = 1
	BattleHitMiss   = 0
)

// Companion rebirth (AC69).
const (
	PetRebirthAscend       = 1
	PetRebirthFailed       = 0
	PetRebirthSucceeded    = 1
	PetRebirthRequestBytes = 3
)

// Companion training (AC68). Selectors are distinct from AC8 stat identifiers.
const (
	PetTrainingPotential    = 1
	PetTrainingAllocate     = 2
	PetTrainingStrength     = 1
	PetTrainingConstitution = 2
	PetTrainingIntelligence = 3
	PetTrainingWisdom       = 4
	PetTrainingAgility      = 5
	PetTrainingFailed       = 0
	PetTrainingSucceeded    = 1
	PetTrainingRequestBytes = 4
)

// Battle-pet feeding by item ID (AC67).
const (
	PetFeedingFeed         = 1
	PetFeedingFailed       = 0
	PetFeedingSucceeded    = 1
	PetFeedingRequestBytes = 4
)

// Daily Lucky Draw. Outbound AC104:1 carries a nested catalog/result kind.
const (
	LuckyDrawSpin         = 1
	LuckyDrawMode         = 1
	LuckyDrawCatalog      = 1
	LuckyDrawResult       = 2
	LuckyDrawRequestBytes = 2
)

// Native login completion and notebook synchronization.
const (
	SceneReadyLoaded                = 0
	SceneReadyStatus                = 1
	SceneReadyAcknowledged          = 1
	SceneReadyShortRequestBytes     = 2
	SceneReadyNativeRequestBytes    = 6
	SceneReadyAckRequestBytes       = 2
	SceneReadyNativeAckRequestBytes = 3
	MonsterBookDiscover             = 9
	StoryConstellationList          = 19
)

// PalaceTrial preserves the AC77 request/reply envelope.
const (
	CommandPalaceTrial        = 77
	PalaceTrialChallenge      = 1
	PalaceTrialRejected  byte = 0
	PalaceTrialAccepted  byte = 1
)

// Native player stalls (AC56).
const (
	CommandStall = 56
	StallOpen    = 1
	StallClose   = 2
	StallView    = 3
	StallBuy     = 4
	StallSign    = 30
)

// Guild gameplay (AC39), distinct from inbound chat AC2:6.
const (
	CommandGuild    = 39
	GuildJournal    = 1
	GuildInvite     = 2
	GuildAccept     = 3
	GuildLeave      = 6
	GuildDismiss    = 7
	GuildMessage    = 8
	GuildRoster     = 8
	GuildNotice     = 9
	GuildDemote     = 11
	GuildMemberList = 12
	GuildPromote    = 14
	GuildPermission = 16
	GuildInsignia   = 18
	GuildBadge      = 30
)

// Outbound legacy parcel mailbox; inbound AC23:77 remains stall synchronization.
const (
	InventoryParcelList    = 76
	InventoryParcelDetails = 77
)

// Native manufacturing (AC59). The reference wire handler selects Forge.
const (
	CommandGemSocket            = 37
	GemSocketRequestBytes       = 4
	CommandTentManufacture      = 64
	TentManufactureStart        = 1
	TentManufactureContinue     = 2
	TentManufactureStop         = 3
	TentManufactureStopped      = 4
	TentManufactureBagComplete  = 9
	TentManufactureStartBytes   = 5
	TentManufactureControlBytes = 3
	CommandManufacture          = 59
	ManufactureRequestBytes     = 8
	ManufactureFailed           = 0
	ManufactureSucceeded        = 1
)

// Fixed Economy/Social inbound payload sizes include command and subcommand.
const (
	StallHeaderRequestBytes     = 2
	StallTargetRequestBytes     = 6
	StallPurchaseRequestBytes   = 8
	GuildHeaderRequestBytes     = 2
	GuildTargetRequestBytes     = 6
	GuildPermissionRequestBytes = 7
)

// Player homes: native interior template plus owner-scoped server instances.
const (
	CommandTent                 = 65
	CommandTentFurniture        = 62
	TentEnter                   = 1
	TentClose                   = 2
	TentExit                    = 3
	TentClosed                  = 4
	TentInteriorStatus          = 7
	TentOpened                  = 59
	TentOpenedStatus            = 2
	TentPlaceFurniture          = 1
	TentMoveFurniture           = 3
	TentFurnitureSucceeded      = 1
	InventoryTentFurniture      = 3
	PresenceMapAvailable        = 255
	InventoryTentPlayers        = 138
	InventoryTentPlayer         = 122
	InventoryTentPlayerComplete = 76
	TentHeaderRequestBytes      = 2
	TentEnterRequestBytes       = 6
	TentPlaceRequestBytes       = 16
	TentMoveRequestBytes        = 16
	TentMoveRotatedRequestBytes = 17
	TentCoordinateMax           = 65535
)

// Native AC10 profile operations share codes with legacy contact adapters.
const (
	SocialNicknameUpdate      = 1
	SocialProfileUpdate       = 2
	SocialProfileRequestBytes = 7
	SocialNicknameMaxBytes    = 14 // Native fixed nickname buffer in AC3/AC10 receive handlers.
)
