package legacyimport

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/game"
	"wonderland-go/internal/store"
)

func legacyFixture(t *testing.T) (string, *assets.Catalog) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := `CREATE TABLE users(userID INTEGER PRIMARY KEY,username TEXT,password TEXT,email TEXT,IM INTEGER);
 INSERT INTO users VALUES(7,'LegacyUser','password','legacy@example.test',123);
 CREATE TABLE characters(charID INTEGER PRIMARY KEY,slot INTEGER,name TEXT,body INTEGER,head INTEGER,element INTEGER,location_map INTEGER,location_x INTEGER,location_y INTEGER,gold INTEGER,cipher TEXT,haircolor INTEGER,skincolor INTEGER,clothingcolor INTEGER,eyecolor INTEGER);
 INSERT INTO characters VALUES(10007,1,'LegacyHero',4,3,3,10017,1042,1075,321,'123456',1,2,3,4);
 INSERT INTO characters VALUES(4510007,2,'OtherHero',4,3,2,10017,1042,1075,10,'123456',1,2,3,4);
 CREATE TABLE stats(charID INTEGER,statID INTEGER,StatusUp INTEGER,potential INTEGER);
 INSERT INTO stats VALUES(10007,25,20,0),(10007,26,10,0),(10007,27,3,0),(10007,28,4,0),(10007,29,5,0),(10007,30,6,0),(10007,33,7,0),(10007,36,6,0),(10007,38,9,0);
 CREATE TABLE inventory(charID INTEGER,storID INTEGER,pos INTEGER,itemID INTEGER,qty INTEGER,dmg INTEGER,forge INTEGER);
 INSERT INTO inventory VALUES(10007,0,2,32176,3,2,7),(10007,1,3,10001,1,3,2),(10007,2,4,32176,5,4,9);
 CREATE TABLE character_skills(charID INTEGER,skillID INTEGER,grade INTEGER,exp INTEGER);
 INSERT INTO character_skills VALUES(10007,11001,3,15);
 CREATE TABLE charquest(charID INTEGER,quest_started INTEGER,quest_pos INTEGER,step INTEGER,kill_count INTEGER);
 INSERT INTO charquest VALUES(10007,13086,1,2,4),(10007,900,3,1,0);
 CREATE TABLE character_pets(charID INTEGER,slot INTEGER,petID INTEGER,petName TEXT,level INTEGER,exp INTEGER,hp INTEGER,sp INTEGER,str INTEGER,con INTEGER,int_ INTEGER,wis INTEGER,agi INTEGER,amity INTEGER,skillPoints INTEGER,isBattle INTEGER,isRide INTEGER,isHotel INTEGER,reborn INTEGER,job INTEGER,skills TEXT,eq_weapon INTEGER,equipment_meta TEXT);
 INSERT INTO character_pets VALUES(10007,1,14156,'Xaolan',12,44,10,5,10,12,14,16,18,77,3,1,0,0,0,0,'11001:3:25',10001,'AAAAAAUHAAAAAAAA');
 INSERT INTO character_pets VALUES(10007,2,14081,'Niss',12,33,10,5,10,12,14,16,18,78,4,0,0,2,0,0,'11001:2:30',0,'');
 CREATE TABLE Friends(charID1 INTEGER,charID2 INTEGER);INSERT INTO Friends VALUES(10007,4510007);
 CREATE TABLE charactersextdata(charID INTEGER,Settings TEXT,Friends TEXT,Guild TEXT,Mail TEXT);INSERT INTO charactersextdata VALUES(10007,'False True 17 False','','0','');
 CREATE TABLE Archive(value INTEGER);INSERT INTO Archive VALUES(1);
 CREATE TABLE character_record_points(charID INTEGER,mapID INTEGER,posX INTEGER,posY INTEGER);INSERT INTO character_record_points VALUES(10007,10017,20,40);
 CREATE TABLE character_monster_book(charID INTEGER,npcID INTEGER);INSERT INTO character_monster_book VALUES(10007,14156);`
	if err = db.Exec(sql).Error; err != nil {
		t.Fatal(err)
	}
	if err = assetdb.Close(db); err != nil {
		t.Fatal(err)
	}
	return path, &assets.Catalog{Maps: map[uint16]assets.Map{10017: {ID: 10017}}, Items: map[uint16]game.ItemDefinition{32176: {ID: 32176, Type: 24}, 10001: {ID: 10001}}, Skills: map[uint16]assets.Skill{11001: {ID: 11001}}, NPCs: map[uint16]assets.NPC{14156: {ID: 14156, Type: 4, Skills: [3]uint16{11001}}, 14081: {ID: 14081, Type: 4, Skills: [3]uint16{11001}}}}
}
func TestLegacyReadOnlyTypedImportRoundTrip(t *testing.T) {
	source, catalog := legacyFixture(t)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	plan, err := Read(ctx, source, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Report.Accounts != 1 || plan.Report.Characters != 2 || plan.Report.Items != 3 || plan.Report.Pets != 2 || plan.Report.Quests != 2 || plan.Report.Skills != 1 || plan.Report.Unmapped["Archive"] != 1 || plan.Report.Friends != 1 {
		t.Fatal(plan.Report)
	}
	c := plan.Accounts[0].Characters[0]
	if c.Bag[1].Count != 3 || c.Bag[1].Forge() != 7 || c.Equipment[2].Damage != 3 || c.Storage[3].Count != 5 || c.Color1 != 0x20001 || c.Color2 != 0x40003 || c.Pets[0].Equipment[2].Damage != 5 || c.Pets[0].Equipment[2].Forge() != 7 || c.Pets[0].Skills[0].Grade != 3 || c.ReservePets[0].Exp != 33 || c.ActivePet != 14156 || c.Settings == nil || c.Settings.Channels != 17 || c.RecordPoint == nil || c.RecordPoint.X != 20 || len(c.DiscoveredMonsters) != 1 {
		t.Fatal("mapping drift", c)
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "wonderland.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.ImportLegacy(ctx, plan.Accounts, plan.Friendships...); err != nil {
		t.Fatal(err)
	}
	a, err := db.Authenticate(ctx, "legacyuser", "password")
	if err != nil || a.ID != 7 || a.IM != 123 {
		t.Fatal(a, err)
	}
	actual, err := db.Characters(ctx, a.ID)
	if err != nil || !reflect.DeepEqual(actual, plan.Accounts[0].Characters) {
		t.Fatalf("typed roundtrip: %v\nwant %+v\ngot %+v", err, plan.Accounts[0].Characters, actual)
	}
	if err = db.ImportLegacy(ctx, plan.Accounts, plan.Friendships...); err == nil {
		t.Fatal("import overwrote existing accounts")
	}
	after, err := os.ReadFile(source)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("source changed", err)
	}
}
func TestLegacyRejectsUnrepresentableAndOrphanedState(t *testing.T) {
	for _, tt := range []struct{ name, sql string }{{"orphan", "UPDATE stats SET charID=999 WHERE statID=25"}, {"negative", "UPDATE inventory SET dmg=-1"}, {"missing item", "UPDATE inventory SET itemID=65535"}, {"duplicate slot", "INSERT INTO inventory VALUES(10007,0,2,32176,1,0,0)"}, {"job", "ALTER TABLE characters ADD job INTEGER DEFAULT 1"}, {"potential", "UPDATE stats SET potential=1 WHERE statID=28"}, {"forum password", "ALTER TABLE users ADD members_pass_salt TEXT DEFAULT 'salt'"}, {"vitals", "UPDATE stats SET StatusUp=9999999 WHERE statID=25"}} {
		t.Run(tt.name, func(t *testing.T) {
			source, catalog := legacyFixture(t)
			db, err := assetdb.Open(source)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Exec(tt.sql).Error; err != nil {
				t.Fatal(err)
			}
			assetdb.Close(db)
			if _, err = Read(context.Background(), source, catalog); err == nil {
				t.Fatal("incompatible source accepted")
			}
		})
	}
}
