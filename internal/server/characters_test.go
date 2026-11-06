package server

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/battle"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
	"wonderland-gonline/internal/store"
)

type captureConn struct{ bytes.Buffer }

func (c *captureConn) Read([]byte) (int, error)         { return 0, io.EOF }
func (c *captureConn) Close() error                     { return nil }
func (c *captureConn) LocalAddr() net.Addr              { return &net.TCPAddr{} }
func (c *captureConn) RemoteAddr() net.Addr             { return &net.TCPAddr{} }
func (c *captureConn) SetDeadline(time.Time) error      { return nil }
func (c *captureConn) SetReadDeadline(time.Time) error  { return nil }
func (c *captureConn) SetWriteDeadline(time.Time) error { return nil }
func (c *captureConn) packets(t *testing.T) [][]byte {
	t.Helper()
	var out [][]byte
	for c.Len() > 0 {
		p, e := protocol.Read(&c.Buffer)
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, p)
	}
	return out
}

func creationCatalog() *assets.Catalog {
	return &assets.Catalog{Story: assets.DefaultStoryDefinitions(), Maps: map[uint16]assets.Map{10017: {ID: 10017}}, Items: map[uint16]game.ItemDefinition{21001: {ID: 21001, EquipSlot: 2}, 24001: {ID: 24001, EquipSlot: 5}, 32176: {ID: 32176, Type: 23}}, StarterItems: []game.StarterGrant{{ID: 32176, Count: 50}}, Skills: map[uint16]assets.Skill{15003: {TableOrder: 188}, 11016: {TableOrder: 1}, 11166: {TableOrder: 2}, 11056: {TableOrder: 3}}}
}
func loginPayload(name string) []byte {
	p, _ := protocol.Builder{63, 4}.String(name)
	p, _ = p.String("password")
	return p
}
func createPayload() []byte {
	return protocol.Builder{9, 1}.U16(1).U16(0).U16(1).U16(2).U16(3).U16(4).U8(3).Bytes([]byte{1, 1, 1, 1, 1})
}

func TestCreateEnterAcknowledgeAndReconnect(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "db")
	db, e := store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { db.Close() }()
	account, e := db.Register(ctx, "tester", "password", "")
	if e != nil {
		t.Fatal(e)
	}
	s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	wire := &captureConn{}
	c := &Session{conn: wire, info: SessionInfo{ID: 1}}
	dispatch := func(p []byte) {
		t.Helper()
		if e := s.dispatch(ctx, c, p); e != nil {
			t.Fatal(e)
		}
	}
	dispatch(loginPayload("tester"))
	wire.Reset()
	dispatch([]byte{63, 2, 1})
	wire.Reset()
	dispatch(append([]byte{9, 2}, []byte("Player")...))
	if p := wire.packets(t); len(p) != 1 || !bytes.Equal(p[0], []byte{9, 3, 0}) {
		t.Fatal(p)
	}
	dispatch(createPayload())
	packets := wire.packets(t)
	warp, base, scene := -1, -1, -1
	for i, p := range packets {
		if p[0] == 12 {
			warp = i
		}
		if bytes.Equal(p, []byte{23, 102}) {
			scene = i
		}
		if p[0] == 5 && len(p) > 1 && p[1] == 3 {
			base = i
		}
	}
	if base < 0 || warp <= base || scene <= warp || c.ready || c.character == nil {
		t.Fatal("wrong world entry ordering/state")
	}
	if e := s.dispatch(ctx, c, []byte{6, 1, 0, 1, 0, 1, 0}); e == nil {
		t.Fatal("movement accepted before map acknowledgment")
	}
	dispatch([]byte{12, 1})
	if p := wire.packets(t); len(p) != 2 || !bytes.Equal(p[0], []byte{5, 4}) || !bytes.Equal(p[1], []byte{56, 30, 0}) {
		t.Fatal(p)
	}
	dispatch([]byte{12, 1})
	if wire.Len() != 0 {
		t.Fatal("duplicate map acknowledgment replayed")
	}
	dispatch(protocol.Builder{6, 1, 3}.U16(1100).U16(1200))
	wire.Reset()
	if e := s.dispatch(ctx, c, createPayload()); e == nil {
		t.Fatal("created character twice")
	}
	// Walking is buffered; a normal disconnect flushes it before reconnect.
	s.leaveWorld(c)
	chars, e := db.Characters(ctx, account.ID)
	if e != nil || len(chars) != 1 || chars[0].X != 1100 || chars[0].Y != 1200 || chars[0].Bag[0].Count != 50 {
		t.Fatal(chars, e)
	}
	db.Close()
	db, e = store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	s = New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	c = &Session{conn: wire, info: SessionInfo{ID: 2}}
	dispatch(loginPayload("tester"))
	wire.Reset()
	dispatch([]byte{63, 2, 1})
	wire.Reset()
	if c.character == nil || c.character.X != 1100 || c.character.Bag[0].Count != 50 {
		t.Fatal("character did not survive restart")
	}
}

