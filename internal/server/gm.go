package server

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/world"
)

type gmCommandDefinition struct {
	aliases []string
	usage   string
	idle    bool
	handle  func(*Server, context.Context, *Session, string, []string) error
}

// Every alias shares the same ownership policy and handler. The usage list also
// drives /help so it cannot advertise commands without implementations.
var gmCommandDefinitions = []gmCommandDefinition{
	{[]string{"packetdebug", "debugpkt"}, "packetdebug [on|off]", false, (*Server).gmPacketDebug},
	{[]string{"bazaar"}, "bazaar list | create <item|pet|vehicle> <finite|infinite> <ID:price:stock,...> <title> | remove <ID>", true, (*Server).gmBazaar},
	{[]string{"reborn"}, "reborn <Killer|Warrior|Knight|Wit|Priest|Seer>", true, (*Server).gmReborn},
	{[]string{"palace"}, "palace <stage>", true, (*Server).gmPalace},
	{[]string{"exprate", "experience"}, "exprate [multiplier]", false, (*Server).gmExpRate},
	{[]string{"allskills", "maxskills"}, "allskills [grade]", true, (*Server).gmPlayerEdit},
	{[]string{"god", "godmode"}, "god", true, (*Server).gmPlayerEdit},
	{[]string{"tent"}, "tent", true, (*Server).gmInventory},
	{[]string{"clearinv"}, "clearinv", true, (*Server).gmInventory},
	{[]string{"buy"}, "buy <item ID or name> [count]", true, (*Server).gmInventory},
	{[]string{"pet"}, "pet <template ID> [name]", true, (*Server).gmPetEdit},
	{[]string{"amity", "petamity"}, "amity [0-100]", true, (*Server).gmPetEdit},
	{[]string{"rebirth", "petrebirth"}, "rebirth", true, (*Server).gmPetEdit},
	{[]string{"petlvl", "petlevel"}, "petlvl <level>", true, (*Server).gmPetEdit},
	{[]string{"petexp"}, "petexp <EXP gain>", true, (*Server).gmPetEdit},
	{[]string{"invis", "invisible", "ghost", "hide", "unhide"}, "invis | hide | unhide", true, (*Server).gmVisibility},
	{[]string{"mute", "unmute"}, "mute <character> [minutes] | unmute <character>", false, (*Server).gmModeration},
	{[]string{"jail", "unjail"}, "jail <character> [minutes] | unjail <character>", true, (*Server).gmModeration},
	{[]string{"online", "who"}, "online", false, (*Server).gmInformation},
	{[]string{"info", "whois"}, "info [character]", false, (*Server).gmInformation},
	{[]string{"kickall"}, "kickall [reason]", false, (*Server).gmOperations},
	{[]string{"reload"}, "reload [all|quests|mall|drops|gms]", false, (*Server).gmReload},
	{[]string{"shutdown"}, "shutdown [1-300 seconds]", false, (*Server).gmOperations},
	{[]string{"battle", "fight"}, "battle <monster template ID>", true, (*Server).gmCombat},
	{[]string{"killall", "killmonsters", "winbattle"}, "winbattle", false, (*Server).gmCombat},
}

var gmCommandRegistry = func() map[string]gmCommandDefinition {
	out := map[string]gmCommandDefinition{}
	for _, command := range gmCommandDefinitions {
		for _, name := range command.aliases {
			if _, exists := out[name]; exists {
				panic("duplicate GM command: " + name)
			}
			out[name] = command
		}
	}
	return out
}()

var existingGMCommands = []string{"heal", "hp", "full", "gold", "money", "item", "warp", "goto", "tp", "summon", "bring", "kick", "b", "broadcast", "notice", "town", "summonall", "level", "lvl", "points", "sp", "statpoint", "statpoints", "stats", "stat", "exp", "skill", "restat", "resetstats", "repair", "fixall", "droprate", "im", "points_im", "mallpoints", "clearskills", "resetskills"}

