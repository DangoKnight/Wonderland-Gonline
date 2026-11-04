package app

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"wonderland-gonline/internal/clientfs"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"wonderland-gonline/client/wlo/login"
)

// sampleRate is the mixing rate; wave files are resampled to it.
const sampleRate = 44100
const voicePendingLimit = 1 // Keep only the newest line behind the active decoder.

// Sounds plays wave effects and extracted OGG voices (FUN_00404fa4),
// given paths relative
// to the client directory with backslashes.
type Sounds struct {
	Root            string
	Shared          *SoundCache
	mu              sync.Mutex
	volume          float64
	configured      bool
	voice           *audio.Player
	voiceRevision   uint64
	voiceJobs       chan voiceRequest
	voiceDone       chan struct{}
	voiceWorkerDone chan struct{}
	voiceClosed     bool
	decodePCM       func(string) []byte // Optional decoder for deterministic lifecycle tests.
}

type voiceRequest struct {
	path     string
	revision uint64
}

// SoundCache owns a workspace's decoded effects. Ebitengine permits only one
// audio context per process, shared by all workspaces and standalone Sounds.
type SoundCache struct {
	once  sync.Once
	ctx   *audio.Context
	mu    sync.Mutex
	cache map[string][]byte
}

var audioContextMu sync.Mutex

func (s *Sounds) Context() *audio.Context {
	s.mu.Lock()
	if s.Shared == nil {
		s.Shared = &SoundCache{}
	}
	shared := s.Shared
	s.mu.Unlock()
	shared.once.Do(func() {
		audioContextMu.Lock()
		defer audioContextMu.Unlock()
		shared.ctx = audio.CurrentContext()
		if shared.ctx == nil {
			shared.ctx = audio.NewContext(sampleRate)
		}
		shared.cache = map[string][]byte{}
	})
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

// PCM is a wave/OGG sound's decoded 16-bit stereo samples at the mixing rate, read
// once; nil for a missing or unreadable file.
func (s *Sounds) PCM(path string) []byte {
	s.Context()
	s.Shared.mu.Lock()
	key := login.Path(s.Root, strings.Split(path, `\`)...)
	pcm, ok := s.Shared.cache[key]
	s.Shared.mu.Unlock()
	if ok {
		return pcm
	}
	// Disk access and decoding must not hold the shared cache lock: a voice
	// decoding in the background must not stall cached effects on the UI thread.
	if s.decodePCM != nil {
		pcm = s.decodePCM(path)
	} else {
		pcm = decodeAudioPCM(key)
	}
	s.Shared.mu.Lock()
	if existing, loaded := s.Shared.cache[key]; loaded {
		pcm = existing
	} else {
		s.Shared.cache[key] = pcm
	}
	s.Shared.mu.Unlock()
	return pcm
}

func decodeAudioPCM(path string) []byte {
	raw, err := clientfs.ReadFile(path)
	if err != nil {
		return nil
	}
	var decoded io.Reader
	if strings.EqualFold(filepath.Ext(path), ".ogg") {
		decoded, err = vorbis.DecodeWithSampleRate(sampleRate, bytes.NewReader(raw))
	} else {
		decoded, err = wav.DecodeWithSampleRate(sampleRate, bytes.NewReader(raw))
	}
	if err != nil {
		return nil
	}
	pcm, _ := io.ReadAll(decoded)
	return pcm
}

// SetVolume controls subsequent sound effects; ambient loops are updated by Client.
func (s *Sounds) SetVolume(v float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.volume = max(0, min(1, v))
	s.configured = true
}

// PlayVoice replaces the preceding recording without decoding on the UI thread.
// One worker and a latest-only queue keep fast dialogue advancement bounded.
func (s *Sounds) PlayVoice(path string) {
	s.Context()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.voiceClosed {
		return
	}
	s.voiceRevision++
	if s.voice != nil {
		s.voice.Close()
		s.voice = nil
	}
	if s.configured && s.volume == 0 {
		return
	}
	if s.voiceJobs == nil {
		s.voiceJobs, s.voiceDone = make(chan voiceRequest, voicePendingLimit), make(chan struct{})
		s.voiceWorkerDone = make(chan struct{})
		go s.runVoices(s.voiceJobs, s.voiceDone)
	}
	request := voiceRequest{path, s.voiceRevision}
	select {
	case s.voiceJobs <- request:
	default:
		select {
		case <-s.voiceJobs:
		default:
		}
		s.voiceJobs <- request
	}
}

func (s *Sounds) runVoices(jobs <-chan voiceRequest, done <-chan struct{}) {
	defer close(s.voiceWorkerDone)
	for {
		select {
		case <-done:
			return
		case request := <-jobs:
			s.mu.Lock()
			valid := !s.voiceClosed && request.revision == s.voiceRevision
			s.mu.Unlock()
			if !valid {
				continue
			}
			pcm := s.PCM(request.path)
			s.mu.Lock()
			if !s.voiceClosed && request.revision == s.voiceRevision && len(pcm) > 0 {
				volume := 1.0
				if s.configured {
					volume = s.volume
				}
				if volume > 0 {
					s.voice = s.Shared.ctx.NewPlayerFromBytes(pcm)
					s.voice.SetVolume(volume)
					s.voice.Play()
				}
			}
			s.mu.Unlock()
		}
	}
}

func (s *Sounds) StopVoice() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.voiceRevision++ // Also cancel queued/in-flight recordings.
	if s.voice != nil {
		s.voice.Close()
		s.voice = nil
	}
}

// Close stops this session's worker without closing the shared audio context.
func (s *Sounds) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.voiceClosed {
		return
	}
	s.voiceClosed = true
	s.voiceRevision++
	if s.voiceDone != nil {
		close(s.voiceDone)
	}
	if s.voice != nil {
		s.voice.Close()
		s.voice = nil
	}
}
