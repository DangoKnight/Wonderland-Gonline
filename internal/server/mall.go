package server

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	serverwire "wonderland-gonline/internal/server/wire"
	"wonderland-gonline/internal/store"
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

func (s *Server) sendMallBalances(ctx context.Context, c *Session) error {
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	return s.sendAll(c, serverwire.MallBalancePackets(balances))
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
		return c.send(serverwire.HeadBanner("Item Mall: this item is unavailable. No points deducted."))
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
		if err := c.send(serverwire.HeadBanner(message)); err != nil {
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
		if err := c.send(serverwire.HeadBanner("Item Mall: insufficient points. No points deducted.")); err != nil {
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
	if err := s.sendAll(c, serverwire.MallBalancePackets(balances)); err != nil {
		return err
	}
	if err := c.send(serverwire.HeadBanner("Item Mall: purchase successful.")); err != nil {
		return err
	}
	if receipts {
		return s.mallReceipts(c, cart, true)
	}
	return nil
}

// AdjustMallBalance is the administrator operation. Exclude account deletion and
// authentication publication, then order balance receipts against purchases.
func (s *Server) AdjustMallBalance(ctx context.Context, account uint32, bonus bool, delta int64) (store.MallBalances, error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	return s.adjustMallBalance(ctx, account, bonus, delta, "administrator")
}

// Caller holds worldMu. A receipt failure closes that client; the committed
// adjustment still succeeds so an administrator is not invited to retry a grant.
func (s *Server) adjustMallBalance(ctx context.Context, account uint32, bonus bool, delta int64, actor string) (store.MallBalances, error) {
	balances, err := s.Store.AdjustMallBalance(ctx, account, bonus, delta, actor)
	if err != nil {
		return store.MallBalances{}, err
	}
	for _, c := range s.world {
		if c.account.ID != account || !c.ready {
			continue
		}
		c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
		for _, p := range serverwire.MallBalancePackets(balances) {
			s.sendOrClose(c, p)
		}
	}
	return balances, nil
}

// gmMallPoints ports AC02's :im/:points_im/:mallpoints and GmManager.AddMallPoints.
// Missing explicit targets are refused, fixing the reference's self-grant fallback.
func (s *Server) gmMallPoints(ctx context.Context, c *Session, words []string) error {
	if len(words) < 2 {
		return nil
	}
	delta, err := strconv.ParseInt(words[1], 10, 32)
	if err != nil || delta == 0 {
		return nil
	}
	target := c
	if len(words) > 2 {
		target = s.findOnline(words[2])
		if target == nil {
			return nil
		}
	}
	_, err = s.adjustMallBalance(ctx, target.account.ID, false, delta, fmt.Sprintf("gm:%d/character:%d", c.account.ID, c.character.ID))
	if errors.Is(err, store.ErrMallAdjustment) {
		return nil
	}
	return err
}

// mallCheckoutCommand implements AC34:1 under worldMu. Catalog refreshes clear
// the client's pending cart, so this request must emit only balance/resume packets.
// The read-only query uses the registry's normal world interaction gates.
func (s *Server) mallCheckoutCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.MallCheckoutRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.MallCheckoutBalanceRequest {
		return ErrUnsupported
	}
	switch p[2] {
	case protocol.MallCheckoutModeWireCode0, protocol.MallCheckoutModeWireCode1:
	default:
		return protocol.ErrMalformed
	}
	balances, err := s.Store.MallBalances(ctx, c.account.ID)
	if err != nil {
		return err
	}
	c.account.IM, c.account.IMBonus = balances.Points, balances.Bonus
	return s.sendAll(c, serverwire.MallCheckoutPackets(balances))
}

// FUN_00175cf0 consumes used count, reward count, then UInt16 ID/byte quantity
// rows. The client derives remaining draws as three minus used, not a balance.
func (s *Server) luckyDrawCatalogPacket(c *game.Character, now time.Time) []byte {
	pool := s.Assets.LuckyDraw
	used := game.LuckyDrawsPerDay - c.LuckyDraw.Remaining(now)
	p := protocol.Builder{protocol.CommandLuckyDraw, protocol.LuckyDrawMode, protocol.LuckyDrawCatalog, used, byte(len(pool.Rewards))}
	for _, reward := range pool.Rewards {
		p = p.U16(reward.ID).U8(byte(reward.Quantity))
	}
	return p
}

func (s *Server) luckyDrawCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.LuckyDrawRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.LuckyDrawSpin {
		return ErrUnsupported
	}
	s.Log.Debug("Lucky Draw spin requested", "session", c.info.ID, "character", c.character.ID, "map", c.character.Map, "event_active", c.event != nil, "storm", c.storm, "beach", c.beach != nil)
	return s.drawLucky(ctx, c, time.Now())
}