func (s *Server) commandHelp(c *Session) error {
	if c.gmLevel.Load() == 0 {
		for _, line := range []string{
			"Commands: /help, /unride, /dismount, /carnie, /world, /team, /whisper, /guild",
			"/guildcreate <name>; /marry <character>; /acceptmarry; /declinemarry; /divorce; /warptospouse",
			"/mail; /readmail <ID>; /claimmail <ID>; /deletemail <ID>",
			"/sendmail <ID> <gold> <bag slot or 0> <count or 0> <subject> | <body>",
			"/compound <slot1> <slot2> [slot3] [slot4] [slot5]; /manufacture <bench> <item1> <count1> <item2> <count2>; /fish; /mine; /chop; /stop",
		} {
			if err := s.chatFeedback(c, line); err != nil {
				return err
			}
		}
		return nil
	}
	lines := []string{
		"/heal [HP] [SP]; /gold <amount>; /item [add] <ID> [count]",
		"/level <level>; /points <gain>; /stats <STR CON INT WIS AGI>; /exp <total>; /skill <ID> [grade]",
		"/restat [character]; /clearskills [character]; /repair [character]; /im <gain>",
		"/warp <map> [X Y] or <character>; /town <name>; /summon <character>; /summonall",
		"/kick <character> [reason]; /broadcast <message>; /droprate [multiplier]",
	}
	for _, command := range gmCommandDefinitions {
		lines = append(lines, "/"+command.usage)
	}
	for _, line := range lines {
		if err := s.chatFeedback(c, line); err != nil {
			return err
		}
	}
	return nil
}

func gmIdle(c *Session) bool {
	return commandTravelAvailable(c) && c.trade == nil && c.outputError() == nil
}

func (s *Server) gmTarget(actor *Session, query string) (*Session, error) {
	var target *Session
	id, idErr := strconv.ParseUint(query, 10, 32)
	for _, player := range s.gmOnline() {
		if (idErr == nil && player.character.ID == uint32(id)) || strings.EqualFold(player.character.Name, query) {
			target = player
			break
		}
	}
	if target == nil {
		return nil, s.chatFeedback(actor, "That character is offline or unavailable.")
	}
	return target, nil
}

func (s *Server) gmOnline() []*Session {
	players := make([]*Session, 0, len(s.world))
	for _, c := range s.world {
		players = append(players, c)
	}
	sort.Slice(players, func(i, j int) bool { return players[i].info.ID < players[j].info.ID })
	return players
}

func (s *Server) gmInformation(ctx context.Context, c *Session, command string, args []string) error {
	if command == "online" || command == "who" {
		if len(args) > 0 {
			return s.chatFeedback(c, "Usage: /online")
		}
		players := s.gmOnline()
		if err := s.chatFeedback(c, fmt.Sprintf("Online characters: %d", len(players))); err != nil {
			return err
		}
		const onlineDisplayLimit = 12
		for _, p := range players[:min(len(players), onlineDisplayLimit)] {
			ch := p.character
			if err := s.chatFeedback(c, fmt.Sprintf("%s (#%d), Lv.%d, Map %d (%d,%d)", ch.Name, ch.ID, ch.Level, ch.Map, ch.X, ch.Y)); err != nil {
				return err
			}
		}
		if len(players) > onlineDisplayLimit {
			return s.chatFeedback(c, fmt.Sprintf("... and %d more.", len(players)-onlineDisplayLimit))
		}
		return nil
	}
	if len(args) > 1 {
		return s.chatFeedback(c, "Usage: /info [character]")
	}
	target := c
	if len(args) == 1 {
		var err error
		target, err = s.gmTarget(c, args[0])
		if target == nil {
			return err
		}
	}
	ch := target.character
	balances, err := s.Store.MallBalances(ctx, target.account.ID)
	if err != nil {
		return err
	}
	a := ch.Base
	lines := []string{
		fmt.Sprintf("%s (#%d): Level %d, element %d, reborn %t", ch.Name, ch.ID, ch.Level, ch.Element, ch.Reborn),
		fmt.Sprintf("HP %d/%d, SP %d/%d; Gold %d; IM %d; points %d", ch.HP, ch.MaxHP, ch.SP, ch.MaxSP, ch.Gold, balances.Points, ch.StatPoints),
		fmt.Sprintf("STR %d, CON %d, INT %d, WIS %d, AGI %d", a.Strength, a.Constitution, a.Intelligence, a.Wisdom, a.Agility),
		fmt.Sprintf("Map %d (%d,%d); Skills %d; Invisible %t; Muted until %s", ch.Map, ch.X, ch.Y, len(ch.Skills), target.invisible, ch.MutedUntil.UTC().Format("2006-01-02 15:04:05")),
	}
	if pet := gmSelectedPet(ch); pet >= 0 {
		p := ch.Pets[pet]
		lines = append(lines, fmt.Sprintf("Pet %s (#%d), Level %d, Amity %d, HP %d/%d", p.Name, p.ID, p.Level, p.Amity, p.HP, p.MaxHP))
	}
	for _, line := range lines {
		if err := s.chatFeedback(c, strings.TrimSpace(line)); err != nil {
			return err
		}
	}
	return nil
}

