package store

import (
	"context"
	"errors"
	"testing"
	"wonderland-go/internal/game"
)

func TestAdminCharacterConflictIdentityAuditAndCleanup(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	row, err := db.AdminCharacter(ctx, refs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	next := row.State.Clone()
	next.Gold = 17
	if err := db.ReplaceAdminCharacter(ctx, next.ID, "", next); !errors.Is(err, ErrAdminConflict) {
		t.Fatal(err)
	}
	bad := next.Clone()
	bad.Name = "Changed"
	if err := db.ReplaceAdminCharacter(ctx, next.ID, row.Version, bad); err == nil {
		t.Fatal("identity changed")
	}
	if err := db.ReplaceAdminCharacter(ctx, next.ID, row.Version, next); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAdminCharacter(ctx, next.ID, row.Version); !errors.Is(err, ErrAdminConflict) {
		t.Fatal("stale deletion", err)
	}
	guild := AdminGuild{Name: "Test Guild", LeaderID: refs[0].ID, Members: []uint32{refs[0].ID, refs[1].ID}}
	if err := db.SaveAdminGuild(ctx, guild); err != nil {
		t.Fatal(err)
	}
	current, err := db.AdminCharacter(ctx, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteAdminCharacter(ctx, next.ID, current.Version); err != nil {
		t.Fatal(err)
	}
	guilds, err := db.AdminGuilds(ctx)
	if err != nil || len(guilds) != 1 || guilds[0].LeaderID != refs[1].ID || len(guilds[0].Members) != 1 {
		t.Fatal(guilds, err)
	}
	if err := db.DeleteAccount(ctx, refs[1].Account); err != nil {
		t.Fatal(err)
	}
	guilds, err = db.AdminGuilds(ctx)
	if err != nil || len(guilds) != 0 {
		t.Fatal("empty guild survived", guilds, err)
	}
	audit, err := db.Audit(ctx)
	if err != nil || len(audit) < 4 {
		t.Fatal(audit, err)
	}
}
func TestAdminMailAtomicBatchIdempotentClaimAndFullBag(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	message := AdminMail{Subject: "Gift", Body: "Hello", Gold: 9, ItemID: 42, Count: 2}
	if err := db.SendAdminMail(ctx, []uint32{refs[0].ID, 999999}, message); err == nil {
		t.Fatal("unknown recipient accepted")
	}
	rows, err := db.AdminMailHistory(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatal("partial dispatch", rows, err)
	}
	if err := db.SendAdminMail(ctx, []uint32{refs[0].ID, refs[1].ID}, message); err != nil {
		t.Fatal(err)
	}
	rows, err = db.PendingAdminMail(ctx, refs[0].ID)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	id := rows[0].ID
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error {
		for i := range c.Bag {
			c.Bag[i] = game.Item{ID: 99, Count: 1}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ClaimAdminMail(ctx, refs[0], id, 50, nil); !errors.Is(err, game.ErrInventoryFull) {
		t.Fatal(err)
	}
	if pairGold(t, db, refs)[0] != 100 {
		t.Fatal("failed grant changed gold")
	}
	if err := db.UpdateCharacter(ctx, refs[0].Account, refs[0].ID, func(c *game.Character) error { c.Bag[0] = game.Item{}; return nil }); err != nil {
		t.Fatal(err)
	}
	c, adds, err := db.ClaimAdminMail(ctx, refs[0], id, 50, nil)
	if err != nil || c.Gold != 109 || c.Bag[0].Count != 2 || len(adds) != 1 {
		t.Fatal(c, adds, err)
	}
	c, adds, err = db.ClaimAdminMail(ctx, refs[0], id, 50, nil)
	if err != nil || c.Gold != 109 || c.Bag[0].Count != 2 || len(adds) != 0 {
		t.Fatal("duplicate gift", c, adds, err)
	}
	if err := db.MarkAdminMailDelivered(ctx, refs[0].ID, id); err != nil {
		t.Fatal(err)
	}
	rows, err = db.PendingAdminMail(ctx, refs[0].ID)
	if err != nil || len(rows) != 0 {
		t.Fatal(rows, err)
	}
}
func TestAdminGuildMembershipAndIPCanonicalization(t *testing.T) {
	db, refs := pairFixture(t)
	ctx := context.Background()
	if err := db.SaveAdminGuild(ctx, AdminGuild{Name: "One", LeaderID: refs[0].ID, Members: []uint32{refs[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveAdminGuild(ctx, AdminGuild{Name: "Two", LeaderID: refs[0].ID, Members: []uint32{refs[0].ID, refs[1].ID}}); err == nil {
		t.Fatal("two guilds accepted")
	}
	rows, err := db.AdminGuilds(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if err := db.SetIPBan(ctx, "::ffff:127.0.0.1", "Test", true); err != nil {
		t.Fatal(err)
	}
	bans, err := db.IPBans(ctx)
	if err != nil || len(bans) != 1 || bans[0].IP != "127.0.0.1" {
		t.Fatal(bans, err)
	}
	if err := db.SetIPBan(ctx, "127.0.0.1", "", false); err != nil {
		t.Fatal(err)
	}
	bans, err = db.IPBans(ctx)
	if err != nil || len(bans) != 0 {
		t.Fatal(bans, err)
	}
}
