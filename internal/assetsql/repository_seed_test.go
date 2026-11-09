package assetsql

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/game"
)

func TestRepositorySeedStartsWithoutClientAssetsAndPreservesEdits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-client-installation", "assets.db")
	created, err := InitializeRepositoryDatabase(path)
	if err != nil || !created {
		t.Fatal(created, err)
	}
	c, err := LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := repositoryMetadata()
	if err != nil {
		t.Fatal(err)
	}
	c.AssetsDatabase = ""
	for _, id := range []uint16{13050, 13051, 13052, 13053, 13054, 13055, 13056, 13057, 13059} {
		area := c.Maps[id]
		if !area.TrainingArea || area.BattleRoundRecoveryHP != 100 || area.BattleRoundRecoverySP != 50 || area.BattleVictoryEXP != 200 {
			t.Fatalf("training map %d lacks repository-owned battle rules", id)
		}
	}
	if entry := c.Maps[13050]; entry.TrainingEntryX != 2632 || entry.TrainingEntryY != 1344 {
		t.Fatal("missing native training entry destination")
	}
	if potion := c.Items[34173]; potion.TimedKind != game.TimedItemPotion || potion.TimedMinutes != 120 || potion.EXPTenths != 100 {
		t.Fatal("missing repository EXP potion definition", potion)
	}
	if pass := c.Items[34181]; pass.TimedKind != game.TimedItemTraining || pass.TimedMinutes != 360 {
		t.Fatal("training voucher must match its six-hour description", pass)
	}
	if ticket := c.Items[34253]; ticket.TimedKind != game.TimedItemTraining || ticket.TimedMinutes != 360 {
		t.Fatal("training ticket must grant six hours", ticket)
	}
	if c.Maps[12000].BattleVictoryEXP != 0 || c.Maps[12000].BattleRoundRecoveryHP != 0 {
		t.Fatal("ordinary map has training bonuses")
	}
	for _, grant := range c.StarterItems {
		if grant.ID == game.NotepadItemID {
			t.Fatal("Notepad must be obtained from the Astrology lady, not the starter pack")
		}
	}
	// Independent native EVE reward bytes: the Astrology lady still grants
	// one Notepad (34038) during her first conversation.
	notepadReward := [21]byte{1, 1, 0, 1, 0, 0xf6, 0x84, 0, 1, 0, 0, 0, 1}
	hasNotepadReward := false
	for _, event := range c.Maps[12003].Events {
		if event.ClickID != 4 {
			continue
		}
		for _, branch := range event.Branches {
			for _, operation := range branch.Operations {
				hasNotepadReward = hasNotepadReward || operation.Data == notepadReward
			}
		}
	}
	if !hasNotepadReward {
		t.Fatal("Astrology lady's native Notepad reward is missing")
	}
	if !reflect.DeepEqual(c.Summary(), m.Definitions) {
		t.Fatal("bundled definition census changed", c.Summary(), m.Definitions)
	}
	counts, err := defaultSkillContacts()
	if err != nil || len(counts) != 1832 {
		t.Fatal("bundled contacts", len(counts), err)
	}
	for id, skill := range c.Skills {
		if skill.ContactEvents != counts[id] {
			t.Fatalf("skill %d contacts differ from tracked table", id)
		}
	}
	grades, err := defaultSkillGrades()
	if err != nil || len(grades) != 953 {
		t.Fatal("bundled grade definitions", len(grades), err)
	}
	for id, skill := range c.Skills {
		if skill.MaximumGrade == nil || skill.GradeLimit() != grades[id] {
			t.Fatalf("skill %d grade limit differs from tracked table", id)
		}
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var imports int64
	if err := db.Model(&assetdb.Record{}).Count(&imports).Error; err != nil {
		t.Fatal(err)
	}
	if imports != 0 || db.Migrator().HasTable("accounts") || db.Migrator().HasTable("characters") {
		t.Fatal("seed contains original archives or gameplay state")
	}
	if err := db.Model(&SkillsRow{}).Where("skills_key = ?", 11016).Updates(map[string]any{"value_name": "Administrator edit", "value_contact_events": 5}).Error; err != nil {
		t.Fatal(err)
	}
	if err := assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	created, err = InitializeRepositoryDatabase(path)
	if err != nil || created {
		t.Fatal("existing catalog replaced", created, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("initialization changed existing database bytes", err)
	}
	if _, err := BuildRepositoryDatabase(path); !os.IsExist(err) {
		t.Fatal("direct seed build overwrote catalog", err)
	}
}

func TestRepositorySeedDoesNotReplaceInvalidExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	original := []byte("keep this file even though it is not a valid SQLite catalog")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	created, err := InitializeRepositoryDatabase(path)
	if err != nil || created {
		t.Fatal(created, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("invalid existing database overwritten", err)
	}
}

func TestRepositorySeedRebuildUsesNoExportsAndKeepsBackup(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "assets.db")
	if _, err := InitializeRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&SkillsRow{}).Where("skills_key = ?", 11016).Update("value_name", "Keep in backup").Error; err != nil {
		t.Fatal(err)
	}
	if err := assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	gameplay := filepath.Join(root, "gameplay-untouched.db")
	sentinel := []byte("gameplay storage is unrelated to an assets-only rebuild")
	if err := os.WriteFile(gameplay, sentinel, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := assetdb.Rebuild(assetdb.RebuildOptions{AssetsDB: path, BuildAssets: BuildRepositoryDatabase, PrepareAssets: MigrateDatabase})
	if err != nil || len(result.Backups) != 1 {
		t.Fatal(result, err)
	}
	backup, err := LoadDatabase(result.Backups[0])
	if err != nil || backup.Skills[11016].Name != "Keep in backup" {
		t.Fatal("old definitions were not retained", err)
	}
	current, err := LoadDatabase(path)
	if err != nil || current.Skills[11016].Name == "Keep in backup" || current.Skills[11016].ContactEvents != 2 {
		t.Fatal("repository rebuild did not restore seed", err)
	}
	data, err := os.ReadFile(gameplay)
	if err != nil || !bytes.Equal(data, sentinel) {
		t.Fatal("assets rebuild changed gameplay state", err)
	}
}
