package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	coreAssetPack        = "client-assets.zip"
	assetPackVersion     = 1
	maxCompanionPacks    = 16
	maxPackManifestBytes = 64 << 10
)

// Read the authored pack set instead of guessing from directory contents.
// Old content-addressed bundles and unrelated private files are never served.
func assetPackNames(directory string) ([]string, error) {
	z, err := zip.OpenReader(filepath.Join(directory, coreAssetPack))
	if err != nil {
		return nil, err
	}
	defer z.Close()
	names := []string{coreAssetPack}
	seen := map[string]bool{coreAssetPack: true}
	manifestFound := false
	for _, entry := range z.File {
		if entry.Name != "_packs.json" {
			continue
		}
		if manifestFound || entry.UncompressedSize64 > maxPackManifestBytes {
			return nil, fmt.Errorf("invalid asset pack manifest")
		}
		manifestFound = true
		r, err := entry.Open()
		if err != nil {
			return nil, err
		}
		var manifest struct {
			Version int      `json:"version"`
			Packs   []string `json:"packs"`
		}
		err = json.NewDecoder(io.LimitReader(r, maxPackManifestBytes)).Decode(&manifest)
		r.Close()
		if err != nil {
			return nil, err
		}
		if manifest.Version != assetPackVersion || len(manifest.Packs) > maxCompanionPacks {
			return nil, fmt.Errorf("unsupported asset pack manifest")
		}
		for _, name := range manifest.Packs {
			if !fs.ValidPath(name) || strings.ContainsAny(name, "/\\:") || !strings.HasSuffix(name, ".zip") || seen[name] {
				return nil, fmt.Errorf("invalid companion pack %q", name)
			}
			info, err := os.Lstat(filepath.Join(directory, name))
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("companion pack %q is not a regular file", name)
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names, nil
}
