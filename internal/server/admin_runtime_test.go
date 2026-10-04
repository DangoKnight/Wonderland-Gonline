package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/battle"
	"wonderland-go/internal/config"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
	"wonderland-go/internal/world"
)

func TestScaleExperienceNativeRoundingAndBounds(t *testing.T) {
	for _, tc := range []struct {
		amount uint64
		rate   float64
		want   uint64
	}{{0, 1000, 0}, {1, .01, 1}, {5, .5, 2}, {7, .5, 4}, {75, 2, 150}, {math.MaxUint64, 1000, math.MaxInt32}, {10, math.NaN(), 10}, {10, math.Inf(1), 10}, {10, 0, 10}} {
		if got := ScaleExperience(tc.amount, tc.rate); got != tc.want {
			t.Fatalf("%d * %v: %d want %d", tc.amount, tc.rate, got, tc.want)
		}
	}
}
func TestRuntimeEXPCommandPersistenceAndRejection(t *testing.T) {
	s, players, wires := chatFixture(t)
	c := players[0]
	ctx := context.Background()
	say(t, s, c, "/exprate 2")
	if s.RuntimeSettings().ExpRate != 1 || wires[0].Len() != 0 {
		t.Fatal("non-GM changed EXP rate")
	}
	c.gmLevel.Store(1)
	say(t, s, c, "/experience 2.125")
	if got := s.RuntimeSettings().ExpRate; got != 2.12 {
		t.Fatal(got)
	}
	for _, text := range []string{"/exprate NaN", "/exprate Inf", "/exprate 0", "/exprate 1001", "/exprate 2 3"} {
		say(t, s, c, text)
		if s.RuntimeSettings().ExpRate != 2.12 {
			t.Fatal(text)
		}
	}
	s.expRateMultiplier = 1
	if err := s.LoadRuntimeSettings(ctx); err != nil || s.RuntimeSettings().ExpRate != 2.12 {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	next := s.RuntimeSettings()
	next.ExpRate = 3
	if err := s.UpdateRuntimeSettings(canceled, next); !errors.Is(err, context.Canceled) || s.RuntimeSettings().ExpRate != 2.12 {
		t.Fatal("canceled update published", err)
	}
	settings, err := s.Store.Settings(ctx)
	if err != nil || settings["exp_rate"] != "2.12" {
		t.Fatal(settings, err)
	}
	c.gmLevel.Store(0)
	say(t, s, c, "/exprate 9")
	if s.RuntimeSettings().ExpRate != 2.12 {
		t.Fatal("revoked GM updated rate")
	}
}
func TestRuntimeStatusOverrideNativeBytes(t *testing.T) {
	s, _, _ := chatFixture(t)
	s.Config.StatusServerIDs = []uint16{101, 102}
	for mode, color := range map[string]byte{"offline": 0, "green": 1, "yellow": 2, "red": 3} {
		s.statusMode = mode
		got := s.launcherStatusPacket(0)
		want := StatusPacket(0, 101, 102)
		want[5], want[8] = color, color
		if !bytes.Equal(got, want) {
			t.Fatalf("%s %v want %v", mode, got, want)
		}
	}
	s.statusMode = "auto"
	if !bytes.Equal(s.launcherStatusPacket(500), StatusPacket(500, 101, 102)) {
		t.Fatal("auto status altered")
	}
}
func TestRuntimeBattleEXPScalesOwnerAndPetOnce(t *testing.T) {
	s, c, _ := battleFixture(t, 1)
	ctx := context.Background()
	settings := s.RuntimeSettings()
	settings.ExpRate = 2
	if err := s.UpdateRuntimeSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	run := c.battle
	pet := game.Pet{ID: 10500, Level: 100, HP: 10, MaxHP: 10}
	c.character.Pets = []game.Pet{pet}
	c.character.ActivePet = pet.ID
	run.members[0].pet = &battle.Fighter{Kind: battle.Pet, Pet: &pet, ID: pet.ID, HP: 10}
	before := c.character.EXP
	exp, _ := run.b.Rewards()
	s.endBattle(run, battle.Victory)
	s.worldMu.Unlock()
	settle(t, s, c)
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || uint64(chars[0].EXP-before) != exp*2 || uint64(chars[0].Pets[0].Exp) != exp*2 {
		t.Fatalf("reward owner/pet: %+v; base %d; %v", chars, exp, err)
	}
}
func TestAdminMailPreservesCheckpointAndClaimsOnce(t *testing.T) {
	s, p, _ := chatFixture(t)
	c := p[0]
	ctx := context.Background()
	itemBefore := adminTestItemCount(c.character.Bag, 32176)
	baseline := c.character.Clone()
	c.autosaveBaseline = &baseline
	c.character.X = 1234
	message := store.AdminMail{Subject: "Gift", Body: "Enjoy", Gold: 7, ItemID: 32176, Count: 1}
	if err := s.AdminDispatchMail(ctx, []uint32{c.character.ID}, message); err != nil {
		t.Fatal(err)
	}
	s.worldMu.Lock()
	err := s.deliverAdminMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || chars[0].X != 1234 || chars[0].Gold != 7 || adminTestItemCount(chars[0].Bag, 32176) != itemBefore+1 {
		t.Fatal(chars, err)
	}
	rows, err := s.Store.AdminMailHistory(ctx)
	if err != nil || len(rows) != 1 || !rows[0].Claimed || !rows[0].Delivered {
		t.Fatal(rows, err)
	}
	if err := s.autosaveSession(ctx, c); err != nil {
		t.Fatal("baseline not advanced", err)
	}
}
func TestAdminCharacterConflictValidationAndLoading(t *testing.T) {
	s, p, _ := chatFixture(t)
	c := p[0]
	ctx := context.Background()
	for _, skill := range c.character.Skills {
		s.Assets.Skills[skill.ID] = assets.Skill{}
	}
	row, err := s.Store.AdminCharacter(ctx, c.character.ID)
	if err != nil {
		t.Fatal(err)
	}
	bad := row.State.Clone()
	bad.Bag[0] = game.Item{ID: 65000, Count: 1}
	if err := s.EditAdminCharacter(ctx, bad.ID, row.Version, bad); err == nil {
		t.Fatal("unknown item accepted")
	}
	next := row.State.Clone()
	next.Gold = 13
	if err := s.EditAdminCharacter(ctx, next.ID, "stale", next); !errors.Is(err, store.ErrAdminConflict) {
		t.Fatal(err)
	}
	delete(s.friendSessions, c.character.ID)
	if err := s.EditAdminCharacter(ctx, next.ID, row.Version, next); !errors.Is(err, ErrAdminPlayerUnavailable) {
		t.Fatal("edited loading character", err)
	}
	if err := s.DeleteAdminCharacter(ctx, next.ID, row.Version); !errors.Is(err, ErrAdminPlayerUnavailable) {
		t.Fatal("deleted loading character", err)
	}
	s.friendSessions[c.character.ID] = c
	if err := s.EditAdminCharacter(ctx, next.ID, row.Version, next); err != nil {
		t.Fatal(err)
	}
	if c.character.Gold != 13 || c.autosaveBaseline != nil {
		t.Fatal("online edit not adopted")
	}
}
func TestAdminLogsConcurrentBoundedAndRedacted(t *testing.T) {
	s, _, _ := chatFixture(t)
	s.logs.SetLevel("debug")
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.Log.Debug("record", "password", "secret", "token", "secret")
				_ = s.Logs()
			}
		}()
	}
	wg.Wait()
	rows := s.Logs()
	if len(rows) != adminLogCapacity {
		t.Fatal(len(rows))
	}
	for _, row := range rows {
		if row.Fields["password"] != "[redacted]" || row.Fields["token"] != "[redacted]" {
			t.Fatal(row)
		}
	}
	rows[0].Fields["password"] = "edited"
	if s.Logs()[0].Fields["password"] != "[redacted]" {
		t.Fatal("snapshot aliases log buffer")
	}
	s.logs.SetLevel("error")
	s.Log.Info("filtered")
	if s.Logs()[adminLogCapacity-1].Message != "record" {
		t.Fatal("level filter")
	}
}
func TestAdminChestWeightedRewardCooldownAndExpiry(t *testing.T) {
	s, c, w := eventFixture(t)
	ctx := context.Background()
	s.Assets.ChestPools = []assets.ChestPool{{MapID: 10017, RespawnSeconds: 60, Rewards: []assets.ChestReward{{Item: 32176, Count: 2, Weight: 1}}}}
	itemBefore := adminTestItemCount(c.character.Bag, 32176)
	ev := s.Assets.Maps[10017].Events[3]
	es := &eventSession{mapID: 10017, click: 8, ev: &ev}
	if !s.canDeliver(c, es) {
		t.Fatal("reward refused")
	}
	ok, err := s.questItem(ctx, c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if !ok || err != nil || adminTestItemCount(c.character.Bag, 32176) != itemBefore+2 {
		t.Fatal(ok, err, c.character.Bag)
	}
	key := world.ChestKey(10017, 4)
	if c.character.ChestRespawns[key].Before(time.Now()) {
		t.Fatal("no durable cooldown")
	}
	w.Reset()
	ok, err = s.questItem(ctx, c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if !ok || err != nil || adminTestItemCount(c.character.Bag, 32176) != itemBefore+2 || w.Len() != 0 {
		t.Fatal("cooldown granted again", err)
	}
	c.character.ChestRespawns[key] = time.Now().Add(-time.Second)
	s.respawnChests(time.Now())
	if !contains(w.packets(t), []byte{22, 1, 4, 0, 0}) {
		t.Fatal("expired prop not closed")
	}
	es.chestReward = nil
	ok, err = s.questItem(ctx, c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if !ok || err != nil || adminTestItemCount(c.character.Bag, 32176) != itemBefore+4 {
		t.Fatal("expired chest failed", err)
	}
}

func adminTestItemCount(b game.Inventory, id uint16) int {
	count := 0
	for _, i := range b {
		if i.ID == id {
			count += int(i.Count)
		}
	}
	return count
}

func TestAdminStartupConfigurationAtomicConflictAndRestart(t *testing.T) {
	s, _, _ := chatFixture(t)
	path := filepath.Join(t.TempDir(), "config.json")
	s.SetConfigurationPath(path)
	current, err := s.StartupConfiguration()
	if err != nil {
		t.Fatal(err)
	}
	next := current.Configuration
	next.IdleSeconds = 123
	if err := s.SaveStartupConfiguration("stale", next); err == nil {
		t.Fatal("stale config accepted")
	}
	if err := s.SaveStartupConfiguration(current.Version, next); err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil || saved.IdleSeconds != 123 {
		t.Fatal(saved, err)
	}
	if s.Config.IdleSeconds == 123 {
		t.Fatal("startup edit changed active listeners")
	}
	if err := s.SaveStartupConfiguration(current.Version, next); err == nil {
		t.Fatal("old version overwrote saved config")
	}
}

func TestAdminChestFullBagPreservesCooldownAndChosenReward(t *testing.T) {
	s, c, _ := eventFixture(t)
	s.Assets.ChestPools = []assets.ChestPool{{MapID: 10017, RespawnSeconds: 60, Rewards: []assets.ChestReward{{Item: 32176, Count: 2, Weight: 1}}}}
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	ev := s.Assets.Maps[10017].Events[3]
	es := &eventSession{mapID: 10017, click: 8, ev: &ev}
	if s.canDeliver(c, es) {
		t.Fatal("full bag passed preflight")
	}
	if es.chestReward == nil || es.chestReward.Item != 32176 {
		t.Fatal("preflight did not select actual pool reward")
	}
	ok, err := s.questItem(context.Background(), c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if ok || err != nil || len(c.character.ChestRespawns) != 0 {
		t.Fatal("full bag consumed cooldown", ok, err)
	}
	c.character.Bag[0] = game.Item{}
	if !s.canDeliver(c, es) {
		t.Fatal("reward did not fit")
	}
	ok, err = s.questItem(context.Background(), c, es, world.DecodeOp(ev.Branches[0].Operations[0]))
	if !ok || err != nil || c.character.Bag[0].ID != 32176 || c.character.Bag[0].Count != 2 {
		t.Fatal("retry changed chosen reward", ok, err)
	}
}

func TestAdminIPBanPublicationAndReload(t *testing.T) {
	s, _, _ := chatFixture(t)
	ctx := context.Background()
	if err := s.AdminSetIPBan(ctx, "::ffff:127.0.0.2", "Test", true); err != nil {
		t.Fatal(err)
	}
	target := &net.TCPAddr{IP: net.ParseIP("127.0.0.2"), Port: 6415}
	if !s.ipBanned(target) {
		t.Fatal("mapped IP not applied")
	}
	if err := s.LoadIPBans(ctx); err != nil || !s.ipBanned(target) {
		t.Fatal("ban not durable", err)
	}
	if err := s.AdminSetIPBan(ctx, "127.0.0.2", "", false); err != nil || s.ipBanned(target) {
		t.Fatal("unban failed", err)
	}
}
func TestAdminNestedLogSecrets(t *testing.T) {
	s, _, _ := chatFixture(t)
	s.Log.Info("Nested", slog.Group("request", slog.String("password", "secret"), slog.String("token", "secret"), slog.String("name", "Alice")))
	rows := s.Logs()
	row := rows[len(rows)-1]
	if row.Fields["request.password"] != "[redacted]" || row.Fields["request.token"] != "[redacted]" || row.Fields["request.name"] != "Alice" {
		t.Fatal(row)
	}
}
func TestAdminWarpCharacterIDAndInvalidItem(t *testing.T) {
	s, p, _ := chatFixture(t)
	ctx := context.Background()
	origin := p[0].character.Map
	if err := s.AdminPlayerAction(ctx, p[0].character.ID, "item", []string{"65000", "1"}); err == nil {
		t.Fatal("unknown item silently accepted")
	}
	if err := s.AdminPlayerAction(ctx, p[0].character.ID, "warp", []string{fmt.Sprint(p[2].character.ID)}); err != nil {
		t.Fatal(err)
	}
	if p[0].character.Map == origin || p[0].character.Map != p[2].character.Map {
		t.Fatal("numeric character interpreted as map")
	}
}
func TestAdminEditedContentRejectsSilentInvalidRows(t *testing.T) {
	s, _, _ := chatFixture(t)
	s.Assets.NPCs = map[uint16]assets.NPC{42: {ID: 42}}
	for _, text := range []string{"garbage", "TID:42 | malformed", "TID:42 | 32176,Name,1,2,NaN", "TID:42 | 65000,Name,1,1,1"} {
		raw, _ := json.Marshal(map[string]string{"text": text})
		if err := validateAdminAsset("monster_drops.txt", raw, s.Assets); err == nil {
			t.Fatal(text)
		}
	}
	raw := []byte(`{"text":"TID:42 | 32176,Name,1,2,50"}`)
	if err := validateAdminAsset("monster_drops.txt", raw, s.Assets); err != nil {
		t.Fatal(err)
	}
}

func TestAdminIdentityPatchPreservesConcurrentEXPRate(t *testing.T) {
	s, _, _ := chatFixture(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if err := s.UpdateIdentitySettings(ctx, map[string]string{"server_name": "Renamed"}); err != nil {
				t.Error(err)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			s.worldMu.Lock()
			v := s.runtimeSettingsLocked()
			v.ExpRate = 2
			err := s.updateRuntimeSettingsLocked(ctx, v)
			s.worldMu.Unlock()
			if err != nil {
				t.Error(err)
			}
		}
	}()
	wg.Wait()
	v := s.RuntimeSettings()
	if v.Name != "Renamed" || v.ExpRate != 2 {
		t.Fatal(v)
	}
}

func TestAdminGiftReceiptFailureCannotDuplicateReward(t *testing.T) {
	s, p, wires := chatFixture(t)
	c := p[0]
	ctx := context.Background()
	before := adminTestItemCount(c.character.Bag, 32176)
	failed := &failedWorldConn{}
	c.conn = failed
	if err := s.AdminDispatchMail(ctx, []uint32{c.character.ID}, store.AdminMail{Subject: "Gift", Body: "Retry", Gold: 5, ItemID: 32176, Count: 1}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Store.AdminMailHistory(ctx)
	if err != nil || len(rows) != 1 || !rows[0].Claimed || rows[0].Delivered || !failed.closed {
		t.Fatal(rows, err)
	}
	c.conn = wires[0]
	s.worldMu.Lock()
	err = s.deliverAdminMail(ctx, c)
	s.worldMu.Unlock()
	if err != nil || c.character.Gold != 5 || adminTestItemCount(c.character.Bag, 32176) != before+1 {
		t.Fatal("duplicate gift on retry", err)
	}
	rows, err = s.Store.AdminMailHistory(ctx)
	if err != nil || !rows[0].Delivered {
		t.Fatal(rows, err)
	}
	for _, packet := range wires[0].packets(t) {
		if len(packet) >= 2 && packet[0] == 23 && packet[1] == 5 {
			t.Fatal("replayed additive item receipt")
		}
	}
}

func TestAdminChestPreflightPlansOnePoolReward(t *testing.T) {
	s, c, _ := eventFixture(t)
	s.Assets.ChestPools = []assets.ChestPool{{MapID: 10017, RespawnSeconds: 60, Rewards: []assets.ChestReward{{Item: 32176, Count: 2, Weight: 1}}}}
	for i := range c.character.Bag {
		c.character.Bag[i] = game.Item{ID: 24001, Count: 1}
	}
	c.character.Bag[0] = game.Item{ID: 32176, Count: 48}
	ev := s.Assets.Maps[10017].Events[3]
	ev.Branches = append([]assets.Branch{}, ev.Branches...)
	ev.Branches[0].Operations = append(append([]assets.Operation{}, ev.Branches[0].Operations...), evOp(2, 1, 1, 1, 24001, 1))
	es := &eventSession{mapID: 10017, click: 8, ev: &ev}
	if !s.canDeliver(c, es) {
		t.Fatal("planned pool reward twice")
	}
	for _, op := range ev.Branches[0].Operations {
		if ok, err := s.questItem(context.Background(), c, es, world.DecodeOp(op)); !ok || err != nil {
			t.Fatal(ok, err)
		}
	}
	if c.character.Bag[0].Count != 50 {
		t.Fatal("pool rewarded more than once")
	}
}

func TestAdminFullHealIncludesSelectedCompanion(t *testing.T) {
	s, p, _ := chatFixture(t)
	c := p[0]
	c.character.HP = 1
	c.character.SP = 0
	c.character.Pets = []game.Pet{{ID: 14156, Level: 1, HP: 1, MaxHP: 1, SP: 0, Base: game.Attributes{Constitution: 10, Wisdom: 10}}}
	c.pets.register(14156)
	if err := s.AdminPlayerAction(context.Background(), c.character.ID, "fullheal", nil); err != nil {
		t.Fatal(err)
	}
	chars, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || chars[0].HP != chars[0].MaxHP || chars[0].SP != chars[0].MaxSP || chars[0].Pets[0].HP != chars[0].Pets[0].MaxHP || chars[0].Pets[0].SP != chars[0].Pets[0].MaxSP {
		t.Fatal("owner/companion not healed", err)
	}
}