func TestNameReservationAndMalformedCreation(t *testing.T) {
	ctx := context.Background()
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	account, e := db.Register(ctx, "tester", "password", "")
	if e != nil {
		t.Fatal(e)
	}
	s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	a := &Session{conn: &captureConn{}, info: SessionInfo{ID: 1}, account: account, slot: 1}
	b := &Session{conn: &captureConn{}, info: SessionInfo{ID: 2}, account: account, slot: 2}
	if ok, e := s.reserveName(ctx, a, "Player"); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if ok, e := s.reserveName(ctx, b, "PLAYER"); e != nil || ok {
		t.Fatal("reservation was stolen", e)
	}
	if e := s.createCharacter(ctx, a, createPayload()[:10]); e != nil {
		t.Fatal(e)
	}
	chars, e := db.Characters(ctx, account.ID)
	if e != nil || len(chars) != 0 {
		t.Fatal("malformed packet wrote character", e)
	}
	s.releaseName(a)
	if ok, e := s.reserveName(ctx, b, "PLAYER"); e != nil || !ok {
		t.Fatal("reservation was not released", e)
	}
}

func TestDeletionWireFlow(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	// Recreate the fixture character with a secondary deletion code.
	char := *c.character
	ctx := context.Background()
	if err := s.Store.DeleteCharacter(ctx, c.account, 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.CreateCharacterWithCode(ctx, c.account, char, "secret-code"); err != nil {
		t.Fatal(err)
	}
	c.character = nil
	deletion := func(code string) []byte {
		p, _ := protocol.Builder{35, 2, 1}.String("")
		p, _ = p.String(code)
		return p
	}
	if err := s.dispatch(ctx, c, deletion("wrong-code")); err != nil {
		t.Fatal(err)
	}
	packets := wires[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{35, 2, 3, 1}) {
		t.Fatal(packets)
	}
	if err := s.dispatch(ctx, c, deletion("secret-code")[:5]); err == nil {
		t.Fatal("truncated code accepted")
	}
	chars, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars) != 1 {
		t.Fatal("failed deletion changed roster", err)
	}
	if err := s.dispatch(ctx, c, deletion("secret-code")); err != nil {
		t.Fatal(err)
	}
	packets = wires[0].packets(t)
	if len(packets) != 6 || !bytes.Equal(packets[5], []byte{35, 2, 1, 1}) {
		t.Fatal(packets)
	}
	chars, err = s.Store.Characters(ctx, c.account.ID)
	if err != nil || len(chars) != 0 {
		t.Fatal("successful deletion retained character", err)
	}
	if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	packets = wires[0].packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{1, 3, 0}) {
		t.Fatal("last deletion retained secondary code", packets)
	}
}

