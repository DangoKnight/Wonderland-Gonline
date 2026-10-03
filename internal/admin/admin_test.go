package admin

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/config"
	"wonderland-go/internal/server"
	"wonderland-go/internal/store"
)

func TestAdministrationAccessAndSettings(t *testing.T) {
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	s := server.New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := "test-only-administrator-token"
	h, e := New(s, token)
	if e != nil {
		t.Fatal(e)
	}
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/accounts", "", ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/", "", ""); w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte("Wonderland")) {
		t.Fatal(w.Code)
	}
	if w := request("POST", "/register", `{"username":"tester","password":"password","email":""}`, ""); w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("GET", "/api/accounts", "", token); w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("password")) {
		t.Fatal(w.Body.String())
	}
	if w := request("PUT", "/api/settings", `{"server_name":"My Wonderland","motd":"Welcome"}`, token); w.Code != 200 || s.Name() != "My Wonderland" {
		t.Fatal(w.Code)
	}
	if w := request("PUT", "/api/settings", `{"server_name":"Other","unknown":"value"}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	settings, e := db.Settings(context.Background())
	if e != nil || settings["server_name"] != "My Wonderland" {
		t.Fatal(settings, e)
	}
	if w := request("POST", "/api/accounts/1/gm", `{"gm_level":1}`, ""); w.Code != 401 {
		t.Fatal("unauthenticated GM grant", w.Code)
	}
	if w := request("POST", "/api/accounts/1/gm", `{"gm_level":1}`, token); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if a, e := db.Authenticate(context.Background(), "tester", "password"); e != nil || a.GMLevel != 1 {
		t.Fatal("GM level not granted", a, e)
	}
	if w := request("POST", "/api/accounts/1/gm", `{"gm_level":300}`, token); w.Code != 400 {
		t.Fatal("accepted out-of-range GM level", w.Code)
	}
	if w := request("POST", "/api/accounts/1/ban", `{"banned":true}`, token); w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	if _, e := db.Authenticate(context.Background(), "tester", "password"); e == nil {
		t.Fatal("banned account authenticated")
	}
	if w := request("POST", "/register", `{"username":"tester","password":"password"} {}`, ""); w.Code != 400 {
		t.Fatal("accepted trailing JSON")
	}
}
