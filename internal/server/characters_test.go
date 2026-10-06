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
	return &assets.Catalog{Maps: map[uint16]assets.Map{10017: {ID: 10017}}, Items: map[uint16]game.ItemDefinition{21001: {ID: 21001, EquipSlot: 2}, 24001: {ID: 24001, EquipSlot: 5}, 32176: {ID: 32176, Type: 23}}, StarterItems: []game.StarterGrant{{ID: 32176, Count: 50}}, Skills: map[uint16]assets.Skill{15003: {TableOrder: 188}, 11016: {TableOrder: 1}, 11166: {TableOrder: 2}, 11056: {TableOrder: 3}}}
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
	if p := wire.packets(t); len(p) != 1 || !bytes.Equal(p[0], []byte{5, 4}) {
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
