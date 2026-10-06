package app

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"wonderland-gonline/internal/clientfs"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"wonderland-gonline/client/wlo/login"
)

// sampleRate is the mixing rate; wave files are resampled to it.
const sampleRate = 44100

// Sounds plays the client's wave files (FUN_00404fa4), given paths relative
// to the client directory with backslashes.
type Sounds struct {
	Root       string
	Shared     *SoundCache
	mu         sync.Mutex
	volume     float64
	configured bool
}

// SoundCache owns the workspace's one audio context and decoded effects.
type SoundCache struct {
	once  sync.Once
	ctx   *audio.Context
	mu    sync.Mutex
	cache map[string][]byte
}

func (s *Sounds) Context() *audio.Context {
	s.mu.Lock()
	if s.Shared == nil {
		s.Shared = &SoundCache{}
	}
	shared := s.Shared
	s.mu.Unlock()
	shared.once.Do(func() { shared.ctx = audio.NewContext(sampleRate); shared.cache = map[string][]byte{} })
	return shared.ctx
}

// Play starts a sound; missing or unreadable files are ignored.
func (s *Sounds) Play(path string) {
	s.mu.Lock()
	v := 1.0
	if s.configured {
		v = s.volume
	}
	s.mu.Unlock()
	if v == 0 {
		return
	}
	if pcm := s.PCM(path); len(pcm) > 0 {
		p := s.Context().NewPlayerFromBytes(pcm)
		p.SetVolume(v)
		p.Play()
	}
}

// PCM is a sound's decoded 16-bit stereo samples at the mixing rate, read
// once; nil for a missing or unreadable file.
func (s *Sounds) PCM(path string) []byte {
	s.Context()
	s.Shared.mu.Lock()
	defer s.Shared.mu.Unlock()
	key := login.Path(s.Root, strings.Split(path, `\`)...)
	pcm, ok := s.Shared.cache[key]
	if !ok {
		raw, err := clientfs.ReadFile(login.Path(s.Root, strings.Split(path, `\`)...))
		if err == nil {
			if st, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(raw)); err == nil {
				pcm, _ = io.ReadAll(st)
			}
		}
		s.Shared.cache[key] = pcm
	}
	return pcm
}

// SetVolume controls subsequent sound effects; ambient loops are updated by Client.
func (s *Sounds) SetVolume(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = max(0, min(1, v))
	s.configured = true
}
