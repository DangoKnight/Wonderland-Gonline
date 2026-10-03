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
	Root  string
	once  sync.Once
	ctx   *audio.Context
	cache map[string][]byte
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
	s.Context()
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
	if len(pcm) > 0 {
		s.ctx.NewPlayerFromBytes(pcm).Play()
	}
}
