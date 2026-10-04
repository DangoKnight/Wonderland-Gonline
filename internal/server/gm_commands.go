package server

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
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
			"/compound <slot1> <slot2>; /manufacture <bench> <item1> <count1> <item2> <count2>; /fish; /mine; /chop; /stop",
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

func gmIdle(c *Session) bool { return commandTravelAvailable(c) && c.trade == nil }

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
