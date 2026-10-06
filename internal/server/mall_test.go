package server

import (
	"bytes"
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
	"wonderland-gonline/internal/world"
)

func mallFixture(t *testing.T) (*Server, *Session, *captureConn, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mall.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	a, err := db.Register(context.Background(), "shopper", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	catalog := creationCatalog()
	catalog.Items[32177] = game.ItemDefinition{ID: 32177, Type: 23}
	catalog.Items[34333] = game.ItemDefinition{ID: 34333, Type: 23}
	catalog.Mall = []assets.MallItem{{ID: 32176, CategoryID: 4, Cost: 10, Count: 5, Order: 12, New: 1}, {ID: 32177, CategoryID: 4, Cost: 6, Count: 3, Order: 30, Bonus: 1}, {ID: 34333, Cost: 1, Count: 1}, {ID: 9999, Cost: 1, Count: 1}}
	c, err := game.NewCharacter(a.CharacterID(1), 1, "Shopper", game.Appearance{Body: 1, Element: 3}, nil, catalog.Items, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateCharacter(context.Background(), a, c); err != nil {
		t.Fatal(err)
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("UPDATE accounts SET im=100,im_bonus=30 WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	wire := &captureConn{}
	player := &Session{conn: wire, info: SessionInfo{ID: 1}, account: a, character: &c, view: world.NewView(), pets: newPetRoster(), ready: true}
	return s, player, wire, path
}
func mallCart(sub byte, rows ...mallCartRow) []byte {
	p := protocol.Builder{75, sub, byte(len(rows))}
	for _, r := range rows {
		p = p.U16(r.item).U8(r.category).U8(r.quantity).U16(r.order)
	}
	return p
}
func TestMallNativeCatalogAndBalances(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	tradeDo(t, s, c, []byte{75, 2})
	p := wire.packets(t)
	expected := [][]byte{
		{75, 1, 1, 0, 0xb0, 0x7d, 5, 10, 0, 100, 1, 4, 12, 0},
		{75, 10, 1, 0, 0xb1, 0x7d, 3, 6, 0, 100, 0, 4, 30, 0},
		{75, 3, 100, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{75, 9, 30, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}
	if len(p) != len(expected) {
		t.Fatal(p)
	}
	for i := range p {
		if !bytes.Equal(p[i], expected[i]) {
			t.Fatal(i, p[i], expected[i])
		}
	}
	if err := s.sendInitialMallSync(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	p = wire.packets(t)
	if len(p) != 6 || !bytes.Equal(p[2], []byte{75, 8, 0, 0}) || !bytes.Equal(p[3], []byte{75, 7, 1}) {
		t.Fatal(p)
	}
}
func TestMallCartBundlesBonusAndLegacyPurchase(t *testing.T) {
	s, c, wire, path := mallFixture(t)
	tradeDo(t, s, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 2, order: 12}))
	p := wire.packets(t)
	if c.character.Bag[0].Count != 10 || c.account.IM != 80 || c.account.IMBonus != 30 || !contains(p, []byte{75, 4, 0xb0, 0x7d, 4, 2, 12, 0, 1}) {
		t.Fatal(c.character.Bag, c.account, p)
	}
	if len(p) < 1 || p[0][0] != 23 || p[0][1] != 5 {
		t.Fatal("delivery receipt missing", p)
	}
	tradeDo(t, s, c, mallCart(5, mallCartRow{item: 32177, category: 4, quantity: 1, order: 30}))
	wire.Reset()
	if c.account.IM != 80 || c.account.IMBonus != 24 || c.character.Bag[1].Count != 3 {
		t.Fatal(c.account, c.character.Bag)
	}
	// Legacy omitted/zero quantity means one bundle.
	tradeDo(t, s, c, []byte{23, 26, 0xb0, 0x7d, 0})
	wire.Reset()
	if c.account.IM != 70 || c.character.Bag[0].Count != 15 {
		t.Fatal(c.account, c.character.Bag)
	}
	s.Store.Close()
	reopened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	balance, err := reopened.MallBalances(context.Background(), c.account.ID)
	if err != nil || balance.Points != 70 || balance.Bonus != 24 {
		t.Fatal(balance, err)
	}
	chars, err := reopened.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag[0].Count != 15 || chars[0].Bag[1].Count != 3 {
		t.Fatal(chars, err)
	}
}
func TestMallRejectsInvalidCartsWithoutMutatingState(t *testing.T) {
	for _, kind := range []string{"category", "order", "zero", "unknown", "pack", "currency", "funds", "full", "partial"} {
		t.Run(kind, func(t *testing.T) {
			s, c, wire, _ := mallFixture(t)
			row := mallCartRow{item: 32176, category: 4, quantity: 1, order: 12}
			sub := byte(1)
			switch kind {
			case "category":
				row.category = 3
			case "order":
				row.order++
			case "zero":
				row.quantity = 0
			case "unknown":
				row.item = 9999
			case "pack":
				row.item = 34333
				row.category = 1
				row.order = 1
			case "currency":
				sub = 5
			case "funds":
				row.quantity = 11
			case "full", "partial":
				for i := range c.character.Bag {
					c.character.Bag[i] = game.Item{ID: 32177, Count: 50}
				}
				if kind == "partial" {
					c.character.Bag[0] = game.Item{ID: 32176, Count: 49}
				}
				if err := s.commit(context.Background(), c, *c.character); err != nil {
					t.Fatal(err)
				}
			}
			before := c.character.Clone()
			tradeDo(t, s, c, mallCart(sub, row))
			p := wire.packets(t)
			if c.character.Bag != before.Bag {
				t.Fatal("partial cart delivered")
			}
			balance, err := s.Store.MallBalances(context.Background(), c.account.ID)
			if err != nil || balance.Points != 100 || balance.Bonus != 30 {
				t.Fatal(balance, err)
			}
			chars, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil || chars[0].Bag != before.Bag {
				t.Fatal("partial cart persisted", err)
			}
			for _, packet := range p {
				if len(packet) > 1 && packet[0] == 23 && packet[1] == 5 {
					t.Fatal("rejected cart emitted delivery", p)
				}
				if len(packet) == 9 && packet[0] == 75 && packet[1] == 4 && packet[8] != 0 {
					t.Fatal("rejected cart acknowledged success", p)
				}
			}
		})
	}
}
func TestMallMalformedAndDatabaseFailureEmitNoSuccess(t *testing.T) {
	for _, p := range [][]byte{{75}, {75, 1}, {75, 1, 0}, {75, 1, 8}, {75, 1, 1}, {75, 2, 0}, {23, 26}, {23, 26, 1}, {23, 25, 0}} {
		s, c, wire, _ := mallFixture(t)
		if err := s.dispatch(context.Background(), c, p); err == nil {
			t.Fatal("malformed cart accepted", p)
		}
		if wire.Len() != 0 {
			t.Fatal("malformed request emitted packets", p)
		}
	}
	s, c, wire, _ := mallFixture(t)
	s.Store.Close()
	before := c.character.Clone()
	if err := s.dispatch(context.Background(), c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12})); err == nil {
		t.Fatal("failed database save ignored")
	}
	if wire.Len() != 0 || c.character.Bag != before.Bag {
		t.Fatal("failed save published success")
	}
}

func TestMallCartLaterItemFailureRollsBackEarlierDelivery(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	// The first item can fill this partial stack; the second has no free slot.
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 32176, Count: 50}
	}
	c.character.Bag[0].Count = 45
	if err := s.commit(context.Background(), c, *c.character); err != nil {
		t.Fatal(err)
	}
	s.Assets.Mall = append(s.Assets.Mall, assets.MallItem{ID: 32177, CategoryID: 4, Cost: 6, Count: 3, Order: 30})
	before := c.character.Clone()
	tradeDo(t, s, c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12}, mallCartRow{item: 32177, category: 4, quantity: 1, order: 30}))
	if c.character.Bag != before.Bag {
		t.Fatal("first cart item escaped rollback")
	}
	balance, err := s.Store.MallBalances(context.Background(), c.account.ID)
	if err != nil || balance.Points != 100 {
		t.Fatal(balance, err)
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].Bag != before.Bag {
		t.Fatal("first item persisted", err)
	}
	for _, p := range wire.packets(t) {
		if len(p) == 9 && p[0] == 75 && p[1] == 4 && p[8] != 0 {
			t.Fatal("partial cart acknowledged success", p)
		}
	}
}
func TestMallPurchasesRespectGameplayOwnership(t *testing.T) {
	for _, state := range []string{"loading", "battle", "minigame", "trade"} {
		t.Run(state, func(t *testing.T) {
			s, c, wire, _ := mallFixture(t)
			switch state {
			case "loading":
				c.ready = false
			case "battle":
				c.battle = &battleRun{}
			case "minigame":
				c.event = &eventSession{onMinigame: func(byte) error { t.Fatal("mall consumed event"); return nil }}
			case "trade":
				c.trade = &tradeSession{}
			}
			err := s.dispatch(context.Background(), c, mallCart(1, mallCartRow{item: 32176, category: 4, quantity: 1, order: 12}))
			if state == "loading" && err == nil {
				t.Fatal("purchase before world acknowledgment accepted")
			}
			if state != "loading" && err != nil {
				t.Fatal(err)
			}
			balance, err := s.Store.MallBalances(context.Background(), c.account.ID)
			if err != nil || balance.Points != 100 {
				t.Fatal(balance, err)
			}
			if c.character.Bag[0].ID != 0 {
				t.Fatal("busy character received items")
			}
			for _, p := range wire.packets(t) {
				if len(p) > 1 && p[0] == 75 {
					t.Fatal("blocked purchase acknowledged", p)
				}
			}
		})
	}
}