func TestNativeCreationPersistsSeparateDeletionPassword(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "native", "login123", "")
	if err != nil {
		t.Fatal(err)
	}
	wire := &captureConn{}
	var logs bytes.Buffer
	s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(&logs, nil)))
	c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account, slot: 1}
	if ok, err := s.reserveName(ctx, c, "NativeHero"); err != nil || !ok {
		t.Fatal("name reservation failed", err)
	}
	// Native request, including account confirmation and distinct deletion code.
	payload := []byte{9, 1, 1, 0, 0, 0, 0x1c, 0xaf, 0x7d, 0x1a, 0x1c, 0xaf, 0x7d, 0x1a, 3, 1, 1, 1, 1, 1, 8, 'l', 'o', 'g', 'i', 'n', '1', '2', '3', 9, 'd', 'e', 'l', 'e', 't', 'e', '4', '5', '6'}
	if err = s.dispatch(ctx, c, payload); err != nil {
		t.Fatal(err)
	}
	chars, err := db.Characters(ctx, account.ID)
	if err != nil || len(chars) != 1 {
		t.Fatal("native character not persisted", err)
	}
	if chars[0].Color1 != 444444444 || chars[0].Color2 != 444444444 || c.character == nil {
		t.Fatal("native appearance or world entry missing")
	}
	if !bytes.Contains(logs.Bytes(), []byte("character created")) {
		t.Fatal("missing commit outcome log")
	}
	if bytes.Contains(logs.Bytes(), []byte("login123")) || bytes.Contains(logs.Bytes(), []byte("delete456")) {
		t.Fatal("creation credentials leaked into logs")
	}
	if _, err = db.Authenticate(ctx, "native", "login123"); err != nil {
		t.Fatal("account password changed", err)
	}
	if err = db.DeleteCharacter(ctx, account, 1, "login123"); !errors.Is(err, store.ErrCredentials) {
		t.Fatal("account password treated as deletion password", err)
	}
	if err = db.DeleteCharacter(ctx, account, 1, "delete456"); err != nil {
		t.Fatal("native deletion password not stored", err)
	}
}

func TestNativeCreationWrongConfirmationDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "native", "login123", "")
	if err != nil {
		t.Fatal(err)
	}
	wire := &captureConn{}
	var logs bytes.Buffer
	s := New(config.Default(), db, creationCatalog(), slog.New(slog.NewTextHandler(&logs, nil)))
	c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account, slot: 1}
	if ok, err := s.reserveName(ctx, c, "NativeHero"); err != nil || !ok {
		t.Fatal(err)
	}
	payload := []byte{9, 1, 1, 0, 0, 0, 0x1c, 0xaf, 0x7d, 0x1a, 0x1c, 0xaf, 0x7d, 0x1a, 3, 1, 1, 1, 1, 1, 8, 'w', 'r', 'o', 'n', 'g', '1', '2', '3', 9, 'd', 'e', 'l', 'e', 't', 'e', '4', '5', '6'}
	if err = s.dispatch(ctx, c, payload); err != nil {
		t.Fatal(err)
	}
	packets := wire.packets(t)
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{0, 30}) {
		t.Fatal("expected DC30 rejection", packets)
	}
	chars, err := db.Characters(ctx, account.ID)
	if err != nil || len(chars) != 0 || c.character != nil {
		t.Fatal("rejected request persisted a character", err)
	}
	if !bytes.Contains(logs.Bytes(), []byte("stage=confirm_password")) {
		t.Fatal("rejection stage not logged")
	}
	if bytes.Contains(logs.Bytes(), []byte("wrong123")) || bytes.Contains(logs.Bytes(), []byte("delete456")) {
		t.Fatal("rejected credentials leaked")
	}
}

func TestNativeCharacterCreationWithInstalledAssets(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range []byte{1, 2, 3, 4} {
		t.Run(map[byte]string{1: "earth", 2: "water", 3: "fire", 4: "wind"}[element], func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			account, err := db.Register(ctx, "native", "login123", "")
			if err != nil {
				t.Fatal(err)
			}
			wire := &captureConn{}
			s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
			c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account, slot: 1}
			if ok, err := s.reserveName(ctx, c, "NativeHero"); err != nil || !ok {
				t.Fatal(err)
			}
			payload := []byte{9, 1, 4, 0, 0, 0, 0x1c, 0xaf, 0x7d, 0x1a, 0x1c, 0xaf, 0x7d, 0x1a, 3, 1, 1, 1, 1, 1, 8, 'l', 'o', 'g', 'i', 'n', '1', '2', '3', 9, 'd', 'e', 'l', 'e', 't', 'e', '4', '5', '6'}
			payload[14] = element
			if err = s.dispatch(ctx, c, payload); err != nil {
				t.Fatal(err)
			}
			chars, err := db.Characters(ctx, account.ID)
			if err != nil || len(chars) != 1 || c.character == nil {
				t.Fatal("native assets did not complete creation", err)
			}
			ready := false
			for _, packet := range wire.packets(t) {
				if bytes.Equal(packet, []byte{0, 30}) {
					t.Fatal("native creation rejected")
				}
				if bytes.Equal(packet, []byte{1, 11}) {
					ready = true
				}
			}
			if !ready {
				t.Fatal("native world entry did not reach ready marker")
			}
		})
	}
}

