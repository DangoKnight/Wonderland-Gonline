package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

// chat handles AC2. Reference: AC02.Recv1/Recv2. The caller holds worldMu.
func (s *Server) chat(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	if c.character == nil || !c.ready {
		return nil
	}
	if time.Now().Before(c.character.MutedUntil) {
		return s.chatFeedback(c, "Your chat privileges are temporarily muted.")
	}
	text := string(p[2:])
	switch p[1] {
	case protocol.ChatWorldMessage:
		return s.worldChat(c, text, false)
	case protocol.ChatMapMessage:
		if strings.HasPrefix(text, ":") || strings.HasPrefix(text, "/") {
			if !validChatText(text, chatCommandMaxBytes) {
				return protocol.ErrMalformed
			}
			if handled, err := s.chatChannelCommand(c, text); handled {
				return err
			}
			s.closeStall(c)
			return s.command(ctx, c, text)
		}
		if !validChatText(text, chatMessageMaxBytes) {
			return s.chatFeedback(c, "Chat messages must contain 1–60 bytes without control characters.")
		}
		s.deliverChat(c, s.peers(c), game.ChatChannelLocal, chatPacket(protocol.ChatMapMessage, c, p[2:]), false)
		return nil
	case protocol.ChatWhisperMessage:
		// The client names the target by ID before the text.
		if len(p) < 2+4 {
			return protocol.ErrMalformed
		}
		target := s.onlineByID(binary.LittleEndian.Uint32(p[2:]))
		return s.whisper(c, target, string(p[6:]))
	case protocol.ChatTeamMessage:
		return s.teamChat(c, text)
	case protocol.ChatGuildMessage:
		return s.guildChat(ctx, c, text)
	}
	return ErrUnsupported
}