const (
	gmMuteDefaultMinutes = 10
	gmMuteMaximumMinutes = 365 * 24 * 60
	gmJailMap            = 10000
	gmJailX              = 600
	gmJailY              = 600
	gmReleaseMap         = 10001
	gmReleaseX           = 800
	gmReleaseY           = 750
)

func (s *Server) gmKick(c *Session, reason string) {
	if reason == "" {
		reason = "Disconnected by a game master."
	}
	_ = s.chatFeedback(c, reason)
	c.closeAfterFlush()
}

func (s *Server) gmModeration(ctx context.Context, c *Session, command string, args []string) error {
	release := command == "unmute" || command == "unjail"
	if len(args) < 1 || len(args) > 2 || (release && len(args) != 1) {
		usage := "Usage: /" + command + " <character>"
		if !release {
			usage += " [minutes]"
		}
		return s.chatFeedback(c, usage)
	}
	minutes := uint64(gmMuteDefaultMinutes)
	if !release && len(args) == 2 {
		n, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil || n < 1 || n > gmMuteMaximumMinutes {
			return s.chatFeedback(c, "Duration must be 1–525600 minutes.")
		}
		minutes = n
	}
	target, err := s.gmTarget(c, args[0])
	if target == nil {
		return err
	}
	jail := command == "jail" || command == "unjail"
	if jail && !gmIdle(target) {
		return s.chatFeedback(c, "That character must finish active interactions first.")
	}
	next := target.character.Clone()
	next.MutedUntil = time.Time{}
	if !release {
		next.MutedUntil = time.Now().UTC().Add(time.Duration(minutes) * time.Minute)
	}
	dst := world.Destination{Map: gmJailMap, X: gmJailX, Y: gmJailY}
	if release {
		dst = world.Destination{Map: gmReleaseMap, X: gmReleaseX, Y: gmReleaseY}
	}
	if jail {
		if target.openTent != nil {
			if err = s.closePlayerTent(ctx, target); err != nil {
				return err
			}
		}
		next.TentReturn = nil
		if _, ok := s.World.Map(dst.Map); !ok {
			return s.chatFeedback(c, "The moderation destination is missing from the assets database.")
		}
		next.Map, next.X, next.Y = dst.Map, dst.X, dst.Y
		// Save both the mute and location together, retaining the old runtime map
		// until its peers have received the departure.
		if err = s.Store.UpdateCharacter(ctx, target.account.ID, next.ID, func(stored *game.Character) error { *stored = next; return nil }); err != nil {
			return err
		}
		baseline := next.Clone()
		target.autosaveBaseline = &baseline
		target.character.MutedUntil = next.MutedUntil
		if err = s.teleportPrelude(target); err == nil {
			err = s.teleportAfterSave(target, dst, 0)
		}
		if err != nil {
			s.depart(target, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(next.ID).U16(dst.Map).U16(dst.X).U16(dst.Y).U16(0).U8(0))
			target.character.Map, target.character.X, target.character.Y = dst.Map, dst.X, dst.Y
			target.conn.Close()
			return s.chatFeedback(c, "Moderation saved; the target must reconnect.")
		}
	} else if err = s.commit(ctx, target, next); err != nil {
		return err
	}
	if err = s.chatFeedback(target, "A game master applied /"+command+"."); err != nil {
		target.conn.Close()
	}
	if target != c {
		return s.chatFeedback(c, fmt.Sprintf("%s: /%s saved.", target.character.Name, command))
	}
	return nil
}

