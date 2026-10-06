package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/server"
	"wonderland-gonline/internal/store"
)

func TestAdminToolsRequireAuthorizationAndPersistOperations(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := server.New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	const token = "test-only-administrator-token"
	handler, err := New(s, token)
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/operations"}, {"PUT", "/api/operations"}, {"POST", "/api/operations/broadcast"}, {"POST", "/api/operations/reload"}, {"POST", "/api/operations/save"}, {"POST", "/api/operations/starter-packs"}, {"POST", "/api/operations/shutdown"}, {"POST", "/api/operations/kickall"},
		{"GET", "/api/assets/maps/1/actors"}, {"GET", "/api/assets/npcs/1/report"}, {"POST", "/api/characters/1/actors"}, {"GET", "/api/logs"}, {"GET", "/api/audit"}, {"GET", "/api/characters"}, {"GET", "/api/characters/1"}, {"PUT", "/api/characters/1"}, {"DELETE", "/api/characters/1"}, {"POST", "/api/players/1/action"},
		{"GET", "/api/friends"}, {"DELETE", "/api/friends/1/2"}, {"GET", "/api/battles"}, {"POST", "/api/battles/1/abort"}, {"GET", "/api/bans"}, {"POST", "/api/bans"}, {"DELETE", "/api/bans"},
		{"GET", "/api/guilds"}, {"PUT", "/api/guilds"}, {"DELETE", "/api/guilds/1"}, {"GET", "/api/marriages"}, {"DELETE", "/api/marriages/1"}, {"POST", "/api/marriages/1/summon"},
		{"GET", "/api/mail"}, {"POST", "/api/mail"}, {"DELETE", "/api/mail/1"}, {"GET", "/api/assets/documents"}, {"GET", "/api/assets/documents/item.dat"}, {"PUT", "/api/assets/documents/item.dat"}, {"POST", "/api/assets/documents/chest_drops.json/initialize"}, {"GET", "/api/assets/documents/item.dat/records?id=42"}, {"PUT", "/api/assets/documents/item.dat/records/0?collection=items"}, {"GET", "/api/assets/maps/1"}, {"GET", "/api/assets/talks"}, {"GET", "/api/configuration"}, {"PUT", "/api/configuration"},
	} {
		for _, auth := range []string{"", "wrong"} {
			if w := request(route.method, route.path, `{}`, auth); w.Code != 401 {
				t.Fatalf("%s %s: %d %s", route.method, route.path, w.Code, w.Body.String())
			}
		}
	}
	value := s.RuntimeSettings()
	value.ExpRate = 2.5
	value.StatusMode = "red"
	value.LogLevel = "debug"
	raw, _ := json.Marshal(value)
	if w := request("PUT", "/api/operations", string(raw), token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if s.RuntimeSettings().ExpRate != 2.5 {
		t.Fatal("rate not applied")
	}
	stored, err := db.Settings(context.Background())
	if err != nil || stored["exp_rate"] != "2.50" || stored["status_mode"] != "red" {
		t.Fatal(stored, err)
	}
	if w := request("PUT", "/api/operations", `{"exp_rate":0}`, token); w.Code != 400 || s.RuntimeSettings().ExpRate != 2.5 {
		t.Fatal("invalid update applied", w.Code, w.Body.String())
	}
	if w := request("POST", "/api/operations/shutdown", `{"seconds":0}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("PUT", "/api/assets/documents/item.dat/records/0", `{}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/tools.js", "", ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("renderOperations")) {
		t.Fatal("tools not embedded", w.Code)
	}
}