// command runs a chat command. GM rights come only from an administrator-granted account level;
// the C# default names and gm_list.txt are deliberately not trusted.
func (s *Server) command(ctx context.Context, c *Session, text string) error {
	words := strings.Fields(text)
	if len(words) == 0 || len(words[0]) < 2 {
		return nil
	}
	name := strings.ToLower(words[0])
	if handled, err := s.tentChatCommand(ctx, c, name[1:], words[1:]); handled {
		return err
	}
	if handled, err := s.socialChatCommand(ctx, c, name[1:], words[1:], text); handled {
		return err
	}
	if handled, err := s.craftingChatCommand(ctx, c, name[1:], words[1:], text); handled {
		return err
	}
	switch name[1:] {
	case "guildcreate":
		if !commandTravelAvailable(c) || c.trade != nil {
			return nil
		}
		guildName := strings.TrimSpace(strings.TrimPrefix(text, words[0]))
		if err := s.Store.CreateGuild(ctx, c.character.ID, guildName); err != nil {
			return s.chatFeedback(c, err.Error())
		}
		return s.refreshGuilds(ctx)
	case "guild":
		return s.guildChat(ctx, c, strings.TrimSpace(strings.TrimPrefix(text, words[0])))
	case "help", "cmds", "cmd":
		return s.commandHelp(c)
	case "unride", "dismount", "carnie":
		if !commandTravelAvailable(c) || c.trade != nil {
			return nil
		}
		if name[1:] == "carnie" {
			return s.visitCarnie(ctx, c)
		}
		return s.publicDismount(ctx, c)
	}
	if c.gmLevel.Load() == 0 {
		return nil
	}
	s.Log.Info("GM command", "account", c.account.Username, "character", c.character.Name, "command", text)
	if spec, ok := gmCommandRegistry[name[1:]]; ok {
		if spec.idle && !gmIdle(c) {
			return s.chatFeedback(c, "Finish active interactions before using this command.")
		}
		return spec.handle(s, ctx, c, name[1:], words[1:])
	}
	switch name[1:] {
	case "town", "summonall", "warp", "goto", "tp", "summon", "bring":
		if !commandTravelAvailable(c) {
			return nil
		}
	}
	switch name[1:] {
	case "level", "lvl", "points", "sp", "statpoint", "statpoints", "stats", "stat", "exp", "skill":
		return s.gmProgress(ctx, c, name[1:], words)
	case "clearskills", "resetskills":
		return s.gmClearSkills(ctx, c, words[1:])
	case "restat", "resetstats":
		return s.gmRestat(ctx, c, words[1:])
	case "repair", "fixall":
		return s.gmRepair(ctx, c, words[1:])
	case "droprate":
		return s.gmDropRate(c, words[1:])
	case "heal", "hp", "full":
		return s.heal(ctx, c, words)
	case "gold", "money":
		if len(words) < 2 {
			return nil
		}
		amount, e := strconv.ParseInt(words[1], 10, 32)
		if e != nil {
			return nil
		}
		gold := uint32(max(0, amount))
		if e = s.Store.UpdateCharacter(ctx, c.account.ID, c.character.ID, func(char *game.Character) error { char.Gold = gold; return nil }); e != nil {
			return e
		}
		s.cancelTrade(c)
		c.character.Gold = gold
		return c.send(protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(gold))
	case "im", "points_im", "mallpoints":
		return s.gmMallPoints(ctx, c, words)
	case "item":
		return s.giveItem(ctx, c, words)
	case "floorup":
		return s.floorUp(ctx, c)
	case "floordown":
		return s.floorDown(ctx, c)
	case "town":
		return s.gmTown(ctx, c, words)
	case "summonall":
		return s.summonAll(ctx, c)
	case "warp", "goto", "tp":
		return s.gmWarp(ctx, c, words)
	case "summon", "bring":
		if len(words) < 2 {
			return nil
		}
		if target := s.findOnline(words[1]); target != nil && commandTravelAvailable(target) {
			return s.commandTeleport(ctx, target, world.Destination{Map: c.character.Map, X: c.character.X, Y: c.character.Y})
		}
	case "kick":
		if len(words) < 2 {
			return nil
		}
		if target := s.findOnline(words[1]); target != nil {
			s.Log.Info("GM kick", "target", target.character.Name)
			reason := strings.Join(words[2:], " ")
			s.gmKick(target, reason)
		}
	case "b", "broadcast", "notice":
		if len(words) >= 2 {
			s.notice(strings.TrimSpace(text[len(words[0]):]))
		}
	default:
		s.Log.Debug("GM command not ported", "command", name)
	}
	return nil
}

// findOnline is GmManager.FindOnlinePlayer over published characters: character ID,
// then exact name, then the first name containing the query. Caller holds worldMu.
func (s *Server) findOnline(query string) *Session {
	query = strings.TrimSpace(query)
	online := make([]*Session, 0, len(s.world))
	for _, c := range s.world {
		online = append(online, c)
	}
	sort.Slice(online, func(i, j int) bool { return online[i].info.ID < online[j].info.ID })
	if id, e := strconv.ParseUint(query, 10, 32); e == nil {
		for _, c := range online {
			if c.character.ID == uint32(id) {
				return c
			}
		}
	}
	for _, c := range online {
		if strings.EqualFold(c.character.Name, query) {
			return c
		}
	}
	lower := strings.ToLower(query)
	for _, c := range online {
		if strings.Contains(strings.ToLower(c.character.Name), lower) {
			return c
		}
	}
	return nil
}

// notice is GmManager.BroadcastNotice: AC2:4 from sender 0 to every character in game.
func (s *Server) notice(text string) {
	if text == "" {
		return
	}
	packet := protocol.Builder{protocol.CommandChat, protocol.ChatGMMessage}.U32(0).Bytes([]byte(text))
	s.mu.Lock()
	recipients := make([]*Session, 0, len(s.sessions))
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 {
			recipients = append(recipients, c)
		}
	}
	s.mu.Unlock()
	for _, c := range recipients {
		if c.send(packet) != nil {
			c.conn.Close()
		}
	}
}