func (s *Server) gmVisibility(_ context.Context, c *Session, command string, args []string) error {
	if len(args) != 0 {
		return s.chatFeedback(c, "Usage: /invis, /hide or /unhide")
	}
	hidden := !c.invisible
	if command == "hide" {
		hidden = true
	}
	if command == "unhide" {
		hidden = false
	}
	if hidden == c.invisible {
		return s.chatFeedback(c, "Visibility is already set.")
	}
	if hidden {
		s.broadcastWorld(c, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(c.character.ID).U16(0).U16(0).U16(0).U16(0).U8(0))
		s.closeStall(c)
		c.gathering = nil
		c.invisible = true
	} else {
		packets, err := serverwire.PeerPackets(*c.character, false, false)
		if err != nil {
			return err
		}
		packets = append(packets, s.peerPetPackets(c.character)...)
		if c.emote != 0 {
			packets = append(packets, protocol.Builder{protocol.CommandPose, protocol.PoseBroadcast}.U32(c.character.ID).U8(c.emote))
		}
		c.invisible = false
		for _, packet := range packets {
			s.broadcastWorld(c, packet)
		}
	}
	return s.chatFeedback(c, "GM visibility: "+strings.ToUpper(strconv.FormatBool(!hidden)))
}

const (
	gmShutdownDefaultSeconds = 10
	gmShutdownMaximumSeconds = 300
	gmShutdownNoticeInterval = 10
	gmShutdownFinalNotices   = 5
)

func (s *Server) gmCombat(_ context.Context, c *Session, command string, args []string) error {
	if command == "battle" || command == "fight" {
		if len(args) != 1 {
			return s.chatFeedback(c, "Usage: /battle <monster template ID>")
		}
		id, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return s.chatFeedback(c, "Invalid monster template ID.")
		}
		npc, ok := s.Assets.NPCs[uint16(id)]
		if !ok || npc.HP == 0 {
			return s.chatFeedback(c, "Monster template is missing or has no HP.")
		}
		return s.startBattle(c, &battleRun{}, []battle.Enemy{{Template: uint32(npc.ID), Name: npc.Name, Level: int(npc.Level), Element: npc.Element}})
	}
	if len(args) != 0 {
		return s.chatFeedback(c, "Usage: /winbattle")
	}
	run := c.battle
	if run == nil || run.b == nil || run.b.Finished {
		return s.chatFeedback(c, "There is no active battle to finish.")
	}
	if run.b.Processing {
		return s.chatFeedback(c, "Wait for the current battle animation to finish.")
	}
	enemies := run.b.Defenders
	outcome := battle.Victory
	if member := run.member(c); member != nil && member.self.Side == battle.Defender {
		enemies = run.b.Attackers
		outcome = battle.Defeat
	}
	for _, enemy := range enemies {
		enemy.HP = 0
	}
	run.b.DiscardReinforcements()
	s.endBattle(run, outcome)
	return nil
}