func TestNativeCharacterReconnectWithInstalledAssets(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "native", "login123", "")
	if err != nil {
		t.Fatal(err)
	}
	char, err := game.NewCharacter(account.CharacterID(1), 1, "NativeHero", game.Appearance{Body: 4, Head: 7, Element: 3, Base: game.Attributes{Strength: 5, Agility: 5, Wisdom: 5, Intelligence: 5, Constitution: 5}}, catalog.StarterItems, catalog.Items, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.CreateCharacterWithCode(ctx, account, char, "delete456"); err != nil {
		t.Fatal(err)
	}
	wire := &captureConn{}
	s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account}
	if err = s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	if c.character == nil || c.ready {
		t.Fatal("expected selected character awaiting map acknowledgment")
	}
	wire.packets(t)
	// Observed native login sequence: AC23:77 arrives before AC12:1.
	if err = s.dispatch(ctx, c, []byte{23, 77}); err != nil {
		t.Fatal(err)
	}
	if got := wire.packets(t); len(got) != 2 || !bytes.Equal(got[0], []byte{23, 4, 0}) || !bytes.Equal(got[1], []byte{23, 102}) {
		t.Fatalf("native list response %x", got)
	}
	if c.ready {
		t.Fatal("list request prematurely published the character")
	}
	// The native refresh follows the list request before map acknowledgment.
	if err = s.dispatch(ctx, c, []byte{5, 7, 0}); err != nil {
		t.Fatal(err)
	}
	replies := wire.packets(t)
	if len(replies) != 2 || !bytes.Equal(replies[0], []byte{5, 8, 0x11, 0x27, 0, 0, 0}) {
		t.Fatalf("native sprite refresh %x", replies)
	}
	// The same loading-time refresh synchronizes the native draw UI without
	// consuming allowance or publishing the character before map acknowledgment.
	if draw := replies[1]; len(draw) < 5 || !bytes.Equal(draw[:4], []byte{104, 1, 1, 0}) || int(draw[4]) != len(catalog.LuckyDraw.Rewards) || len(draw) != 5+3*int(draw[4]) {
		t.Fatalf("native Lucky Draw refresh %x", draw)
	}
	if c.ready {
		t.Fatal("sprite refresh prematurely published the character")
	}
	for _, request := range [][]byte{{23, 54}, {23, 25}, {75, 2}} {
		if err = s.dispatch(ctx, c, request); err != nil {
			t.Fatalf("loading request %x: %v", request, err)
		}
		replies := wire.packets(t)
		if len(replies) == 0 {
			t.Fatalf("loading request %x had no response", request)
		}
		for _, reply := range replies {
			if len(reply) < 2 || reply[0] != 75 {
				t.Fatalf("unexpected mall sync %x", reply)
			}
		}
		if c.ready {
			t.Fatal("mall synchronization prematurely published the character")
		}
	}
	if err = s.dispatch(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	if !c.ready {
		t.Fatal("native reconnect did not finish map loading")
	}
}

// installedCatalog runs native gameplay checks against the runtime SQL source
// when provided, with the original file loader retained for reference checks.
func installedCatalog(t *testing.T) (*assets.Catalog, error) {
	t.Helper()
	if path := os.Getenv("WONDERLAND_TEST_ASSETS_DB"); path != "" {
		if !filepath.IsAbs(path) {
			path = filepath.Join("..", "..", path)
		}
		return assetsql.LoadDatabase(path)
	}
	dir := os.Getenv("WONDERLAND_TEST_DATA")
	if dir == "" {
		t.Skip("set WONDERLAND_TEST_ASSETS_DB or WONDERLAND_TEST_DATA")
	}
	return assets.Load(dir, filepath.Join("..", "..", "data", "item_data.json"))
}

