package app

import (
	"bytes"
	"strings"
	"sync"
	"wonderland-gonline/internal/clientfs"

	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"wonderland-gonline/client/wlo/login"
)

// Background music. At login FormCreate's timer (CheckStartMusic,
// FUN_004a3c98) plays Sound\BGM0013.wav through a media player and rewinds
// it whenever it reaches the end. In game, entering a map plays its scene's
// track (0x3bce95 → FUN_004048b8), which keeps a track that is already
// playing. The original also remembers where each map's track stopped
// (+0x5706, +0x5710) and plays BGM0028 on some maps under a player flag
// (+0x5768); those two behaviors are not ported. Local on/off and volume
// preferences are applied through settings.go.
const (
	loginMusic = `Sound\BGM0013.wav`
	musicDir   = `Sound\`
	musicExt   = ".wav"
)

// Music plays one looping track at a time.
type Music struct {
	Root    string
	Context func() *audio.Context

	mu         sync.Mutex
	current    string
	player     *audio.Player
	volume     float64
	configured bool
}

// Play loops the track at path (relative to the client directory, with
// backslashes); the same track keeps playing, a missing one stops the music.
func (m *Music) Play(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.EqualFold(path, m.current) && m.player != nil {
		return
	}
	m.stop()
	m.current = path
	raw, err := readTrack(m, path)
	if err != nil {
		return
	}
	ctx := m.Context()
	s, err := wav.DecodeWithSampleRate(ctx.SampleRate(), bytes.NewReader(raw))
	if err != nil {
		return
	}
	loop := audio.NewInfiniteLoop(s, s.Length())
	p, err := ctx.NewPlayer(loop)
	if err != nil {
		return
	}
	m.player = p
	if m.configured {
		p.SetVolume(m.volume)
	}
	p.Play()
}

// readTrack reads a track's file.
func readTrack(m *Music, path string) ([]byte, error) {
	return clientfs.ReadFile(login.Path(m.Root, strings.Split(path, `\`)...))
}

// Stop ends the music.
func (m *Music) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stop()
	m.current = ""
}

func (m *Music) stop() {
	if m.player != nil {
		m.player.Close()
		m.player = nil
	}
}

// playMapMusic plays the current map's scene track.
func (c *Client) playMapMusic() {
	if c.Music == nil || c.World == nil {
		return
	}
	if c.sceneMusic == nil {
		c.resources.loadScenes(c.Assets)
		c.sceneMusic = c.resources.sceneMusic
	}
	if track := c.sceneMusic[c.mapScenes[c.World.Player.Map]]; track != "" {
		c.Music.Play(musicDir + track + musicExt)
	}
}

// SetVolume applies local music preferences without restarting the current track.
func (m *Music) SetVolume(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.volume = max(0, min(1, v))
	m.configured = true
	if m.player != nil {
		m.player.SetVolume(m.volume)
	}
}
