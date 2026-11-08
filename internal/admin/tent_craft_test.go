package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
	"wonderland-gonline/internal/config"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/server"
	"wonderland-gonline/internal/store"
)

func tentCraftHandler(t *testing.T) (*server.Server, func(method, body string) *httptest.ResponseRecorder) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	catalog := &assets.Catalog{
		Items: map[uint16]game.ItemDefinition{
			38027: {ID: 38027, Name: "Coconut Basin"},
			38014: {ID: 38014, Name: "Low-Temperature Kiln"},
		},
		Manufacturing: map[uint16]assets.ManufacturingFormula{
			511: {ID: 511, Output: assets.ManufacturingInput{ItemID: 38027, Count: 1}, DurationSeconds: 0, TentOutput: true},
			499: {ID: 499, Output: assets.ManufacturingInput{ItemID: 38014, Count: 1}, DurationSeconds: 60, TentOutput: true},
			7:   {ID: 7, Output: assets.ManufacturingInput{ItemID: 38027, Count: 1}, DurationSeconds: 30},
		},
	}
	s := server.New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	const token = "test-only-administrator-token"
	handler, err := New(s, token)
	if err != nil {
		t.Fatal(err)
	}
	return s, func(method, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/tent-craft", bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
}

func TestTentCraftListsFormulas(t *testing.T) {
	_, request := tentCraftHandler(t)
	w := request("GET", "")
	if w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	var body struct {
		Rows []struct {
			FormulaID uint16 `json:"formula_id"`
			ItemID    uint16 `json:"item_id"`
			Name      string `json:"name"`
			Duration  uint32 `json:"duration_seconds"`
			Tent      bool   `json:"tent_output"`
		} `json:"formulas"`
		Max uint32 `json:"max_duration_seconds"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Rows) != 3 {
		t.Fatalf("rows %d", len(body.Rows))
	}
	// Tent recipes come first, then the rest by formula id.
	if body.Rows[0].FormulaID != 499 || body.Rows[1].FormulaID != 511 || body.Rows[2].FormulaID != 7 {
		t.Fatalf("order %+v", body.Rows)
	}
	if body.Rows[0].Name != "Low-Temperature Kiln" || !body.Rows[0].Tent || body.Rows[0].Duration != 60 {
		t.Fatalf("row %+v", body.Rows[0])
	}
	if body.Max != assets.ManufacturingMaxDurationSeconds {
		t.Fatalf("max %d", body.Max)
	}
}

func TestTentCraftRejectsInvalidRequests(t *testing.T) {
	_, request := tentCraftHandler(t)
	for _, tc := range []struct{ name, body string }{
		{"bad json", `{`},
		{"no selection", `{"all":false}`},
		{"unknown field", `{"ids":[1],"duration_seconds":0,"nope":1}`},
	} {
		if w := request("POST", tc.body); w.Code != 400 {
			t.Fatalf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
	}
}

func TestTentCraftRejectsLongDurations(t *testing.T) {
	_, request := tentCraftHandler(t)
	w := request("POST", `{"all":true,"duration_seconds":700000}`)
	if w.Code == 200 {
		t.Fatalf("oversized duration accepted: %s", w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("duration above")) {
		t.Fatalf("unclear error: %s", w.Body.String())
	}
}

func TestTentCraftUpdatesRepositoryCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assets.db")
	if _, err := assetsql.InitializeRepositoryDatabase(path); err != nil {
		t.Fatal(err)
	}
	catalog, err := assetsql.LoadDatabase(path)
	if err != nil {
		t.Fatal(err)
	}
	var formula uint16
	var duration uint32
	for id, f := range catalog.Manufacturing {
		if f.DurationSeconds > 0 && f.DurationSeconds%assets.ManufacturingMinutesToSeconds == 0 {
			formula, duration = id, f.DurationSeconds
			break
		}
	}
	if formula == 0 {
		t.Fatal("repository catalog has no timed formula")
	}
	assetsDB, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(assetsDB)
	var records int64
	if err := assetsDB.Model(&assetdb.Record{}).Count(&records).Error; err != nil || records != 0 {
		t.Fatal("repository seed unexpectedly depends on native archive records", records, err)
	}
	assetdb.Close(assetsDB)
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := server.New(config.Default(), db, catalog, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Config.AssetsDatabase = path
	const token = "test-only-administrator-token"
	handler, err := New(s, token)
	if err != nil {
		t.Fatal(err)
	}
	request := func(body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "/api/tent-craft", bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := request(`{"ids":[` + strconv.Itoa(int(formula)) + `],"duration_seconds":0}`)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"updated":1`)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if got := s.AssetSnapshot().Manufacturing[formula].DurationSeconds; got != 0 {
		t.Fatalf("served duration %d", got)
	}
	for _, id := range []uint16{game.TentMapID, game.TentUpperMapID} {
		if _, ok := s.Assets.Maps[id]; !ok {
			t.Fatalf("tent map %d lost after a duration update", id)
		}
	}
	w = request(`{"reset":true}`)
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"updated":`)) {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	if got := s.AssetSnapshot().Manufacturing[formula].DurationSeconds; got != duration {
		t.Fatalf("reset duration %d, want %d", got, duration)
	}
	for _, id := range []uint16{game.TentMapID, game.TentUpperMapID} {
		if _, ok := s.Assets.Maps[id]; !ok {
			t.Fatalf("tent map %d lost after a catalog reload", id)
		}
	}
	ctx := context.Background()
	story, err := s.ReadAssetDocument(ctx, "Story")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.EditAsset(ctx, "Story", story, "", 0); err != nil {
		t.Fatalf("definition edit after a catalog reload: %v", err)
	}
	for _, id := range []uint16{game.TentMapID, game.TentUpperMapID} {
		if _, ok := s.Assets.Maps[id]; !ok {
			t.Fatalf("tent map %d lost after a definition edit", id)
		}
	}
	// A catalog validation failure must roll back the duration update as well
	// as retaining the served snapshot. Work only on this isolated test catalog.
	invalidDB, err := assetdb.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer assetdb.Close(invalidDB)
	if err := invalidDB.Model(&assetsql.ManufacturingRow{}).Where("manufacturing_key = ?", formula).Update("value_output_count", 0).Error; err != nil {
		t.Fatal(err)
	}
	w = request(`{"ids":[` + strconv.Itoa(int(formula)) + `],"duration_seconds":0}`)
	if w.Code == 200 {
		t.Fatal("invalid catalog update reported success")
	}
	var after assetsql.ManufacturingRow
	if err := invalidDB.First(&after, "manufacturing_key = ?", formula).Error; err != nil {
		t.Fatal(err)
	}
	if after.ValueDurationSeconds != duration || s.AssetSnapshot().Manufacturing[formula].DurationSeconds != duration {
		t.Fatal("failed catalog reload committed the duration edit")
	}
}
