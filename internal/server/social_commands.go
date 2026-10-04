package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

const marriageProposalTTL = time.Minute
const weddingFireworksEffect = 60010

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
		s.sendOrClose(partner, tradeMessage(c.character.Name+" proposed marriage. Use /acceptmarry or /declinemarry."))
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
		effect := protocol.Builder{protocol.CommandCharacterState, protocol.CharacterStateRepairEffect}.U32(proposer.character.ID).U16(weddingFireworksEffect)
		s.broadcastWorld(proposer, effect)
		s.sendOrClose(proposer, effect)
		message := tradeMessage("Congratulations! " + proposer.character.Name + " and " + c.character.Name + " are married.")
		s.broadcastWorld(c, message)
		return true, c.send(message)
	case "declinemarry":
		if len(args) != 0 {
			return fail("Usage: /declinemarry")
		}
		if request := c.marriageProposal; request != nil {
			s.sendOrClose(request.From, tradeMessage("Marriage proposal declined."))
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
			s.sendOrClose(peer, tradeMessage("Your marriage has been annulled."))
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
			s.sendOrClose(peer, tradeMessage("You received a letter from "+c.character.Name+"."))
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
