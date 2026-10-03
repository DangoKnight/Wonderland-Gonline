package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"os"
	"wonderland-go/internal/assetdb"
	"wonderland-go/internal/assets"
	"wonderland-go/internal/assetsql"
)

type AssetDocument struct {
	Asset  string `json:"asset"`
	Origin string `json:"origin"`
	Source string `json:"source"`
}
type AssetEdit struct {
	Version string          `json:"version"`
	Value   json.RawMessage `json:"value"`
}

func (s *Server) AssetDocuments(ctx context.Context) ([]AssetDocument, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	var docs []assetdb.Document
	if err = db.WithContext(ctx).Select("asset", "origin", "source").Order("asset").Find(&docs).Error; err != nil {
		return nil, err
	}
	out := []AssetDocument{}
	for _, d := range docs {
		out = append(out, AssetDocument{d.Asset, d.Origin, d.Source})
	}
	return out, nil
}
func (s *Server) ReadAssetDocument(ctx context.Context, asset string) (AssetEdit, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return AssetEdit{}, err
	}
	defer assetdb.Close(db)
	raw, err := assetdb.ReadDocument(db.WithContext(ctx), asset)
	if err != nil {
		return AssetEdit{}, err
	}
	return AssetEdit{Version: assetdb.DocumentVersion(raw), Value: raw}, nil
}
func (s *Server) ReadAssetRecords(ctx context.Context, asset string, id int64) ([]assetdb.Record, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	var rows []assetdb.Record
	err = db.WithContext(ctx).Where(map[string]any{"asset": asset, "game_id": id}).Order("ordinal").Limit(20).Find(&rows).Error
	return rows, err
}
func (s *Server) EditAsset(ctx context.Context, asset string, edit AssetEdit, collection string, ordinal int) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	s.worldMu.Lock()
	defer s.worldMu.Unlock()
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()
	for _, c := range s.friendSessions {
		if !gmIdle(c) {
			return errors.New("finish active interactions before editing assets")
		}
	}
	s.mu.Lock()
	loading := false
	for _, c := range s.sessions {
		if c.info.CharacterID != 0 && s.friendSessions[c.info.CharacterID] != c {
			loading = true
			break
		}
	}
	s.mu.Unlock()
	if loading {
		return errors.New("finish map loading before editing assets")
	}
	if _, err := os.Stat(s.Config.AssetsDatabase); err != nil {
		return err
	}
	db, err := assetdb.Open(s.Config.AssetsDatabase)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	var candidate *assets.Catalog
	validate := func(tx *gorm.DB) error {
		var err error
		candidate, err = assetsql.LoadTransaction(tx)
		if err != nil {
			return err
		}
		if err := validateAdminAsset(asset, edit.Value, candidate); err != nil {
			return err
		}
		for id := range s.Assets.Maps {
			if _, ok := candidate.Maps[id]; !ok {
				return fmt.Errorf("cannot remove existing map %d", id)
			}
		}
		return nil
	}
	if collection == "" {
		err = assetdb.ReplaceDocument(ctx, db, asset, edit.Version, edit.Value, validate)
	} else {
		err = assetdb.ReplaceRecord(ctx, db, asset, collection, ordinal, edit.Version, edit.Value, validate)
	}
	if err != nil {
		return err
	}
	candidate.AssetsDatabase = s.Config.AssetsDatabase
	*s.Assets = *candidate
	s.World.ReloadCatalog(s.Assets)
	// Reconnect completes native catalog and map snapshots coherently.
	for _, c := range s.friendSessions {
		if err := s.autosaveSession(ctx, c); err != nil {
			s.Log.Error("asset edit autosave failed", "character", c.character.ID, "error", err)
		}
		c.conn.Close()
	}
	s.Log.Info("administrator asset edit", "asset", asset, "collection", collection, "ordinal", ordinal)
	return nil
}
func (s *Server) EnsureAdminAsset(ctx context.Context, asset string) error {
	s.adminEditMu.Lock()
	defer s.adminEditMu.Unlock()
	if _, err := os.Stat(s.Config.AssetsDatabase); err != nil {
		return err
	}
	db, err := assetdb.Open(s.Config.AssetsDatabase)
	if err != nil {
		return err
	}
	defer assetdb.Close(db)
	return assetdb.EnsureDocument(ctx, db, asset)
}
