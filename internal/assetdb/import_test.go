package assetdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-go/internal/store"
)

func fixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	sourceHash := strings.Repeat("a", 64)
	document := map[string]any{"schema_version": 1, "source_sha256": sourceHash, "items": []any{
		map[string]any{"definition": map[string]any{"id": 42, "name": "Sample Sword", "type": 2, "equip_slot": 1, "level": 7}, "unknown": 123},
		map[string]any{"definition": map[string]any{"id": 42, "name": "Duplicate ID retained", "type": 2, "equip_slot": 1, "level": 7}},
	}}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(data, "item_data.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(encoded)
	entry := Entry{Asset: "item.dat", Origin: "client", SourceSHA256: sourceHash, Output: "item_data.json", OutputBytes: int64(len(encoded)), OutputSHA256: hex.EncodeToString(digest[:])}
	// Audio metadata imports even when payload files are absent: the server never
	// needs them and the database must not embed them.
	audio := []byte(`{"source_sha256":"` + sourceHash + `","entries":[{"name":"test.ogg","file_offset":0,"bytes":9000000}]}`)
	if err = os.WriteFile(filepath.Join(data, "audio_index.json"), audio, 0600); err != nil {
		t.Fatal(err)
	}
	audioHash := sha256.Sum256(audio)
	audioEntry := Entry{Asset: "odd.dat", Origin: "client", SourceSHA256: sourceHash, Output: "audio_index.json", OutputBytes: int64(len(audio)), OutputSHA256: hex.EncodeToString(audioHash[:]), Payload: &Payload{Output: "missing.ogg", Bytes: 9000000, SHA256: sourceHash}}
	manifest := Manifest{SchemaVersion: 1, Priority: "client-over-server", Assets: []Entry{entry, audioEntry}}
	encoded, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(data, "asset_manifest.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	return root, data
}

func TestImportQueryableRecordsAndExactDocuments(t *testing.T) {
	root, data := fixture(t)
	path := filepath.Join(root, "assets.db")
	summary, err := Build(data, path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Assets != 2 || summary.Records != 3 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	var rows []struct {
		ID    int
		Name  string
		Level int
	}
	if err = db.Table("asset_items").Order("name").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != 42 || rows[1].Level != 7 {
		t.Fatalf("bad SQL projection: %+v", rows)
	}
	var document Document
	if err = db.Where(map[string]any{"asset": "item.dat"}).Take(&document).Error; err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(filepath.Join(data, "item_data.json"))
	if err != nil {
		t.Fatal(err)
	}
	if document.JSON != string(original) {
		t.Fatal("original JSON bytes changed")
	}
	var unknown int
	if err = db.Raw(`SELECT json_extract(json, '$.unknown') FROM asset_records WHERE asset='item.dat' AND ordinal=0`).Scan(&unknown).Error; err != nil {
		t.Fatal(err)
	}
	if unknown != 123 {
		t.Fatal("unresolved field unavailable in SQL")
	}
	if db.Migrator().HasTable("asset_payload_chunks") {
		t.Fatal("audio bytes must remain outside SQL")
	}
}

func TestFailedImportPreservesBothExistingDatabases(t *testing.T) {
	root, data := fixture(t)
	assets := filepath.Join(root, "assets.db")
	gameplay := filepath.Join(root, "gameplay.db")
	if _, err := Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: assets, GameplayDB: gameplay}); err != nil {
		t.Fatal(err)
	}
	oldAssets, _ := os.ReadFile(assets)
	oldGameplay, _ := os.ReadFile(gameplay)
	if err := os.WriteFile(filepath.Join(data, "item_data.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: assets, GameplayDB: gameplay}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
	currentAssets, _ := os.ReadFile(assets)
	currentGameplay, _ := os.ReadFile(gameplay)
	if string(currentAssets) != string(oldAssets) || string(currentGameplay) != string(oldGameplay) {
		t.Fatal("failed import changed existing databases")
	}
	leftovers, _ := filepath.Glob(filepath.Join(root, ".database-rebuild-*"))
	if len(leftovers) > 0 {
		t.Fatal("staging files leaked")
	}
}

func TestGameplayResetAndBackup(t *testing.T) {
	root, data := fixture(t)
	assets := filepath.Join(root, "assets.db")
	gameplay := filepath.Join(root, "gameplay.db")
	old, err := store.Open(gameplay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Register(context.Background(), "olduser", "secret", ""); err != nil {
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(gameplay)
	if err != nil {
		t.Fatal(err)
	}
	result, err := Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: assets, GameplayDB: gameplay})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Backups) != 1 {
		t.Fatalf("missing gameplay backup: %+v", result)
	}
	backup, err := os.ReadFile(result.Backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != string(original) {
		t.Fatal("backup bytes changed")
	}
	fresh, err := store.Open(gameplay)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	accounts, err := fresh.Accounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 0 {
		t.Fatal("old gameplay accounts survived reset")
	}
	if _, err = fresh.Register(context.Background(), "newuser", "secret", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRebuildRejectsUnsafeDestinations(t *testing.T) {
	root, data := fixture(t)
	arbitrary := filepath.Join(root, "notes.txt")
	os.WriteFile(arbitrary, []byte("preserve"), 0600)
	cases := []RebuildOptions{
		{DataDirectory: data, AssetsDB: filepath.Join(data, "item_data.json")},
		{DataDirectory: data, AssetsDB: arbitrary},
		{DataDirectory: data, AssetsDB: filepath.Join(root, "same.db"), GameplayDB: filepath.Join(root, "same.db")},
	}
	for _, options := range cases {
		if _, err := Rebuild(options); err == nil {
			t.Fatalf("unsafe destination accepted: %+v", options)
		}
	}
	original, _ := os.ReadFile(arbitrary)
	if string(original) != "preserve" {
		t.Fatal("unrelated file changed")
	}
	target := filepath.Join(root, "assets.db")
	os.WriteFile(target+"-wal", []byte("active"), 0600)
	if _, err := Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: target}); err == nil || !strings.Contains(err.Error(), "sidecar") {
		t.Fatalf("active sidecar not rejected: %v", err)
	}
}

func TestAssetsOnlyPreservesGameplay(t *testing.T) {
	root, data := fixture(t)
	gameplay := filepath.Join(root, "gameplay.db")
	old, err := store.Open(gameplay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Register(context.Background(), "olduser", "secret", ""); err != nil {
		t.Fatal(err)
	}
	old.Close()
	original, _ := os.ReadFile(gameplay)
	_, err = Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: filepath.Join(root, "assets.db")})
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(gameplay)
	if string(current) != string(original) {
		t.Fatal("asset rebuild changed gameplay")
	}
	old, err = store.Open(gameplay)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	if _, err = old.Authenticate(context.Background(), "olduser", "secret"); err != nil {
		t.Fatal(err)
	}
	// Ensure a database passed in the wrong role cannot be wiped.
	_, err = Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: gameplay})
	if err == nil {
		t.Fatal("gameplay database accepted as assets destination")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestSecondPublicationFailureRestoresFirstDatabase(t *testing.T) {
	root, data := fixture(t)
	assets := filepath.Join(root, "assets.db")
	gameplay := filepath.Join(root, "gameplay.db")
	if _, err := Rebuild(RebuildOptions{DataDirectory: data, AssetsDB: assets, GameplayDB: gameplay}); err != nil {
		t.Fatal(err)
	}
	oldAssets, _ := os.ReadFile(assets)
	oldGameplay, _ := os.ReadFile(gameplay)
	options := RebuildOptions{DataDirectory: data, AssetsDB: assets, GameplayDB: gameplay, Progress: func(string) {
		if err := os.WriteFile(gameplay+"-journal", []byte("client started during import"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	if _, err := Rebuild(options); err == nil || !strings.Contains(err.Error(), "became active") {
		t.Fatalf("expected publication failure, got %v", err)
	}
	currentAssets, _ := os.ReadFile(assets)
	currentGameplay, _ := os.ReadFile(gameplay)
	if string(currentAssets) != string(oldAssets) || string(currentGameplay) != string(oldGameplay) {
		t.Fatal("partial publication did not restore the previous databases")
	}
}

func TestGachaSQLViewsPreservePackAndRewardOrder(t *testing.T) {
	root, data := fixture(t)
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	document := []byte(`{"value":[{"item_id":64000,"name":"Custom Pack","rewards":[{"item_id":42,"weight":6000,"quantity":1},{"item_id":42,"weight":4000,"quantity":2}]}]}`)
	if _, err := importRecords(db, "gacha_packs.json", document); err != nil {
		t.Fatal(err)
	}
	var packs []struct {
		ItemID, PackOrder int
		Name              string
	}
	if err := db.Table("asset_gacha_packs").Order("pack_order").Find(&packs).Error; err != nil {
		t.Fatal(err)
	}
	if len(packs) != 1 || packs[0].ItemID != 64000 || packs[0].PackOrder != 0 || packs[0].Name != "Custom Pack" {
		t.Fatal(packs)
	}
	var rewards []struct{ PackID, PackOrder, RewardOrder, ItemID, Quantity, Weight int }
	if err := db.Table("asset_gacha_rewards").Order("pack_order, reward_order").Find(&rewards).Error; err != nil {
		t.Fatal(err)
	}
	if len(rewards) != 2 || rewards[0].PackID != 64000 || rewards[0].ItemID != 42 || rewards[0].Weight != 6000 || rewards[1].RewardOrder != 1 || rewards[1].Quantity != 2 || rewards[1].Weight != 4000 {
		t.Fatal(rewards)
	}
}

func TestLuckyDrawSQLViewRetainsWeightedOutcomes(t *testing.T) {
	root, data := fixture(t)
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer Close(db)
	payload := []byte(`{"value":[{"item_id":42,"name":"Reward","quantity":1,"weight":2,"slot":5},{"item_id":42,"name":"Reward","quantity":3,"weight":7,"slot":8}]}`)
	if _, err := importRecords(db, "lucky_draw.json", payload); err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		RewardOrder, ItemID, Quantity, Weight, Slot int
		Name                                        string
	}
	if err := db.Table("asset_lucky_draw_rewards").Order("reward_order").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].RewardOrder != 0 || rows[0].ItemID != 42 || rows[0].Quantity != 1 || rows[0].Weight != 2 || rows[0].Slot != 5 || rows[1].RewardOrder != 1 || rows[1].Quantity != 3 || rows[1].Weight != 7 || rows[1].Slot != 8 || rows[1].Name != "Reward" {
		t.Fatal(rows)
	}
}

func TestImportRejectsOriginalDatabaseSnapshot(t *testing.T) {
	root, data := fixture(t)
	manifestPath := filepath.Join(data, "asset_manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Assets = append(manifest.Assets, Entry{Asset: "serverdatabase.db", Output: "server/database_data.json"})
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err == nil || !strings.Contains(err.Error(), "snapshots are excluded") {
		t.Fatalf("expected snapshot exclusion, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("excluded snapshot created a database: %v", err)
	}
}

func TestVerifiedJSONRejectsRenamedDatabaseSnapshot(t *testing.T) {
	_, data := fixture(t)
	raw := []byte(`{"source_sha256":"` + strings.Repeat("a", 64) + `","format":"plaintext-sqlite-snapshot","tables":[]}`)
	if err := os.WriteFile(filepath.Join(data, "renamed.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	entry := Entry{Asset: "renamed.txt", Output: "renamed.json", SourceSHA256: strings.Repeat("a", 64), OutputBytes: int64(len(raw)), OutputSHA256: hex.EncodeToString(digest[:])}
	if _, err := verifiedJSON(data, entry); err == nil || !strings.Contains(err.Error(), "snapshots are excluded") {
		t.Fatalf("expected renamed snapshot exclusion, got %v", err)
	}
}

func TestImportRejectsOriginalDatabaseOverride(t *testing.T) {
	root, data := fixture(t)
	manifestPath := filepath.Join(data, "asset_manifest.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Assets = append(manifest.Assets, Entry{Asset: "database.override.txt", Output: "server/database.override.json"})
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "assets.db")
	if _, err := Build(data, path, nil); err == nil || !strings.Contains(err.Error(), "overrides are excluded") {
		t.Fatalf("expected override exclusion, got %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("excluded override created a database: %v", err)
	}
}