// Caller holds worldMu. Store rechecks durable allowance and inventory capacity.
func (s *Server) drawLucky(ctx context.Context, c *Session, now time.Time) error {
	if c.event != nil || c.storm || c.beach != nil {
		s.Log.Debug("Lucky Draw ignored", "session", c.info.ID, "reason", "active interaction")
		return nil
	}
	pool := s.Assets.LuckyDraw
	if pool.TotalWeight <= 0 {
		s.Log.Debug("Lucky Draw unavailable", "session", c.info.ID, "reason", "empty reward pool")
		return c.send(serverwire.HeadBanner("Lucky Draw rewards are unavailable."))
	}
	roll, err := rand.Int(rand.Reader, big.NewInt(pool.TotalWeight))
	if err != nil {
		return err
	}
	reward, ok := pool.RewardForRoll(roll.Int64())
	if !ok {
		return errors.New("invalid Lucky Draw pool")
	}
	limit, known := s.stackLimit(reward.ID)
	if !known {
		return errors.New("unknown Lucky Draw reward")
	}
	next, adds, err := s.Store.DrawLucky(ctx, store.CharacterRef{Account: c.account.ID, ID: c.character.ID}, reward.ID, reward.Quantity, limit, now, s.Assets.Items, c.character.Clone())
	if errors.Is(err, store.ErrLuckyDrawLimit) {
		s.Log.Debug("Lucky Draw refused", "session", c.info.ID, "reason", "daily limit")
		c.character.LuckyDraw = next.LuckyDraw
		return s.sendAll(c, [][]byte{serverwire.HeadBanner("You have used all three Lucky Draws for today."), s.luckyDrawCatalogPacket(c.character, now)})
	}
	if errors.Is(err, game.ErrInventoryFull) {
		s.Log.Debug("Lucky Draw refused", "session", c.info.ID, "reason", "inventory full")
		return c.send(serverwire.HeadBanner("Please free an inventory slot. Lucky Draw retained."))
	}
	if err != nil {
		return err
	}
	s.adoptSavedCharacter(c, next)
	s.Log.Debug("Lucky Draw committed", "session", c.info.ID, "character", next.ID, "reward", reward.ID, "quantity", reward.Quantity, "result_slot", reward.Slot, "remaining", next.LuckyDraw.Remaining(now))
	// Persist first. Failed receipts cannot restore a consumed draw or duplicate its reward.
	return s.sendAll(c, [][]byte{
		next.Bag.AdditionPacket(adds),
		{protocol.CommandLuckyDraw, protocol.LuckyDrawMode, protocol.LuckyDrawResult, reward.Slot, game.LuckyDrawsPerDay - next.LuckyDraw.Remaining(now)},
		serverwire.HeadBanner(fmt.Sprintf("Lucky Draw: received [%s] x%d.", s.itemName(reward.ID), reward.Quantity)),
	})
}

// Refresh remaining allowance for ready characters at UTC midnight. No state is
// saved until a draw succeeds; reset eligibility is derived from the saved day.
func (s *Server) refreshLuckyDrawCatalog(now time.Time) {
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	if s.Assets.LuckyDraw.TotalWeight <= 0 {
		return
	}
	for _, c := range s.world {
		if !c.ready {
			continue
		}
		if err := c.send(s.luckyDrawCatalogPacket(c.character, now)); err != nil {
			c.conn.Close()
		}
	}
}

func (s *Server) runLuckyDrawResets(ctx context.Context) {
	for {
		now := time.Now().UTC()
		next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case now := <-timer.C:
			s.refreshLuckyDrawCatalog(now)
		}
	}
}