// commandTeleport is GameMap.Teleport(CmD). Caller holds worldMu.
func (s *Server) commandTeleport(ctx context.Context, c *Session, dst world.Destination) error {
	if _, ok := s.World.Map(dst.Map); !ok {
		return nil
	}
	if e := s.teleportPrelude(c); e != nil {
		return e
	}
	return s.teleport(ctx, c, dst, 0)
}

const (
	gmBangkokMapID   = 12600
	gmBangkokTempleX = 2450
	gmBangkokTempleY = 450
)

func (s *Server) gmWarp(ctx context.Context, c *Session, words []string) error {
	if len(words) == 2 {
		if alias := strings.ToLower(words[1]); alias == "bangkok" || alias == "bangkokmonk" || alias == "monk" {
			return s.commandTeleport(ctx, c, world.Destination{Map: gmBangkokMapID, X: gmBangkokTempleX, Y: gmBangkokTempleY})
		}
		if m, e := strconv.ParseUint(words[1], 10, 16); e == nil {
			if m == gmBangkokMapID {
				return s.commandTeleport(ctx, c, world.Destination{Map: gmBangkokMapID, X: gmBangkokTempleX, Y: gmBangkokTempleY})
			}
			return s.commandTeleport(ctx, c, world.Destination{Map: uint16(m), X: 600, Y: 600})
		}
		if target := s.findOnline(words[1]); target != nil {
			t := target.character
			return s.commandTeleport(ctx, c, world.Destination{Map: t.Map, X: t.X, Y: t.Y})
		}
		return nil
	}
	if len(words) >= 4 {
		m, e1 := strconv.ParseUint(words[1], 10, 16)
		x, e2 := strconv.ParseUint(words[2], 10, 16)
		y, e3 := strconv.ParseUint(words[3], 10, 16)
		if e1 == nil && e2 == nil && e3 == nil {
			return s.commandTeleport(ctx, c, world.Destination{Map: uint16(m), X: uint16(x), Y: uint16(y)})
		}
	}
	return nil
}

// heal restores HP/SP, optionally to given values, capped at the equipped maxima.
func (s *Server) heal(ctx context.Context, c *Session, words []string) error {
	next := *c.character
	full := next.Combat(s.Assets.Items)
	maxHP, maxSP := uint32(max(full.MaxHP, 1)), uint32(max(full.MaxSP, 0))
	hp, sp := maxHP, maxSP
	if len(words) >= 2 {
		if v, e := strconv.Atoi(words[1]); e == nil {
			hp = uint32(min(max(v, 0), int(maxHP)))
			if len(words) >= 3 {
				if v, e := strconv.Atoi(words[2]); e == nil {
					sp = uint32(min(max(v, 0), int(maxSP)))
				}
			}
		}
	}
	next.MaxHP, next.MaxSP, next.HP, next.SP = maxHP, maxSP, hp, sp
	base, e := s.nativeStatsSnapshot(next).BaseStatsPacket(func(id uint16) (uint16, bool) { skill, ok := s.Assets.Skills[id]; return skill.TableOrder, ok })
	if e != nil {
		return nil
	}
	if e = s.Store.UpdateCharacter(ctx, c.account.ID, next.ID, func(char *game.Character) error {
		char.MaxHP, char.MaxSP, char.HP, char.SP = maxHP, maxSP, hp, sp
		return nil
	}); e != nil {
		return e
	}
	// Only vitals were committed; keep the position checkpoint unchanged.
	*c.character = next
	for _, packet := range append(next.StatPackets(s.Assets.Items), base) {
		if e = c.send(packet); e != nil {
			return e
		}
	}
	return nil
}

// radioSet may be held only once.
const radioSet = 34076

