package admin

import (
	"bytes"
	"context"
	"errors"
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

func TestAdminAccountDeletionAndPasswordReset(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := server.New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := "test-only-administrator-token"
	h, err := New(s, token)
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.Register(context.Background(), "tester", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, auth string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	for _, tt := range []struct {
		method, path, body, auth string
		code                     int
	}{
		{"DELETE", "/api/accounts/1", "", "", 401},
		{"POST", "/api/accounts/1/password", `{"password":"new-secret"}`, "", 401},
		{"DELETE", "/api/accounts/0", "", token, 400},
		{"DELETE", "/api/accounts/nope", "", token, 400},
		{"DELETE", "/api/accounts/999", "", token, 404},
		{"POST", "/api/accounts/999/password", `{"password":"new-secret"}`, token, 404},
		{"POST", "/api/accounts/1/password", `{}`, token, 400},
		{"POST", "/api/accounts/1/password", `{"password":"abc"}`, token, 400},
		{"POST", "/api/accounts/1/password", `{"password":"new-secret","unknown":1}`, token, 400},
		{"POST", "/api/accounts/1/password", `{"password":"new-secret"} {}`, token, 400},
		{"POST", "/api/accounts/1/password", `{"password":"new-secret"}`, token, 200},
	} {
		w := request(tt.method, tt.path, tt.body, tt.auth)
		if w.Code != tt.code {
			t.Fatal(tt.path, w.Code, w.Body.String())
		}
		if bytes.Contains(w.Body.Bytes(), []byte("new-secret")) {
			t.Fatal("password leaked in response")
		}
	}
	if _, err := db.Authenticate(context.Background(), a.Username, "password"); !errors.Is(err, store.ErrCredentials) {
		t.Fatal("old password accepted", err)
	}
	if _, err := db.Authenticate(context.Background(), a.Username, "new-secret"); err != nil {
		t.Fatal(err)
	}
	if w := request("DELETE", "/api/accounts/1", "", token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w := request("DELETE", "/api/accounts/1", "", token); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request("GET", "/api/accounts", "", token); w.Code != 200 || w.Body.String() != "[]\n" {
		t.Fatal("deleted account still listed", w.Body.String())
	}
	db.Close()
	if w := request("POST", "/api/accounts/1/password", `{"password":"new-secret"}`, token); w.Code != 500 {
		t.Fatal("DB error not reported", w.Code)
	}
}