// packContentsCommand is native AC91:1. Preview is read-only, requires no owned
// pack and always returns the current list rather than trusting the cache hint.
func (s *Server) packContentsCommand(c *Session, p []byte) error {
	if len(p) != protocol.PackContentsRequestBytes {
		return protocol.ErrMalformed
	}
	if p[1] != protocol.PackContentsRequest {
		return ErrUnsupported
	}
	id := protocol.NewReader(p[2:]).U16()
	packet := protocol.Builder{protocol.CommandPackContents, protocol.PackContentsResponse}.U16(id).U8(protocol.PackContentsVersion)
	for _, reward := range s.Assets.GachaPacks[id].Rewards {
		packet = packet.U16(reward.ID).U8(byte(reward.Quantity))
	}
	return c.send(packet)
}

// openPackCommand ports native AC23:75/128. The operand is a full UInt16 slot;
// truncating to a byte would turn malformed large slots into valid inventory IDs.
func (s *Server) openPackCommand(ctx context.Context, c *Session, p []byte) error {
	if len(p) != protocol.PackOpenRequestBytes {
		return protocol.ErrMalformed
	}
	slot := protocol.NewReader(p[2:]).U16()
	if slot < 1 || slot > game.BagSize {
		return nil
	}
	handled, err := s.openGachaPack(ctx, c, byte(slot))
	if err != nil || handled {
		return err
	}
	if handled, err := s.openBoxedItem(ctx, c, byte(slot)); handled || err != nil {
		return err
	}
	return c.send(serverwire.HeadBanner("This pack has no configured rewards. Item retained."))
}

// openGachaPack is GachaManager.TryOpen with commit-before-receipt ordering.
// Caller holds worldMu. One draw consumes one pack; rewards use fresh metadata.
func (s *Server) openGachaPack(ctx context.Context, c *Session, slot byte) (bool, error) {
	if slot < 1 || slot > game.BagSize {
		return false, nil
	}
	item := c.character.Bag[slot-1]
	if !s.Assets.IsGachaPack(item.ID) {
		return false, nil
	}
	if item.Locked {
		return true, c.send(serverwire.HeadBanner(game.ErrItemLocked.Error()))
	}
	if item.Empty() || c.event != nil || c.battle != nil || c.storm || c.beach != nil || (c.character.ActiveVehicle != 0 && c.character.VehicleSlot == slot) {
		return true, nil
	}
	if !s.Assets.GachaAvailable(item.ID) {
		return true, c.send(serverwire.HeadBanner("This pack's rewards are unavailable. Pack retained."))
	}
	// crypto/rand.Int uses rejection sampling: each weight slot is equally likely.
	draw, err := rand.Int(rand.Reader, big.NewInt(game.GachaWeightTotal))
	if err != nil {
		return true, err
	}
	reward, ok := s.Assets.GachaPacks[item.ID].RewardForRoll(int(draw.Int64()))
	if !ok {
		return true, c.send(serverwire.HeadBanner("This pack's rewards are unavailable. Pack retained."))
	}
	limit, known := s.stackLimit(reward.ID)
	if !known || reward.Quantity <= 0 || reward.Quantity > game.GachaMaxRewardQuantity {
		return true, c.send(serverwire.HeadBanner("This pack's rewards are unavailable. Pack retained."))
	}
	next := c.character.Clone()
	if err := next.Bag.Remove(slot, game.GachaPacksPerOpening); err != nil {
		return true, err
	}
	adds, err := next.Bag.Grant(game.Item{ID: reward.ID}, reward.Quantity, limit, s.Assets.Items)
	if err != nil {
		return true, c.send(serverwire.HeadBanner("Please free an inventory slot. Pack retained."))
	}
	if err := s.commit(ctx, c, next); err != nil {
		return true, err
	}
	return true, s.sendAll(c, [][]byte{
		{protocol.CommandInventory, protocol.InventoryRemove, slot, game.GachaPacksPerOpening},
		next.Bag.AdditionPacket(adds),
		{protocol.CommandInventory, protocol.InventoryItemUse},
		serverwire.HeadBanner(fmt.Sprintf("Gacha: received [%s] x%d.", s.itemName(reward.ID), reward.Quantity)),
	})
}
