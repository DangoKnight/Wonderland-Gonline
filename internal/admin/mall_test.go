package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
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

func TestAdminMallAdjustmentValidationAndAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "admin.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	account, err := db.Register(context.Background(), "shopper", "password", "")
	if err != nil {
		t.Fatal(err)
	}
	s := server.New(config.Default(), db, &assets.Catalog{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	token := "test-only-administrator-token"
	handler, err := New(s, token)
	if err != nil {
		t.Fatal(err)
	}
	route := fmt.Sprintf("/api/accounts/%d/mall", account.ID)
	request := func(path, body, auth string) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		if auth != "" {
			r.Header.Set("Authorization", "Bearer "+auth)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request(route, `{"currency":"points","delta":50}`, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	for _, body := range []string{`{}`, `{"currency":"points"}`, `{"delta":1}`, `{"currency":"unknown","delta":1}`, `{"currency":"points","delta":0}`, `{"currency":"points","delta":1.5}`, `{"currency":"points","delta":2147483648}`, `{"currency":"points","delta":1,"extra":1}`, `{"currency":"points","delta":1} {}`} {
		if w := request(route, body, token); w.Code != 400 {
			t.Fatal(body, w.Code, w.Body.String())
		}
	}
	if w := request("/api/accounts/0/mall", `{"currency":"points","delta":1}`, token); w.Code != 400 {
		t.Fatal(w.Code)
	}
	if w := request("/api/accounts/999999/mall", `{"currency":"points","delta":1}`, token); w.Code != 404 {
		t.Fatal(w.Code)
	}
	if w := request(route, `{"currency":"points","delta":50}`, token); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	w := request(route, `{"currency":"bonus","delta":10}`, token)
	var response struct {
		OK       bool               `json:"ok"`
		Balances store.MallBalances `json:"balances"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || w.Code != 200 || !response.OK || response.Balances != (store.MallBalances{Points: 50, Bonus: 10}) {
		t.Fatal(w.Code, w.Body.String(), err)
	}
	w = request(route, `{"currency":"points","delta":-100}`, token)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	var count int
	if err := raw.QueryRow("SELECT count(*) FROM audit WHERE action='mall_adjust'").Scan(&count); err != nil || count != 3 {
		t.Fatal(count, err)
	}
	if balance, err := db.MallBalances(context.Background(), account.ID); err != nil || balance != (store.MallBalances{Points: 0, Bonus: 10}) {
		t.Fatal(balance, err)
	}
	if _, err := raw.Exec("CREATE TRIGGER fail_audit BEFORE INSERT ON audit BEGIN SELECT RAISE(ABORT,'audit unavailable'); END"); err != nil {
		t.Fatal(err)
	}
	if w := request(route, `{"currency":"points","delta":5}`, token); w.Code != 500 {
		t.Fatal(w.Code, w.Body.String())
	}
	if balance, err := db.MallBalances(context.Background(), account.ID); err != nil || balance.Points != 0 {
		t.Fatal("API failed adjustment persisted", balance, err)
	}
}
