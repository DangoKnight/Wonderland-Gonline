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

func TestPublicAccountRegistrationPage(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := server.New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h, err := New(s, "test-only-administrator-token")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
		return w
	}
	for _, path := range []string{"/register", "/register.css", "/register.js"} {
		w := request("GET", path, "")
		if w.Code != http.StatusOK || w.Header().Get("Content-Security-Policy") == "" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("public registration resource %s: %d", path, w.Code)
		}
	}
	if w := request("GET", "/register", ""); !bytes.Contains(w.Body.Bytes(), []byte("Create your account")) || !bytes.Contains(w.Body.Bytes(), []byte(`autocomplete="new-password"`)) {
		t.Fatal("registration URL does not serve the public form")
	}
	body := `{"username":"player0001","password":"Secret1234","email":"player@example.com"}`
	w := request("POST", "/register", body)
	if w.Code != http.StatusCreated || bytes.Contains(w.Body.Bytes(), []byte("Secret1234")) {
		t.Fatal("registration failed or exposed password", w.Code, w.Body.String())
	}
	if _, err := db.Authenticate(context.Background(), "player0001", "Secret1234"); err != nil {
		t.Fatal("new account cannot authenticate", err)
	}
	if w := request("POST", "/register", body); w.Code != http.StatusConflict {
		t.Fatal("duplicate account accepted", w.Code)
	}
	if w := request("POST", "/register", `{"username":"invalid!","password":"Secret1234"}`); w.Code != http.StatusBadRequest {
		t.Fatal("invalid registration accepted", w.Code)
	}
	if w := request("GET", "/api/accounts", ""); w.Code != http.StatusUnauthorized {
		t.Fatal("public form bypassed administrator authentication", w.Code)
	}
}
