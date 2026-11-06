package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-gonline/internal/clientfs"
)

func TestNormalizedAudioNativeReferencesAndMusic(t *testing.T) {
	c, _, _ := enteredClient(t)
	voice, err := clientfs.ReadFile(c.Assets.DataPath("audio", "odd_d01", "57153.ogg"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sound"), 0755); err != nil {
		t.Fatal(err)
	}
	ogg := filepath.Join(root, "sound", "BGM0013.ogg")
	wav := filepath.Join(root, "sound", "BGM0013.wav")
	if err := os.WriteFile(ogg, voice, 0600); err != nil {
		t.Fatal(err)
	}
	// A stale WAV must not hide the standardized extraction's Vorbis file.
	if err := os.WriteFile(wav, []byte("old wave"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := audioAssetPath(wav); got != ogg {
		t.Fatal("native WAV reference did not select normalized audio", got)
	}
	if pcm := decodeAudioPCM(wav); len(pcm) == 0 {
		t.Fatal("sound effect/ambient decoder could not read normalized audio")
	}
	raw, resolved, err := readAudioAsset(root, `sound\BGM0013.wav`)
	if err != nil || !strings.HasSuffix(resolved, ".ogg") {
		t.Fatal("music asset resolution failed", resolved, err)
	}
	stream, err := decodeAudioStream(raw, resolved, sampleRate)
	if err != nil || stream.Length() <= 0 {
		t.Fatal("music decoder could not stream normalized audio", err)
	}
	if _, err := stream.Seek(0, 0); err != nil {
		t.Fatal("normalized music cannot loop", err)
	}
	if err := os.Remove(ogg); err != nil {
		t.Fatal(err)
	}
	if got := audioAssetPath(wav); got != wav {
		t.Fatal("older WAV-only extraction lost compatibility", got)
	}
}

func TestNormalizedExtractedMedia(t *testing.T) {
	root := os.Getenv("WONDERLAND_TEST_NORMALIZED_MEDIA")
	if root == "" {
		t.Skip("set WONDERLAND_TEST_NORMALIZED_MEDIA to a fresh standardized media extraction")
	}
	for _, path := range []string{`Sound\BGM0013.wav`, `sound\wav0152.wav`, `voice\am_003.wav`} {
		raw, resolved, err := readAudioAsset(root, path)
		if err != nil || !strings.HasSuffix(resolved, ".ogg") {
			t.Fatal("normalized extracted resource missing", path, resolved, err)
		}
		stream, err := decodeAudioStream(raw, resolved, sampleRate)
		if err != nil {
			t.Fatal(path, err)
		}
		buffer := make([]byte, 512)
		if n, err := stream.Read(buffer); n == 0 || err != nil {
			t.Fatal("normalized resource has no playable samples", path, n, err)
		}
		if _, err := stream.Seek(0, 0); err != nil {
			t.Fatal("normalized resource cannot rewind", path, err)
		}
	}
}