func TestStarterPackCreationDoesNotRefillOnLogin(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(ctx, "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	catalog := creationCatalog()
	s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	wire := &captureConn{}
	c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account, slot: 1}
	for _, packet := range [][]byte{append([]byte{9, 2}, []byte("Player")...), createPayload()} {
		if err := s.dispatch(ctx, c, packet); err != nil {
			t.Fatal(err)
		}
	}
	chars, err := db.Characters(ctx, account.ID)
	if err != nil || len(chars) != 1 || bagCount(chars[0].Bag, 32176) != 50 || bagCount(c.character.Bag, 32176) != 50 {
		t.Fatal("starter pack not persisted at creation", chars, err)
	}
	s.leaveWorld(c)
	if err := db.UpdateCharacter(ctx, account.ID, account.CharacterID(1), func(char *game.Character) error {
		char.Bag = game.Inventory{}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Editing the pack must not grant new items to an existing character either.
	catalog.Items[32177] = game.ItemDefinition{ID: 32177, Type: 23}
	catalog.StarterItems = append(catalog.StarterItems, game.StarterGrant{ID: 32177, Count: 1})
	for i := uint64(2); i <= 3; i++ {
		wire.Reset()
		c = &Session{conn: wire, info: SessionInfo{ID: i}, account: account}
		if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
			t.Fatal(err)
		}
		if c.character == nil || c.character.Level != 1 || c.character.Bag != (game.Inventory{}) {
			t.Fatal("login refilled starter pack", c.character)
		}
		if !contains(wire.packets(t), c.character.Bag.Packet(protocol.CommandInventory, protocol.InventoryItems)) {
			t.Fatal("login inventory snapshot missing")
		}
		s.leaveWorld(c)
	}
	chars, err = db.Characters(ctx, account.ID)
	if err != nil || len(chars) != 1 || chars[0].Bag != (game.Inventory{}) {
		t.Fatal("login persisted another starter pack", chars, err)
	}
}

func TestCharacterCreationPersistsQualifiedStarterSkills(t *testing.T) {
	for _, tc := range []struct {
		name    string
		element byte
		ids     []uint16
		gated   uint16
	}{
		{"earth", 1, []uint16{15085, 12006, 11057}, 11017},
		{"water", 2, []uint16{15091, 15097, 15100}, 11001},
		{"fire", 3, []uint16{11016, 11166, 11056}, 15101},
		{"wind", 4, []uint16{11007, 30002, 11052}, 15079},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open(filepath.Join(t.TempDir(), "db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			account, err := db.Register(ctx, "tester", "password", "")
			if err != nil {
				t.Fatal(err)
			}
			catalog := creationCatalog()
			for i, id := range tc.ids {
				catalog.Skills[id] = assets.Skill{ID: id, TableOrder: uint16(101 + i)}
			}
			catalog.Skills[tc.gated] = assets.Skill{ID: tc.gated, TableOrder: 201}
			s := New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
			wire := &captureConn{}
			c := &Session{conn: wire, info: SessionInfo{ID: 1}, account: account, slot: 1}
			if ok, err := s.reserveName(ctx, c, "FreshHero"); err != nil || !ok {
				t.Fatal(err)
			}
			payload := createPayload()
			payload[14] = tc.element
			if err := s.dispatch(ctx, c, payload); err != nil {
				t.Fatal(err)
			}
			stored, err := db.Characters(ctx, account.ID)
			if err != nil || len(stored) != 1 || c.character == nil {
				t.Fatal("creation failed", stored, err)
			}
			want := append([]uint16{11075}, tc.ids...)
			for _, character := range []game.Character{stored[0], *c.character} {
				if len(character.Skills) != len(want) {
					t.Fatal("wrong skill count", character.Skills)
				}
				for i, id := range want {
					if character.Skills[i] != (game.LearnedSkill{ID: id, Grade: 1}) {
						t.Fatal("wrong persisted/session skill", character.Skills)
					}
				}
			}
			found := false
			for _, packet := range wire.packets(t) {
				if len(packet) < 2 || packet[0] != 5 || packet[1] != 3 {
					continue
				}
				found = true
				// Native AC5:3 puts skill count at byte 62 and seven-byte records
				// after it. Stunt uses wire order 188, then the three starter skills.
				if len(packet) < 92 {
					t.Fatal("truncated base stats", packet)
				}
				r := protocol.NewReader(packet[62:])
				if r.U16() != 4 {
					t.Fatal("wrong wire skill count")
				}
				for _, order := range []uint16{188, 101, 102, 103} {
					if r.U16() != order || r.U8() != 1 || r.U32() != 0 {
						t.Fatal("wrong wire skill record", packet)
					}
				}
			}
			if !found {
				t.Fatal("missing initial skill snapshot")
			}
		})
	}
}

func TestCompactCreationAuthenticatedOwnedSlotAndAllocation(t *testing.T) {
	s, c, wire, _ := mallFixture(t)
	ctx := context.Background()
	account := c.account
	c.character = nil
	c.slot = 0
	c.ready = false
	payload := []byte{63, 3, 2, 6, 'S', 'e', 'c', 'o', 'n', 'd', 1, 3, 1, 1, 1, 1, 1}
	invalid := append([]byte(nil), payload...)
	invalid[len(invalid)-1] = 99
	if err := s.dispatch(ctx, c, invalid); err != nil || c.character != nil {
		t.Fatal("invalid allocation", err)
	}
	if got := wire.packets(t); len(got) != 1 || !bytes.Equal(got[0], []byte{1, 3, 1}) {
		t.Fatal(got)
	}
	if err := s.dispatch(ctx, c, payload); err != nil || c.character == nil || c.character.Slot != 2 || c.character.Name != "Second" {
		t.Fatal(c.character, err)
	}
	if got := wire.packets(t); len(got) == 0 || !bytes.Equal(got[0], []byte{1, 3, 0}) {
		t.Fatal("compact creation ACK", got)
	}
	chars, err := s.Store.Characters(ctx, account.ID)
	if err != nil || len(chars) != 2 {
		t.Fatal("owned character not committed", err)
	}
	if err = s.dispatch(ctx, c, payload); err == nil {
		t.Fatal("live session recreated character")
	}
}

func TestSQLGoddessEffectsAndDurableProficiency(t *testing.T) {
	catalog, err := installedCatalog(t)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint16{15188, 15189} {
		name := "Shrink"
		if id == 15189 {
			name = "Hot Fire Attack"
		}
		t.Run(name, func(t *testing.T) {
			s, c, wire := battleFixture(t, 1000)
			defer s.abandonBattle(c)
			skill, ok := catalog.Skills[id]
			if !ok || skill.Name != name {
				t.Fatal("native Goddess skill missing", skill)
			}
			s.Assets.Skills = catalog.Skills
			c.character.Skills = append(c.character.Skills, game.LearnedSkill{ID: id, Grade: 1})
			run := c.battle
			actor := run.members[0].self
			actor.Char = c.character
			actor.SP = 120
			tx := byte(2)
			if id == 15189 {
				tx = 4
			}
			wire.Reset()
			if err := s.worldCommand(context.Background(), c, []byte{50, 1, 4, 2, tx, 2, byte(id), byte(id >> 8)}); err != nil {
				t.Fatal(err)
			}
			settle(t, s, c)
			s.worldMu.Lock()
			shrunk, hot := run.b.Defenders[0].HasEffect(15188), actor.HasEffect(15189)
			hp, sp := run.b.Defenders[0].HP, actor.SP
			s.worldMu.Unlock()
			if hp != 1000 || sp != 120-int(skill.SP) || id == 15188 && !shrunk || id == 15189 && !hot {
				t.Fatal("Goddess became damage or missed its target", hp, sp, shrunk, hot)
			}
			stored, err := s.Store.Characters(context.Background(), c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, sk := range stored[0].Skills {
				if sk.ID == id {
					// A successful status cast awards 15 support EXP plus 25 buff EXP.
					found = sk.EXP == 40
				}
			}
			if !found {
				t.Fatal("successful Goddess cast did not save proficiency")
			}
			want := []byte{50, 1, 17, 0, 4, 2, byte(id), byte(id >> 8), 0, 1, tx, 2, 1, 0, 1, 0, 0, 0, 0, 0, 1}
			if !contains(wire.packets(t), want) {
				t.Fatal("non-HP Goddess animation missing")
			}
		})
	}
}

func TestBattleSkillProgressIsDurable(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	run := c.battle
	packets, err := s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].self, Skill: 11001}})
	if err != nil || !contains(packets, protocol.Builder{8, 1, 110, 1}.U32(2).U32(11001)) {
		t.Fatal(packets, err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) == 0 || stored[0].Skills[0].Grade != 2 || stored[0].Skills[0].EXP != 9 {
		t.Fatal(stored, err)
	}
	packets, err = s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].self, Skill: 65535}})
	if err != nil || len(packets) != 0 {
		t.Fatal("unlearned action progressed", packets, err)
	}
}

