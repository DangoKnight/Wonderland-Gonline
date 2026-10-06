package clientassets

import (
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"wonderland-gonline/internal/clientfs"
)

// SpriteJSON opens the lossless verification bytes embedded in sprites.json.
// Temporary decoded storage is removed on Close; no native payload is kept in data.
type SpriteJSON struct {
	Archive *SpriteArchive
	file    *os.File
	path    string
}

func (s *SpriteJSON) Close() error {
	err := s.file.Close()
	removeErr := os.Remove(s.path)
	if err != nil {
		return err
	}
	if errors.Is(removeErr, os.ErrNotExist) {
		return nil // already unlinked at creation
	}
	return removeErr
}
func OpenSpriteJSON(path string) (*SpriteJSON, error) {
	f, err := clientfs.Open(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Data   string `json:"decoded_archive_zlib_base64"`
		Bytes  int64  `json:"decoded_archive_bytes"`
		SHA256 string `json:"payload_sha256"`
	}
	err = json.NewDecoder(f).Decode(&doc)
	f.Close()
	if err != nil {
		return nil, err
	}
	if doc.Data == "" || doc.Bytes <= 0 {
		return nil, fmt.Errorf("%s: missing lossless sprite data", path)
	}
	reader, err := zlib.NewReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(doc.Data)))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	tmp, err := os.CreateTemp("", "wonderland-sprite-json-*.bin")
	if err != nil {
		return nil, err
	}
	// Where open files can be unlinked, the decoded bytes live only as
	// long as the descriptor, even if the process ends without Close.
	if runtime.GOOS != "windows" {
		os.Remove(tmp.Name())
	}
	keep := false
	defer func() {
		if !keep {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(reader, doc.Bytes+1))
	if err != nil {
		return nil, err
	}
	if size != doc.Bytes || hex.EncodeToString(hash.Sum(nil)) != doc.SHA256 {
		return nil, fmt.Errorf("%s: lossless sprite checksum mismatch", path)
	}
	archive, err := OpenSpriteArchive(tmp, size)
	if err != nil {
		return nil, err
	}
	keep = true
	return &SpriteJSON{Archive: archive, file: tmp, path: tmp.Name()}, nil
}
