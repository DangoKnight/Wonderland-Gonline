package clientassets

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpriteJSONBoundsAndChecksum(t *testing.T) {
	// Independent empty JMA archive (14-byte header).
	payload := []byte{'j', 'a', 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	var compressed bytes.Buffer
	z := zlib.NewWriter(&compressed)
	if _, err := z.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	for _, tc := range []struct {
		name string
		size int64
		hash string
		want string
	}{
		{"valid", 14, hex.EncodeToString(hash[:]), ""},
		{"oversized", maxDecodedSpriteJSONBytes + 1, "", "size limit"},
		{"overflow", math.MaxInt64, "", "size limit"},
		{"too much expansion", 1, "", "checksum mismatch"},
		{"checksum", 14, "wrong", "checksum mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"decoded_archive_bytes": tc.size, "payload_sha256": tc.hash, "decoded_archive_zlib_base64": base64.StdEncoding.EncodeToString(compressed.Bytes())})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "sprites.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			got, err := OpenSpriteJSON(path)
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatal("unbounded or invalid archive accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Archive.Entries) != 0 {
				t.Fatal("archive changed")
			}
			if err := got.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSpriteJSONRejectsOversizedInputBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sprites.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// A sparse file verifies the pre-allocation guard without allocating a
	// hostile payload in the test process.
	if err := f.Truncate(maxSpriteJSONBytes + 1); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	if _, err := OpenSpriteJSON(path); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatal("oversized JSON reached parser", err)
	}
}