// giveItem is ":item <id> [count]" or ":item add <id> [count]".
func (s *Server) giveItem(ctx context.Context, c *Session, words []string) error {
	args := words[1:]
	if len(args) > 0 && strings.EqualFold(args[0], "add") {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil
	}
	id, e := strconv.ParseUint(args[0], 10, 16)
	if e != nil || id == 0 {
		return nil
	}
	count := uint64(1)
	if len(args) > 1 {
		if n, e := strconv.ParseUint(args[1], 10, 8); e == nil {
			count = max(n, 1)
		}
	}
	def, ok := s.Assets.Items[uint16(id)]
	if !ok {
		return nil
	}
	if id == radioSet {
		for _, item := range c.character.Bag {
			if item.ID == radioSet {
				return nil
			}
		}
		for _, item := range c.character.Equipment {
			if item.ID == radioSet {
				return nil
			}
		}
	}
	bag := c.character.Bag
	adds, e := bag.Grant(game.Item{ID: uint16(id)}, int(count), def.StackLimit(), s.Assets.Items)
	if e != nil {
		return nil
	}
	if e = s.saveBag(ctx, c, bag); e != nil {
		return e
	}
	return c.send(bag.AdditionPacket(adds))
}

// emote handles AC32. Native AC32:1 is a temporary expression, while AC32:2
// is a held pose. Expressions repeat independently of the saved pose and are
// never replayed on arrival. The sender renders locally. Caller holds worldMu.
func (s *Server) emote(c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	sub, pose := p[1], byte(0)
	switch sub {
	case protocol.PoseEmote:
		if len(p) != 3 {
			return protocol.ErrMalformed
		}
		// FUN_00432478 -> FUN_00430700 starts a fresh, timed expression.
		// Do not suppress repeated selections or overwrite the held pose.
		s.broadcastWorld(c, protocol.Builder{protocol.CommandPose, protocol.PoseEmote}.U32(c.character.ID).U8(p[2]))
		return nil
	case protocol.PoseBroadcast:
		if len(p) > 2 {
			pose = p[2]
		}
	case protocol.PoseStop:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		s.endEvent(c) // Clear callbacks even when the pose is already zero.
		// Stopping a pose is reported as AC32:2 with pose zero.
		sub = protocol.PoseBroadcast
	default:
		return ErrUnsupported
	}
	if c.emote == pose {
		return nil
	}
	c.emote = pose
	s.broadcastWorld(c, protocol.Builder{protocol.CommandPose, sub}.U32(c.character.ID).U8(pose))
	return nil
}

const (
	chatMessageMaxBytes = 60
	chatCommandMaxBytes = 512
)

func validChatText(text string, limit int) bool {
	if len(text) == 0 || len(text) > limit || strings.TrimSpace(text) == "" {
		return false
	}
	for _, b := range []byte(text) {
		if b < 32 || b == 127 {
			return false
		}
	}
	return true
}

// chatPacket is 2/sub with the speaker's ID and the text, the layout the
// client reads for every chat channel (2/1 World, 2/2 Local, 2/3 Whisper,
// 2/5 Team).
func chatPacket(sub byte, sender *Session, text []byte) []byte {
	return protocol.Builder{protocol.CommandChat, sub}.U32(sender.character.ID).Bytes(text)
}

func (s *Server) deliverChat(sender *Session, recipients []*Session, channel byte, packet []byte, echo bool) {
	for _, peer := range recipients {
		if peer == nil || !peer.ready || peer.character == nil || (peer == sender && !echo) || peer.character.Preferences().Channels&channel == 0 {
			continue
		}
		if err := peer.send(packet); err != nil {
			peer.conn.Close()
		}
	}
}

func (s *Server) chatFeedback(c *Session, text string) error {
	if c.adminFeedback != nil {
		*c.adminFeedback = append(*c.adminFeedback, text)
	}
	return c.send(serverwire.HeadBanner(text))
}

// worldChat is 2/1 to every player in the world. The client logs its own
// line when it sends 2/1, so only the chat commands echo it.
func (s *Server) worldChat(c *Session, text string, echo bool) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Chat messages must contain 1–60 bytes without control characters.")
	}
	recipients := make([]*Session, 0, len(s.world))
	for _, peer := range s.world {
		recipients = append(recipients, peer)
	}
	s.deliverChat(c, recipients, game.ChatChannelWorld, chatPacket(protocol.ChatWorldMessage, c, []byte(text)), echo)
	return nil
}