func TestBattleSkillSaveFailurePreservesSession(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	c.character.Skills = []game.LearnedSkill{{ID: 11001, Grade: 1, EXP: 99}}
	s.Store.Close()
	packets, err := s.commitSkillUses(c, c.battle, []battle.Action{{Actor: c.battle.members[0].self, Skill: 11001}})
	if err == nil || len(packets) != 0 || c.character.Skills[0].Grade != 1 || c.character.Skills[0].EXP != 99 {
		t.Fatal(packets, err, c.character.Skills)
	}
}

func TestBattlePetSkillProgressWithoutClientSlot(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	pet := game.Pet{ID: 10501, Skills: []game.PetSkill{{ID: 11001, Grade: 9, Exp: 899}}}
	c.character.Pets = []game.Pet{pet}
	run := c.battle
	run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
	packets, err := s.commitSkillUses(c, run, []battle.Action{{Actor: run.members[0].pet, Skill: 11001}})
	if err != nil || len(packets) != 0 || c.character.Pets[0].Skills[0].Grade != 10 || run.members[0].pet.Pet.Skills[0].Grade != 10 {
		t.Fatal(packets, err, c.character.Pets)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || stored[0].Pets[0].Skills[0].Grade != 10 || stored[0].Pets[0].Skills[0].Exp != 0 {
		t.Fatal(stored, err)
	}
}

func TestBattleNativeSkillGradeProgressPersistsBeyondTen(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	const skillID = 50000
	s.Assets.Skills[skillID] = assets.Skill{ID: skillID, MaximumGrade: assets.GradeLimit(15)}
	c.character.Skills = []game.LearnedSkill{{ID: skillID, Grade: 10, EXP: 999}}
	pet := game.Pet{ID: 10501, Skills: []game.PetSkill{{ID: skillID, Grade: 14, Exp: 1399}}}
	c.character.Pets = []game.Pet{pet}
	run := c.battle
	run.members[0].pet = battle.PetFighter(pet, c.character.ID, nil, 0)
	_, err := s.commitSkillUses(c, run, []battle.Action{
		{Actor: run.members[0].self, Skill: skillID, SkillEXP: 1},
		{Actor: run.members[0].pet, Skill: skillID, SkillEXP: 1},
	})
	if err != nil || c.character.Skills[0].Grade != 11 || c.character.Pets[0].Skills[0].Grade != 15 {
		t.Fatal("native player/pet progression", c.character.Skills, c.character.Pets, err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil || len(stored) == 0 || stored[0].Skills[0].Grade != 11 || stored[0].Pets[0].Skills[0].Grade != 15 || stored[0].Pets[0].Skills[0].Exp != 0 {
		t.Fatal("high native grades were not persisted", stored, err)
	}
}

func TestEventSkillRewardPersistenceAndUnknownPreflight(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "learn", false: "unknown"}[known], func(t *testing.T) {
			s, c, wire := eventFixture(t)
			ctx := context.Background()
			if known {
				s.Assets.Skills[15101] = assets.Skill{ID: 15101}
			}
			ev := assets.Event{ClickID: 77, Branches: []assets.Branch{{Index: 1, Operations: []assets.Operation{
				evOp(1, 1, 1, 2, 0, 10), // No gold is delivered if the later skill is unknown.
				evOp(2, 11, 15101, 1, 0, 1),
			}}}}
			before := c.character.Gold
			if err := s.startEvent(ctx, c, 5, &ev, 0, false); err != nil {
				t.Fatal(err)
			}
			got := wire.packets(t)
			chars, err := s.Store.Characters(ctx, c.account.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, skill := range chars[0].Skills {
				found = found || skill.ID == 15101
			}
			if found != known {
				t.Fatal("skill persistence", chars[0].Skills)
			}
			if known {
				if chars[0].Gold != before+10 || !contains(got, []byte{5, 16, 0, 0xfd, 0x3a, 1}) {
					t.Fatal("skill reward", got)
				}
			} else if chars[0].Gold != before {
				t.Fatal("unknown skill allowed earlier reward")
			}
		})
	}
}

func TestAllocationUnlockCommitsWithStats(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	c.character.StatPoints = 1
	c.character.Base.Strength = 15 - game.CharacterBonuses(c.character.Body, c.character.Head).Strength
	beforeHP, beforeSP := c.character.HP, c.character.SP
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 1, 0}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil || stored[0].StatPoints != 0 {
		t.Fatal(stored, err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15101 && sk.Grade == 1 {
			found = true
		}
	}
	if !found || !contains(wires[0].packets(t), protocol.Builder{5, 12}.U16(15101).U8(1)) {
		t.Fatal("unlock not persisted/sent")
	}
	if c.character.HP != beforeHP || c.character.SP != beforeSP {
		t.Fatal("allocation refilled HP/SP")
	}
}

func TestCombatEvolutionCommitsWithGrade(t *testing.T) {
	s, c, _ := battleFixture(t, 1000)
	defer s.abandonBattle(c)
	s.Assets.Skills[15104] = assets.Skill{ID: 15104}
	c.character.Skills = []game.LearnedSkill{{ID: 11166, Grade: 9, EXP: 899}}
	packets, err := s.commitSkillUses(c, c.battle, []battle.Action{{Actor: c.battle.members[0].self, Skill: 11166}})
	if err != nil || !contains(packets, protocol.Builder{5, 12}.U16(15104).U8(1)) {
		t.Fatal(packets, err)
	}
	stored, err := s.Store.Characters(context.Background(), c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15104 && sk.Grade == 1 {
			found = true
		}
	}
	if stored[0].Skills[0].Grade != 10 || !found {
		t.Fatal(stored[0].Skills)
	}
}

func TestLoginUnlockIsPersistedBeforeSnapshot(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	next := c.character.Clone()
	next.Base.Strength = 16 - game.CharacterBonuses(next.Body, next.Head).Strength
	if err := s.commit(ctx, c, next); err != nil {
		t.Fatal(err)
	}
	s.Assets.Skills[15101] = assets.Skill{ID: 15101, TableOrder: 900}
	c.character = nil
	wires[0].Reset()
	if err := s.dispatch(ctx, c, []byte{63, 2, 1}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Store.Characters(ctx, c.account.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sk := range stored[0].Skills {
		if sk.ID == 15101 {
			found = true
		}
	}
	if !found || c.character == nil {
		t.Fatal("login unlock missing")
	}
	// Snapshot only: no allocation packets are allowed to overwrite login stats.
	if contains(wires[0].packets(t), protocol.Builder{8, 1, 110, 1}.U32(1).U32(15101)) {
		t.Fatal("incremental unlock sent during login")
	}
}

func TestAllocationUnlockSaveFailureHasNoSuccess(t *testing.T) {
	s, players, wires := worldFixture(t)
	c := players[0]
	ctx := context.Background()
	if err := s.worldCommand(ctx, c, []byte{12, 1}); err != nil {
		t.Fatal(err)
	}
	wires[0].Reset()
	s.Assets.Skills[15101] = assets.Skill{ID: 15101}
	c.character.StatPoints = 1
	c.character.Base.Strength = 15 - game.CharacterBonuses(c.character.Body, c.character.Head).Strength
	before := c.character.Base.Strength
	s.Store.Close()
	if err := s.worldCommand(ctx, c, []byte{8, game.StatSTR, 1, 0}); err == nil {
		t.Fatal("save failure ignored")
	}
	if c.character.Base.Strength != before || c.character.StatPoints != 1 || wires[0].Len() != 0 {
		t.Fatal("failed allocation changed state or sent success")
	}
}
