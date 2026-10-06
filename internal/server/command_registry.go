package server

import (
	"context"
	"time"
	"wonderland-gonline/internal/protocol"
)

type commandHandler func(*Server, context.Context, *Session, []byte) error

type commandRegistration struct {
	handler          commandHandler
	policy           protocol.CommandPolicy
	beforeWorldGates func([]byte) bool
}

func (command commandRegistration) handle(s *Server, ctx context.Context, c *Session, p []byte) error {
	if command.handler == nil || (command.policy.RequiresBattle && c.battle == nil) {
		return ErrUnsupported
	}
	return command.handler(s, ctx, c, p)
}

func withoutContext(handler func(*Server, *Session, []byte) error) commandHandler {
	return func(s *Server, _ context.Context, c *Session, p []byte) error { return handler(s, c, p) }
}

// commandRegistry is immutable after initialization. Register each inbound command
// once, together with its handler and exceptions to the default interaction gates.
var commandRegistry = map[byte]commandRegistration{
	protocol.CommandAppearance:         {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandPosition:           {handler: (*Server).waypointCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandGesture:            {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandScene:              {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandHeartbeatSync:      {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandMovie:              {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandMallMatrixRequest:  {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandNativeMall:         {handler: (*Server).nativeMallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.NativeMallWindow, protocol.NativeMallBalance)},
	protocol.CommandTitle:              {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandRebornJob:          {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandBath:               {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandFishing:            {handler: (*Server).nativeCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTent:               {handler: (*Server).tentCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTentFurniture:      {handler: (*Server).tentFurnitureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandTentManufacture:    {handler: (*Server).tentManufactureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandGemSocket:          {handler: (*Server).gemSocketCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandManufacture:        {handler: (*Server).manufactureCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandGuild:              {handler: (*Server).guildCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandStall:              {handler: (*Server).stallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandSceneReady:         {handler: withoutContext((*Server).sceneReadyCommand), policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandSceneReadyAck:      {handler: withoutContext((*Server).sceneReadyCommand), policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandLuckyDraw:          {handler: (*Server).luckyDrawCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandDiscovery:          {handler: (*Server).discoveryCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandHandshake:          {handler: (*Server).handshakeCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandLogin:              {handler: (*Server).login, policy: protocol.CommandPolicy{}},
	protocol.CommandTeam:               {handler: (*Server).partyCommand, policy: protocol.CommandPolicy{}},
	protocol.CommandCharacterCreation:  {handler: (*Server).createCharacter, policy: protocol.CommandPolicy{}},
	protocol.CommandCharacterSelection: {handler: (*Server).deleteCharacter, policy: protocol.CommandPolicy{CharacterDeletion: true}},
	protocol.CommandChat:               {handler: (*Server).chat, policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true, AllowedDuringMinigame: true}},
	protocol.CommandCharacterState:     {handler: (*Server).menuCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.CharacterStateRefreshRequest, protocol.CharacterStateRefresh)},
	protocol.CommandMovement:           {handler: (*Server).movementCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandStats:              {handler: (*Server).allocateStats, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPalaceTrial:        {handler: withoutContext((*Server).palaceTrialCommand), policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandBattleState:        {handler: withoutContext((*Server).battleStateCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true}},
	protocol.CommandMapAcknowledgment:  {handler: (*Server).mapAcknowledgmentCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandSocialRelations:    {handler: (*Server).socialCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandFriends:            {handler: (*Server).friendCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandPetControl:         {handler: (*Server).petControlCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandDirectSettings:     {handler: (*Server).settingsCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandBattlePet:          {handler: (*Server).petCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandEvent:              {handler: (*Server).eventCommand, policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true, BlockedDuringTrade: true}},
	protocol.CommandInventory:          {handler: (*Server).itemCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.InventoryStallListRequest, protocol.InventoryMallCatalog, protocol.InventoryMallBalance)},
	protocol.CommandTrade:              {handler: (*Server).tradeCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandShop:               {handler: (*Server).sellItems, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandStorage:            {handler: (*Server).storageCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandNPCService:         {handler: (*Server).npcServiceCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPose:               {handler: withoutContext((*Server).emote), policy: protocol.CommandPolicy{World: true}, beforeWorldGates: matchingSubcommands(protocol.PoseStop)},
	protocol.CommandSettings:           {handler: (*Server).settingsCommand, policy: protocol.CommandPolicy{World: true, BeforeWorldGates: true}},
	protocol.CommandBattleAction:       {handler: withoutContext((*Server).battleCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringBattle: true, RequiresBattle: true}},
	protocol.CommandMinigame:           {handler: withoutContext((*Server).minigameCommand), policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true}},
	protocol.CommandArcadeGame:         {handler: (*Server).arcadeCommand, policy: protocol.CommandPolicy{World: true, AllowedDuringMinigame: true, BlockedDuringTrade: true}},
	protocol.CommandMallCheckout:       {handler: (*Server).mallCheckoutCommand, policy: protocol.CommandPolicy{World: true}},
	protocol.CommandMall:               {handler: (*Server).mallCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}, beforeWorldGates: matchingSubcommands(protocol.MallRefresh)},
	protocol.CommandPackContents:       {handler: withoutContext((*Server).packContentsCommand), policy: protocol.CommandPolicy{World: true}},
	protocol.CommandEquipmentRepair:    {handler: (*Server).repairCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandBank:               {handler: (*Server).bankCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandAlchemy:            {handler: (*Server).hotbarOrAlchemyCommand, beforeWorldGates: isNativeHotbarAssignment, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetFeeding:         {handler: (*Server).petFeedingCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetTraining:        {handler: (*Server).petTrainingCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
	protocol.CommandPetRebirth:         {handler: (*Server).petRebirthCommand, policy: protocol.CommandPolicy{World: true, BlockedDuringTrade: true}},
}

// These read-only native synchronization requests also run while loading or in
// an interaction. Matching a subcommand never bypasses its handler's validation.
func matchingSubcommands(subcommands ...byte) func([]byte) bool {
	return func(p []byte) bool {
		if len(p) < 2 {
			return false
		}
		for _, sub := range subcommands {
			if p[1] == sub {
				return true
			}
		}
		return false
	}
}

func (s *Server) discoveryCommand(_ context.Context, c *Session, _ []byte) error {
	if e := c.send(append([]byte{protocol.CommandHandshake, protocol.HandshakeServerDescription, 101, 0, 1}, []byte(s.Name())...)); e != nil {
		return e
	}
	return c.send([]byte{protocol.CommandDiscoveryChannels, protocol.DiscoveryChannelsWireCode201, 0, 1, 101, 0, 3, 103, 0, 2, 104, 0, 3, 102, 0, 3})
}

func (s *Server) handshakeCommand(_ context.Context, c *Session, p []byte) error {
	sub := byte(1)
	if len(p) > 1 {
		sub = p[1]
	}
	out := protocol.Builder{protocol.CommandHandshake, sub, 1}
	if sub == 1 {
		out = out.U32(uint32(time.Since(s.Started).Milliseconds()))
	}
	return c.send(out)
}

func (s *Server) mapAcknowledgmentCommand(_ context.Context, c *Session, p []byte) error {
	if len(p) != 2 || p[1] != protocol.MapAcknowledgmentMapLoaded {
		return protocol.ErrMalformed
	}
	return s.acknowledgeWorld(c)
}

func (s *Server) npcServiceCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) == 2 && p[1] == protocol.ClinicRestConfirm {
		return s.confirmRest(ctx, c)
	}
	return s.hotelCommand(ctx, c, p)
}
