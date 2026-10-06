package admin

import (
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

func TestSessionTerminationAuthorizationAndValidation(t *testing.T) {
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
	for _, tt := range []struct {
		id, token string
		status    int
	}{
		{"1", "", 401}, {"1", "wrong-token", 401}, {"0", token, 400}, {"nope", token, 400}, {"18446744073709551616", token, 400}, {"1", token, 404},
	} {
		r := httptest.NewRequest("POST", "/api/sessions/"+tt.id+"/kick", nil)
		r.Header.Set("Authorization", "Bearer "+tt.token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Fatal(tt.id, w.Code, w.Body.String())
		}
	}
}