// teamChat is 2/5 to the party, the sender included: the client does not
// log its own Team line.
func (s *Server) teamChat(c *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Usage: /team <message> (up to 60 bytes)")
	}
	if c.party == nil {
		return s.chatFeedback(c, "You are not in a party.")
	}
	s.deliverChat(c, c.party.members, game.ChatChannelTeam, chatPacket(protocol.ChatTeamMessage, c, []byte(text)), true)
	return nil
}

// whisper is 2/3 from the sender to the target, echoed to the sender: the
// client does not log its own whisper.
func (s *Server) whisper(c *Session, target *Session, text string) error {
	if !validChatText(text, chatMessageMaxBytes) {
		return s.chatFeedback(c, "Usage: /whisper <character ID or name> <message>")
	}
	if target == nil || target.character.Preferences().Channels&game.ChatChannelWhisper == 0 {
		return s.chatFeedback(c, "That character is unavailable for whispers.")
	}
	recipients := []*Session{target}
	if target != c {
		recipients = append(recipients, c)
	}
	s.deliverChat(c, recipients, game.ChatChannelWhisper, chatPacket(protocol.ChatWhisperMessage, c, []byte(text)), true)
	return nil
}

// onlineByID is a ready player in the world by character ID.
func (s *Server) onlineByID(id uint32) *Session {
	for _, peer := range s.world {
		if peer.character != nil && peer.ready && peer.character.ID == id {
			return peer
		}
	}
	return nil
}

// Public channel commands are handled before the administrative command gate.
func (s *Server) chatChannelCommand(c *Session, text string) (bool, error) {
	header, body, _ := strings.Cut(text, " ")
	switch strings.ToLower(header) {
	case "/world", ":world", "/global", ":global":
		return true, s.worldChat(c, body, true)
	case "/team", ":team", "/party", ":party", "/p":
		return true, s.teamChat(c, body)
	case "/whisper", ":whisper", "/w", "/tell":
		targetName, message, _ := strings.Cut(strings.TrimSpace(body), " ")
		var target *Session
		id, err := strconv.ParseUint(targetName, 10, 32)
		for _, peer := range s.world {
			matches := peer.character != nil && strings.EqualFold(peer.character.Name, targetName)
			if err == nil {
				matches = peer.character != nil && peer.character.ID == uint32(id)
			}
			if peer.character != nil && peer.ready && matches {
				target = peer
				break
			}
		}
		return true, s.whisper(c, target, message)
	}
	return false, nil
}

const marriageProposalTTL = time.Minute

type marriageProposal struct {
	From *Session
	At   time.Time
}

