package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"os"
	"wonderland-gonline/internal/assetdb"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/assetsql"
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
	out := []AssetDocument{}
	for _, name := range assetsql.DefinitionNames {
		out = append(out, AssetDocument{Asset: name, Origin: "SQL", Source: "assets database"})
	}
	return out, nil
}
func (s *Server) ReadAssetDocument(ctx context.Context, asset string) (AssetEdit, error) {
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return AssetEdit{}, err
	}
	defer assetdb.Close(db)
	var raw []byte
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var err error; raw, err = assetsql.ReadDefinition(tx, asset); return err })
	if err != nil {
		return AssetEdit{}, err
	}
	return AssetEdit{Version: assetdb.DocumentVersion(raw), Value: raw}, nil
}

type AssetRecord struct {
	Collection string `json:"collection"`
	Ordinal    int    `json:"ordinal"` // Numeric game identity, independent of source position.
	AssetEdit
}

func (s *Server) ReadAssetRecords(ctx context.Context, asset string, id int64) ([]AssetRecord, error) {
	if id < 0 || uint64(id) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("invalid asset identity")
	}
	db, err := assetdb.OpenReadOnly(s.Config.AssetsDatabase)
	if err != nil {
		return nil, err
	}
	defer assetdb.Close(db)
	var raw []byte
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		raw, err = assetsql.ReadDefinitionRecord(tx, asset, int(id))
		return err
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return []AssetRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []AssetRecord{{Collection: asset, Ordinal: int(id), AssetEdit: AssetEdit{Version: assetdb.DocumentVersion(raw), Value: raw}}}, nil
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
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []byte
		var err error
		var recordID *int
		if collection == "" {
			current, err = assetsql.ReadDefinition(tx, asset)
		} else {
			if collection != asset {
				return errors.New("record collection does not match dataset")
			}
			recordID = &ordinal
			current, err = assetsql.ReadDefinitionRecord(tx, asset, ordinal)
		}
		if err != nil {
			return err
		}
		if edit.Version == "" || assetdb.DocumentVersion(current) != edit.Version {
			return assetdb.ErrEditConflict
		}
		candidate, err = assetsql.DefinitionCandidate(tx, asset, edit.Value, recordID)
		if err != nil {
			return err
		}
		for id := range s.Assets.Maps {
			if _, ok := candidate.Maps[id]; !ok {
				return fmt.Errorf("cannot remove existing map %d", id)
			}
		}
		return assetsql.SaveEditedCatalog(tx, candidate)
	})
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
	_, err = assetsql.ReadDefinition(db.WithContext(ctx), asset)
	return err
}
