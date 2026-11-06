package app

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"

	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/internal/clientfs"
)

// Normalized extraction uses Vorbis for every audio asset. Native resource
// references still contain .wav (including BGM names); keep those identities
// and resolve their exported representation at the asset boundary.
func audioAssetPath(path string) string {
	ext := filepath.Ext(path)
	switch strings.ToLower(ext) {
	case ".wav", ".mp3", ".flac", ".aif", ".aiff", ".m4a", ".oga":
		ogg := strings.TrimSuffix(path, ext) + ".ogg"
		if _, err := clientfs.Stat(ogg); err == nil {
			return ogg
		}
	}
	return path
}

type decodedAudioStream interface {
	io.ReadSeeker
	Length() int64
}

func decodeAudioStream(raw []byte, path string, rate int) (decodedAudioStream, error) {
	if strings.EqualFold(filepath.Ext(path), ".ogg") {
		return vorbis.DecodeWithSampleRate(rate, bytes.NewReader(raw))
	}
	return wav.DecodeWithSampleRate(rate, bytes.NewReader(raw))
}

func readAudioAsset(root, path string) ([]byte, string, error) {
	resolved := audioAssetPath(login.Path(root, strings.Split(path, `\`)...))
	raw, err := clientfs.ReadFile(resolved)
	return raw, resolved, err
}
