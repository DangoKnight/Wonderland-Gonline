package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

func (s *Server) gmBazaar(ctx context.Context, c *Session, _ string, args []string) error {
	usage := "/bazaar list | remove <ID> | create <item|pet|vehicle> <finite|infinite> <ID:price:stock,...> <title>"
	if len(args) == 0 {
		return s.chatFeedback(c, usage)
	}
	switch strings.ToLower(args[0]) {
	case "list":
		if len(args) != 1 {
			return s.chatFeedback(c, usage)
		}
		bs, err := s.Store.Bazaars(ctx, 0)
		if err != nil {
			return err
		}
		for _, b := range bs {
			s.sendOrClose(c, systemLine(fmt.Sprintf("Bazaar %d: %s, map %d (%d,%d), %d listings", b.VendorID, b.Title, b.Map, b.X, b.Y, len(b.Listings))))
		}
		return nil
	case "remove":
		if len(args) != 2 {
			return s.chatFeedback(c, usage)
		}
		id, err := strconv.ParseUint(args[1], 10, 32)
		if err != nil {
			return s.chatFeedback(c, usage)
		}
		b, err := s.Store.Bazaar(ctx, uint32(id))
		if err != nil {
			return s.chatFeedback(c, "Bazaar not found.")
		}
		if err = s.Store.DeleteBazaar(ctx, b.VendorID); err != nil {
			return err
		}
		packet := protocol.Builder{protocol.CommandInventory, protocol.InventoryStallDespawn}.U32(b.VendorID)
		for _, peer := range s.world {
			delete(peer.bazaarQuotes, b.VendorID)
			if peer.tentOwner == 0 && peer.character.Map == b.Map {
				s.sendOrClose(peer, packet, protocol.Builder{protocol.CommandMapAcknowledgment}.U32(b.VendorID).U16(0).U16(0).U16(0).U16(0).U8(0))
			}
		}
		return s.chatFeedback(c, "Bazaar removed.")
	case "create":
		if len(args) < 5 || c.tentOwner != 0 {
			return s.chatFeedback(c, usage)
		}
		kinds := map[string]byte{"item": store.BazaarItems, "pet": store.BazaarPets, "vehicle": store.BazaarVehicles}
		kind := kinds[strings.ToLower(args[1])]
		mode := strings.ToLower(args[2])
		if kind == 0 || mode != "finite" && mode != "infinite" {
			return s.chatFeedback(c, usage)
		}
		b := store.Bazaar{Name: "Merchant", Title: strings.Join(args[4:], " "), Map: c.character.Map, X: c.character.X, Y: c.character.Y, Kind: kind, Infinite: mode == "infinite"}
		for _, entry := range strings.Split(args[3], ",") {
			parts := strings.Split(entry, ":")
			if len(parts) != 3 {
				return s.chatFeedback(c, usage)
			}
			id, e1 := strconv.ParseUint(parts[0], 10, 16)
			price, e2 := strconv.ParseUint(parts[1], 10, 32)
			stock, e3 := strconv.ParseUint(parts[2], 10, 32)
			if e1 != nil || e2 != nil || e3 != nil {
				return s.chatFeedback(c, usage)
			}
			if kind == store.BazaarPets {
				if _, known := s.Assets.NPCs[uint16(id)]; !known {
					return s.chatFeedback(c, "Unknown pet template.")
				}
			} else {
				d, known := s.Assets.Items[uint16(id)]
				if !known || kind == store.BazaarVehicles && d.Type != game.VehicleType && d.Type != game.ItemTypeAppliance {
					return s.chatFeedback(c, "Unknown item or vehicle/appliance definition.")
				}
			}
			b.Listings = append(b.Listings, store.BazaarListing{ItemID: uint16(id), Price: uint32(price), Quantity: uint32(stock)})
		}
		saved, err := s.Store.SaveBazaar(ctx, b)
		if err != nil {
			return s.chatFeedback(c, err.Error())
		}
		for _, peer := range s.world {
			if peer.tentOwner == 0 && peer.character.Map == b.Map {
				s.sendOrClose(peer, bazaarPackets(saved)...)
			}
		}
		return s.chatFeedback(c, fmt.Sprintf("Bazaar %d created.", saved.VendorID))
	}
	return s.chatFeedback(c, usage)
}
