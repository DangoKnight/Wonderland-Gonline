package assetsql

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"wonderland-gonline/internal/assetdb"
)

// Repository-owned, server-only definitions. Media and gameplay state are never
// included. These inputs are used only for initial creation or explicit rebuild.
//
//go:embed server_defaults/catalog.db.gz
var repositorySeed []byte

//go:embed server_defaults/catalog.db.gz.json
var repositorySeedMetadata []byte

const repositorySeedMaximumBytes = 512 << 20

func repositoryMetadata() (repositorySeedManifest, error) {
	var m repositorySeedManifest
	if err := json.Unmarshal(repositorySeedMetadata, &m); err != nil {
		return m, err
	}
	digest, err := hex.DecodeString(m.DatabaseSHA256)
	if err != nil || len(digest) != sha256.Size || m.Format != repositorySeedFormat || m.Version != repositorySeedVersion || m.SchemaVersion != catalogSchemaVersion || m.DatabaseBytes <= 0 || m.DatabaseBytes > repositorySeedMaximumBytes || m.CompressedBytes != int64(len(repositorySeed)) {
		return m, fmt.Errorf("invalid repository server seed metadata; refresh the tracked seed")
	}
	return m, nil
}

// BuildRepositoryDatabase creates a new SQL catalog exclusively from the
// tracked seed. It refuses to overwrite any existing file.
func BuildRepositoryDatabase(path string) (result assetdb.Summary, err error) {
	m, err := repositoryMetadata()
	if err != nil {
		return result, err
	}
	reader, err := gzip.NewReader(bytes.NewReader(repositorySeed))
	if err != nil {
		return result, err
	}
	defer reader.Close()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	defer func() {
		file.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	digest := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(reader, m.DatabaseBytes+1))
	if err != nil {
		return result, err
	}
	if n != m.DatabaseBytes || hex.EncodeToString(digest.Sum(nil)) != m.DatabaseSHA256 {
		return result, fmt.Errorf("repository server seed checksum/length mismatch")
	}
	if err := file.Sync(); err != nil {
		return result, err
	}
	if err := file.Close(); err != nil {
		return result, err
	}
	if _, err := LoadDatabase(path); err != nil {
		return result, err
	}
	return assetdb.Summary{Assets: m.Tables, Records: int(m.Rows)}, nil
}

// InitializeRepositoryDatabase bootstraps a missing catalog before normal SQL
// loading. Existing databases, including invalid/old ones, are never replaced
// or silently migrated. Publication is atomic and refuses competing writes.
func InitializeRepositoryDatabase(path string) (bool, error) {
	if _, err := os.Lstat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return false, err
	}
	file, err := os.CreateTemp(parent, ".server-initialization-*.db")
	if err != nil {
		return false, err
	}
	staged := file.Name()
	defer os.Remove(staged)
	if err := file.Close(); err != nil {
		return false, err
	}
	if err := os.Remove(staged); err != nil {
		return false, err
	}
	if _, err := BuildRepositoryDatabase(staged); err != nil {
		return false, err
	}
	if err := os.Link(staged, path); err != nil {
		if os.IsExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("publish initialized server catalog: %w", err)
	}
	return true, nil
}
