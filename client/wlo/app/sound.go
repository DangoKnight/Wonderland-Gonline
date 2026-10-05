package app

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"wonderland-go/client/wlo/login"
)

// sampleRate is the mixing rate; wave files are resampled to it.
const sampleRate = 44100

// Sounds plays the client's wave files (FUN_00404fa4), given paths relative
// to the client directory with backslashes.
type Sounds struct {
	Root       string
	once       sync.Once
	ctx        *audio.Context
	mu         sync.Mutex
	cache      map[string][]byte
	volume     float64
	configured bool
}

// Context is the process's one audio context, made on first use.
func (s *Sounds) Context() *audio.Context {
	s.once.Do(func() {
		s.ctx = audio.NewContext(sampleRate)
		s.cache = map[string][]byte{}
	})
	return s.ctx
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
		p := s.ctx.NewPlayerFromBytes(pcm)
		p.SetVolume(v)
		p.Play()
	}
}

// PCM is a sound's decoded 16-bit stereo samples at the mixing rate, read
// once; nil for a missing or unreadable file.
func (s *Sounds) PCM(path string) []byte {
	s.Context()
	s.mu.Lock()
	defer s.mu.Unlock()
	pcm, ok := s.cache[path]
	if !ok {
		raw, err := os.ReadFile(login.Path(s.Root, strings.Split(path, `\`)...))
		if err == nil {
			if st, err := wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(raw)); err == nil {
				pcm, _ = io.ReadAll(st)
			}
		}
		s.cache[path] = pcm
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
