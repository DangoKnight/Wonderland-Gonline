package server

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/protocol"
	"wonderland-go/internal/store"
)

type mallCartRow struct {
	item               uint16
	category, quantity byte
	order              uint16
}

func mallCategory(entry assets.MallItem) byte {
	if entry.CategoryID > 0 {
		return byte(entry.CategoryID)
	}
	category := strings.ToLower(strings.TrimSpace(entry.Category))
	contains := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(category, word) {
				return true
			}
		}
		return false
	}
	switch {
	case category == "2" || category == "armory" || category == "armor" || category == "armors" || contains("cloth", "shield", "helm", "boot"):
		return protocol.MallCategoryArmory
	case category == "3" || category == "weaponry" || category == "weapon" || category == "weapons" || contains("sword", "gun", "bow", "wand", "staff"):
		return protocol.MallCategoryWeaponry
	case category == "4" || category == "grocery" || category == "groceries" || category == "consumable" || category == "consumables" || contains("pot", "pill", "scroll", "gem", "spar", "oil", "diamond", "food", "rice"):
		return protocol.MallCategoryGrocery
	case category == "5" || category == "furniture" || category == "furn" || category == "tent" || category == "house" || category == "vehic" || category == "mount":
		return protocol.MallCategoryFurniture
	case category == "6" || contains("slot", "machine", "minigame"):
		return protocol.MallCategoryGames
	case category == "7" || contains("forg", "refin"):
		return protocol.MallCategoryForging
	default:
		return protocol.MallCategoryHot
	}
}
func mallOrder(entry assets.MallItem) uint16 {
	if entry.Order > 0 {
		return uint16(entry.Order)
	}
	return uint16(max(1, entry.Cost))
}

func (s *Server) mallEntries(bonus bool) []assets.MallItem {
	entries := []assets.MallItem{}
	for _, entry := range s.Assets.Mall {
		_, known := s.Assets.Items[entry.ID]
		if (entry.Bonus > 0) != bonus || !known || !s.Assets.GachaAvailable(entry.ID) || entry.ID == 0 || entry.Count == 0 || entry.Cost < 0 || entry.Cost > math.MaxUint16 || entry.Order < 0 || entry.Order > math.MaxUint16 || entry.CategoryID < 0 || entry.CategoryID > math.MaxUint8 {
			continue
		}
		entries = append(entries, entry)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Order < entries[j].Order })
	// A catalog is one frame; do not advertise rows the client cannot receive.
	limit := (protocol.MaxPayload - protocol.MallCatalogHeaderBytes) / protocol.MallCatalogRecordBytes
	return entries[:min(len(entries), limit)]
}
func (s *Server) mallCatalogPacket(bonus bool) []byte {
	entries := s.mallEntries(bonus)
	sub := byte(protocol.MallPointsCatalog)
	if bonus {
		sub = protocol.MallBonusCatalog
	}
	p := protocol.Builder{protocol.CommandMall, sub}.U16(uint16(len(entries)))
	for _, entry := range entries {
		badge := entry.Badge
		if badge == 0 {
			switch {
			case entry.New > 0:
				badge = protocol.MallBadgeNew
			case entry.Hot > 0:
				badge = protocol.MallBadgeHot
			case entry.Limited > 0:
				badge = protocol.MallBadgeLimited
			}
		}
		p = p.U16(entry.ID).U8(entry.Count).U16(uint16(entry.Cost)).U8(protocol.MallNoDiscount).U8(byte(max(0, min(badge, math.MaxUint8)))).U8(mallCategory(entry)).U16(mallOrder(entry))
	}
	return p
}

// mallWireBalance uses the native signed point range for every balance reply.
func mallWireBalance(value int64) uint32 {
	return uint32(max(0, min(value, int64(game.MaxMallPoints))))
}

func mallBalancePackets(balances store.MallBalances) [][]byte {
	packet := func(sub byte, value int64) []byte {
		return protocol.Builder{protocol.CommandMall, sub}.U32(mallWireBalance(value)).U32(0).U16(0).U8(0)
	}
	return [][]byte{packet(protocol.MallPointsBalance, balances.Points), packet(protocol.MallBonusBalance, balances.Bonus)}
}
func (s *Server) sendMallBalances(ctx context.Context, c *Session) error {
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	return s.sendAll(c, mallBalancePackets(balances))
}
func (s *Server) sendMallCatalogs(ctx context.Context, c *Session, initial bool) error {
	packets := [][]byte{s.mallCatalogPacket(false), s.mallCatalogPacket(true)}
	if initial {
		packets = append(packets, []byte{protocol.CommandMall, protocol.MallSettings, 0, 0}, []byte{protocol.CommandMall, protocol.MallStatus, protocol.MallStatusEnabled})
	}
	if err := s.sendAll(c, packets); err != nil {
		return err
	}
	return s.sendMallBalances(ctx, c)
}
func (s *Server) sendInitialMallSync(ctx context.Context, c *Session) error {
	return s.sendMallCatalogs(ctx, c, true)
}