// Public commands provide the reference's social workflows without inventing
// undocumented native request layouts. Outbound mailbox packets preserve Mail.cs.
func (s *Server) socialChatCommand(ctx context.Context, c *Session, name string, args []string, text string) (bool, error) {
	ref := store.CharacterRef{Account: c.account.ID, ID: c.character.ID}
	fail := func(message string) (bool, error) { return true, s.chatFeedback(c, message) }
	idle := func() bool { return commandTravelAvailable(c) && c.trade == nil && c.stall == nil }
	switch name {
	case "marry":
		if !idle() {
			return fail("Finish active interactions before proposing.")
		}
		if len(args) != 1 {
			return fail("Usage: /marry <character ID or name>")
		}
		partner := s.findOnline(args[0])
		if partner == nil || partner == c || !partner.ready || !tradeNear(c, partner) || !commandTravelAvailable(partner) || partner.trade != nil {
			return fail("Your partner must be nearby and available.")
		}
		rules := s.Assets.Economy.Marriage
		if uint16(c.character.Level) < rules.MinimumLevel || uint16(partner.character.Level) < rules.MinimumLevel || c.character.Gold < rules.Fee {
			return fail(fmt.Sprintf("Marriage requires level %d and %d gold.", rules.MinimumLevel, rules.Fee))
		}
		for _, peer := range []*Session{c, partner} {
			row, err := s.Store.Marriage(ctx, peer.character.ID)
			if err != nil {
				return true, err
			}
			if row != nil {
				return fail("One of you is already married.")
			}
		}
		if partner.marriageProposal != nil && time.Since(partner.marriageProposal.At) < marriageProposalTTL {
			return fail("That character already has a pending proposal.")
		}
		partner.marriageProposal = &marriageProposal{From: c, At: time.Now()}
		s.sendOrClose(partner, serverwire.TradeMessage(c.character.Name+" proposed marriage. Use /acceptmarry or /declinemarry."))
		return fail("Marriage proposal sent.")
	case "acceptmarry":
		if len(args) != 0 {
			return fail("Usage: /acceptmarry")
		}
		request := c.marriageProposal
		c.marriageProposal = nil
		if request == nil {
			return fail("You have no pending proposal.")
		}
		proposer := request.From
		if !idle() || time.Since(request.At) > marriageProposalTTL || s.world[proposer.info.ID] != proposer || !proposer.ready || !commandTravelAvailable(proposer) || proposer.trade != nil || proposer.stall != nil || !tradeNear(c, proposer) {
			return fail("Your marriage proposal is no longer valid.")
		}
		rules := s.Assets.Economy.Marriage
		chars, adds, err := s.Store.Marry(ctx, [2]store.CharacterRef{{Account: proposer.account.ID, ID: proposer.character.ID}, ref}, rules.MinimumLevel, rules.Fee, rules.Rings, s.Assets.Items, proposer.character.Clone(), c.character.Clone())
		if err != nil {
			return fail("Marriage failed: " + err.Error() + ".")
		}
		for i, peer := range []*Session{proposer, c} {
			s.adoptSavedCharacter(peer, chars[i])
			s.sendOrClose(peer, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(peer.character.Gold))
			if len(adds[i]) > 0 {
				s.sendOrClose(peer, peer.character.Bag.AdditionPacket(adds[i]))
			}
		}
		s.clearShapeshift(proposer)
		message := serverwire.TradeMessage("Congratulations! " + proposer.character.Name + " and " + c.character.Name + " are married.")
		s.broadcastWorld(c, message)
		return true, c.send(message)
	case "declinemarry":
		if len(args) != 0 {
			return fail("Usage: /declinemarry")
		}
		if request := c.marriageProposal; request != nil {
			s.sendOrClose(request.From, serverwire.TradeMessage("Marriage proposal declined."))
		}
		c.marriageProposal = nil
		return fail("Marriage proposal declined.")
	case "divorce":
		if !idle() || len(args) != 0 {
			return fail("Usage: /divorce (finish active interactions first)")
		}
		partner, err := s.Store.Divorce(ctx, ref)
		if err != nil {
			return fail(err.Error())
		}
		if peer := s.onlineByID(partner); peer != nil {
			s.sendOrClose(peer, serverwire.TradeMessage("Your marriage has been annulled."))
		}
		return fail("You are divorced.")
	case "warptospouse":
		if !idle() || len(args) != 0 {
			return fail("Usage: /warptospouse (finish active interactions first)")
		}
		marriage, err := s.Store.Marriage(ctx, c.character.ID)
		if err != nil {
			return true, err
		}
		if marriage == nil {
			return fail("You are not married.")
		}
		id := marriage.Character1
		if id == c.character.ID {
			id = marriage.Character2
		}
		partner := s.onlineByID(id)
		if partner == nil || !commandTravelAvailable(partner) || partner.trade != nil || partner.invisible {
			return fail("Your spouse is unavailable.")
		}
		return true, s.commandTeleport(ctx, c, world.Destination{Map: partner.character.Map, X: partner.character.X, Y: partner.character.Y})
	case "mail":
		if len(args) != 0 {
			return fail("Usage: /mail")
		}
		return true, s.parcelInbox(ctx, c)
	case "readmail", "claimmail", "deletemail":
		if len(args) != 1 {
			return fail("Usage: /" + name + " <mail ID>")
		}
		id, err := strconv.ParseUint(args[0], 10, 32)
		if err != nil || id == 0 {
			return fail("Invalid mail ID.")
		}
		if name == "readmail" {
			row, err := s.Store.ReadParcel(ctx, c.character.ID, uint32(id))
			if err != nil {
				return fail(err.Error())
			}
			p, _ := protocol.Builder{protocol.CommandInventory, protocol.InventoryParcelDetails}.U32(row.ID).String(row.Body)
			return true, c.send(p)
		}
		if !idle() {
			return fail("Finish active interactions before changing mail attachments.")
		}
		if name == "deletemail" {
			if err = s.Store.DeleteParcel(ctx, c.character.ID, uint32(id)); err != nil {
				return fail(err.Error())
			}
			return true, s.parcelInbox(ctx, c)
		}
		next, adds, err := s.Store.ClaimParcel(ctx, ref, uint32(id), s.Assets.Items, c.character.Clone())
		if err != nil {
			return fail(err.Error())
		}
		s.adoptSavedCharacter(c, next)
		if len(adds) > 0 {
			s.sendOrClose(c, c.character.Bag.AdditionPacket(adds))
		}
		s.sendOrClose(c, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
		return true, s.parcelInbox(ctx, c)
	case "sendmail":
		if !idle() {
			return fail("Finish active interactions before sending mail.")
		}
		// Fixed numeric fields, then a free-form subject | body.
		rest := strings.TrimSpace(strings.TrimPrefix(text, strings.Fields(text)[0]))
		fields := strings.SplitN(rest, " ", 5)
		if len(fields) != 5 {
			return fail("Usage: /sendmail <recipient ID> <gold> <bag slot or 0> <count or 0> <subject> | <body>")
		}
		var values [4]uint64
		for i := range values {
			bits := 32
			if i > 1 {
				bits = 8
			}
			value, err := strconv.ParseUint(fields[i], 10, bits)
			if err != nil {
				return fail("Invalid mail attachment fields.")
			}
			values[i] = value
		}
		subject, body, ok := strings.Cut(fields[4], "|")
		if !ok {
			return fail("Separate the subject and body with |.")
		}
		next, err := s.Store.SendParcel(ctx, ref, uint32(values[0]), strings.TrimSpace(subject), strings.TrimSpace(body), uint32(values[1]), byte(values[2]), byte(values[3]), s.Assets.Items, c.character.Clone())
		if err != nil {
			return fail(err.Error())
		}
		s.adoptSavedCharacter(c, next)
		if values[2] != 0 {
			s.sendOrClose(c, []byte{protocol.CommandInventory, protocol.InventoryRemove, byte(values[2]), byte(values[3])})
		}
		s.sendOrClose(c, protocol.Builder{protocol.CommandGold, protocol.GoldBalance}.U32(next.Gold))
		if peer := s.onlineByID(uint32(values[0])); peer != nil {
			s.sendOrClose(peer, serverwire.TradeMessage("You received a letter from "+c.character.Name+"."))
		}
		return fail("Letter sent.")
	}
	return false, nil
}
func (s *Server) parcelInbox(ctx context.Context, c *Session) error {
	rows, err := s.Store.Parcels(ctx, c.character.ID)
	if err != nil {
		return err
	}
	p := protocol.Builder{protocol.CommandInventory, protocol.InventoryParcelList}.U8(byte(len(rows)))
	for _, row := range rows {
		sender := uint32(0)
		if row.SenderID != nil {
			sender = *row.SenderID
		}
		p = p.U32(row.ID).U32(sender)
		p, err = p.String(row.SenderName)
		if err != nil {
			return err
		}
		p, err = p.String(row.Subject)
		if err != nil {
			return err
		}
		read, claimed := byte(0), byte(0)
		if row.IsRead {
			read = 1
		}
		if row.Claimed {
			claimed = 1
			row.Gold, row.ItemID, row.Count = 0, 0, 0
		}
		p = p.U8(read).U8(claimed).U32(row.Gold).U16(row.ItemID).U8(row.Count)
	}
	return c.send(p)
}