func (s *Server) gmOperations(_ context.Context, c *Session, command string, args []string) error {
	if command == "kickall" {
		reason := strings.Join(args, " ")
		if reason == "" {
			reason = "Server maintenance."
		}
		count := 0
		s.mu.Lock()
		var targets []*Session
		for _, target := range s.sessions {
			if target.info.CharacterID != 0 {
				targets = append(targets, target)
			}
		}
		s.mu.Unlock()
		for _, target := range targets {
			if target.gmLevel.Load() == 0 {
				s.gmKick(target, reason)
				count++
			}
		}
		return s.chatFeedback(c, fmt.Sprintf("Disconnected %d non-GM characters.", count))
	}
	if len(args) > 1 {
		return s.chatFeedback(c, "Usage: /shutdown [1-300 seconds]")
	}
	seconds := uint64(gmShutdownDefaultSeconds)
	if len(args) == 1 {
		n, err := strconv.ParseUint(args[0], 10, 16)
		if err != nil {
			return s.chatFeedback(c, "Invalid shutdown delay.")
		}
		seconds = min(max(n, 1), gmShutdownMaximumSeconds)
	}
	if !s.shutdownAt.IsZero() {
		return s.chatFeedback(c, "A shutdown is already scheduled.")
	}
	s.shutdownAt = time.Now().Add(time.Duration(seconds) * time.Second)
	s.notice(fmt.Sprintf("Server shutdown in %d seconds.", seconds))
	return s.chatFeedback(c, "Graceful shutdown scheduled.")
}

// Shutdown stops Run through its normal connection cleanup and autosave path.
func (s *Server) Shutdown() { s.shutdownOnce.Do(func() { close(s.shutdown) }) }

func (s *Server) runShutdown(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.shutdownTick(now)
		}
	}
}

func (s *Server) shutdownTick(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if s.shutdownAt.IsZero() {
		return
	}
	if !now.Before(s.shutdownAt) {
		s.shutdownAt = time.Time{}
		s.notice("Server shutting down.")
		s.Shutdown()
		return
	}
	remaining := int(s.shutdownAt.Sub(now).Seconds()) + 1
	if remaining <= gmShutdownFinalNotices || remaining%gmShutdownNoticeInterval == 0 {
		s.notice(fmt.Sprintf("Server shutdown in %d seconds.", remaining))
	}
}

// Reuse the existing durable log-level control. Packet traces retain their
// command-specific bounds and credential redaction when debugging is enabled.
func (s *Server) gmPacketDebug(ctx context.Context, c *Session, _ string, args []string) error {
	if len(args) > 1 {
		return s.chatFeedback(c, "/packetdebug [on|off]")
	}
	enabled := s.logs.Level() != "debug"
	if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "on", "true", "1", "enable":
			enabled = true
		case "off", "false", "0", "disable":
			enabled = false
		default:
			return s.chatFeedback(c, "/packetdebug [on|off]")
		}
	}
	level := "info"
	if enabled {
		level = "debug"
	}
	if err := s.Store.SaveSettings(ctx, map[string]string{"log_level": level}); err != nil {
		return err
	}
	s.logs.SetLevel(level)
	return s.chatFeedback(c, "Packet diagnostics: "+level)
}

// gmDropRate ports AC02's process-local loot multiplier. The caller holds worldMu.
// It applies to subsequent victory loot rolls, including battles already in progress.
func (s *Server) gmDropRate(c *Session, arguments []string) error {
	if len(arguments) == 0 {
		multiplier := s.dropRateMultiplier
		if multiplier == 0 {
			multiplier = battle.DefaultDropRateMultiplier
		}
		return s.chatFeedback(c, fmt.Sprintf("Current drop rate multiplier: %.1fx. Usage: /droprate <multiplier>", multiplier))
	}
	if len(arguments) != 1 {
		return s.chatFeedback(c, "Usage: /droprate <multiplier> (0.1–100)")
	}
	multiplier, err := strconv.ParseFloat(arguments[0], 64)
	if err != nil || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return s.chatFeedback(c, "Drop rate multiplier must be a finite number.")
	}
	s.dropRateMultiplier = min(battle.MaxDropRateMultiplier, max(battle.MinDropRateMultiplier, multiplier))
	return s.chatFeedback(c, fmt.Sprintf("Global drop rate multiplier set to %.1fx!", s.dropRateMultiplier))
}