func (s *Server) mallCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) < 2 {
		return protocol.ErrMalformed
	}
	switch p[1] {
	case protocol.MallRefresh:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendMallCatalogs(ctx, c, false)
	case protocol.MallGameCategory:
		return s.mallGameCommand(ctx, c, p)
	case protocol.MallForge:
		return s.forgeCommand(ctx, c, p)
	case protocol.MallBuyPoints, protocol.MallBuyBonus:
		if len(p) < 3 {
			return protocol.ErrMalformed
		}
		rows := int(p[2])
		if rows < 1 || rows > protocol.MallCartMaxRows || len(p) != 3+rows*protocol.MallCartRowBytes {
			return protocol.ErrMalformed
		}
		r := protocol.NewReader(p[3:])
		cart := make([]mallCartRow, rows)
		for i := range cart {
			cart[i] = mallCartRow{item: r.U16(), category: r.U8(), quantity: r.U8(), order: r.U16()}
		}
		if r.Err() != nil {
			return r.Err()
		}
		return s.purchaseMall(ctx, c, cart, p[1] == protocol.MallBuyBonus, true)
	default:
		return ErrUnsupported
	}
}

func (s *Server) legacyMallCommand(ctx context.Context, c *Session, p []byte) error {
	switch p[1] {
	case protocol.InventoryMallBalance:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendMallBalances(ctx, c)
	case protocol.InventoryMallCatalog:
		if len(p) != 2 {
			return protocol.ErrMalformed
		}
		return s.sendMallCatalogs(ctx, c, false)
	case protocol.InventoryMallBuy:
		if len(p) != 4 && len(p) != 5 {
			return protocol.ErrMalformed
		}
		item := uint16(p[2]) | uint16(p[3])<<8
		quantity := byte(1)
		if len(p) == 5 && p[4] > 0 {
			quantity = p[4]
		}
		for _, entry := range s.mallEntries(false) {
			if entry.ID == item {
				return s.purchaseMall(ctx, c, []mallCartRow{{item: item, category: mallCategory(entry), quantity: quantity, order: mallOrder(entry)}}, false, false)
			}
		}
		return c.send(headBanner("Item Mall: this item is unavailable. No points deducted."))
	}
	return ErrUnsupported
}
func (s *Server) mallReceipts(c *Session, cart []mallCartRow, success bool) error {
	status := byte(protocol.MallPurchaseFailed)
	if success {
		status = protocol.MallPurchaseSucceeded
	}
	for _, row := range cart {
		if err := c.send(protocol.Builder{protocol.CommandMall, protocol.MallCartReceipt}.U16(row.item).U8(row.category).U8(row.quantity).U16(row.order).U8(status)); err != nil {
			return err
		}
	}
	return nil
}
func (s *Server) purchaseMall(ctx context.Context, c *Session, cart []mallCartRow, bonus, receipts bool) error {
	reject := func(message string) error {
		if err := c.send(headBanner(message)); err != nil {
			return err
		}
		if receipts {
			return s.mallReceipts(c, cart, false)
		}
		return nil
	}
	entries := s.mallEntries(bonus)
	amounts := map[uint16]int{}
	ids := []uint16{}
	var cost int64
	for _, row := range cart {
		var entry *assets.MallItem
		for i := range entries {
			candidate := &entries[i]
			if candidate.ID == row.item && mallCategory(*candidate) == row.category && mallOrder(*candidate) == row.order {
				entry = candidate
				break
			}
		}
		if entry == nil || row.quantity == 0 {
			return reject("Item Mall: invalid or outdated cart. Please reopen the mall.")
		}
		cost += int64(entry.Cost) * int64(row.quantity)
		if _, found := amounts[row.item]; !found {
			ids = append(ids, row.item)
		}
		amounts[row.item] += int(entry.Count) * int(row.quantity)
	}
	var adds []game.Addition
	next, balances, err := s.Store.PurchaseMall(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, bonus, cost, func(character *game.Character) error {
		if err := game.PreserveItemLocks(*c.character, character); err != nil {
			return err
		}
		for _, id := range ids {
			granted, err := character.Bag.Grant(game.Item{ID: id}, amounts[id], s.Assets.Items[id].StackLimit(), s.Assets.Items)
			if err != nil {
				return err
			}
			adds = append(adds, granted...)
		}
		return nil
	})
	switch {
	case errors.Is(err, store.ErrMallFunds):
		if err := c.send(headBanner("Item Mall: insufficient points. No points deducted.")); err != nil {
			return err
		}
		if err := s.sendMallBalances(ctx, c); err != nil {
			return err
		}
		if receipts {
			return s.mallReceipts(c, cart, false)
		}
		return nil
	case errors.Is(err, game.ErrInventoryFull), errors.Is(err, game.ErrInvalidItem), errors.Is(err, store.ErrMallPurchase):
		return reject("Item Mall: cannot deliver this cart. Check free inventory space. No points deducted.")
	case err != nil:
		return err
	}
	s.adoptSavedCharacter(c, next)
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	if err := c.send(next.Bag.AdditionPacket(adds)); err != nil {
		return err
	}
	if err := s.sendAll(c, mallBalancePackets(balances)); err != nil {
		return err
	}
	if err := c.send(headBanner("Item Mall: purchase successful.")); err != nil {
		return err
	}
	if receipts {
		return s.mallReceipts(c, cart, true)
	}
	return nil
}
