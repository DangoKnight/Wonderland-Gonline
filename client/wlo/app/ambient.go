package app

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"

	"wonderland-gonline/client/wlo/world"
)

// Ambient sounds: waves along a shore, birds in the trees. Every 500 ms
// the frame (FUN_00498aec, while no movie runs) looks up the scene's sound
// zone under the player (world.SoundZoneAt) and plays its wav####.wav
// around the zone's centre (FUN_004054f0) on one of six looping channels
// (2..7 of the sound bank): the sound starts while the player is within
// the zone's radius of the centre, measured vertically; its volume drops 3
// hundredths of a dB per pixel (DirectSound volume) and its pan follows the
// centre's horizontal offset (×10). A channel stops once the player is out
// of its range (FUN_004057c8). A map load or a movie stops them all
// (FUN_004057c8(-1, -1)).
//
// Sound preferences apply through settings.go. Not ported: FUN_004057c8's
// early return, which leaves later channels playing when an
// earlier one is in range.
const (
	ambientInterval   = 500 * time.Millisecond
	ambientChannels   = 6
	ambientFalloff    = 3  // hundredths of a dB per pixel
	ambientPanPerPx   = 10 // hundredths of a dB per pixel of offset
	ambientPanLimit   = 10000
	ambientSoundPad   = 4
	ambientSoundStart = "wav"
	hundredthsPerBel  = 2000 // linear = 10^(-hundredths/2000)
)

// ambientChannel is one playing zone sound.
type ambientChannel struct {
	srcX, srcY, rng int
	player          *audio.Player
	stream          *panLoop
}

// ambience is the client's ambient channels.
type ambience struct {
	at time.Time
	ch [ambientChannels]*ambientChannel
}

// ambientTick is FUN_00498aec's sound part.
func (c *Client) ambientTick() {
	if c.World == nil || c.World.Scene == nil || c.sfx == nil {
		return
	}
	if c.movie != nil || c.effectsGain() == 0 {
		c.stopAmbience()
		return
	}
	now := c.Now()
	a := &c.ambient
	if now.Sub(a.at) < ambientInterval {
		return
	}
	a.at = now
	px, py := c.World.Player.X, c.World.Player.Y
	// FUN_004057c8: channels out of range stop.
	for i, ch := range a.ch {
		if ch != nil && abs(py-ch.srcY) >= ch.rng {
			ch.player.Close()
			a.ch[i] = nil
		}
	}
	z, ok := c.World.Scene.SoundZoneAt(px, py)
	if !ok {
		return
	}
	c.playZone(z, px, py)
}

// playZone is FUN_004054f0 for the zone's sound.
func (c *Client) playZone(z world.SoundZone, px, py int) {
	a := &c.ambient
	srcX, srcY, rng := z.X*cellSizePixels, z.Y*cellSizePixels, z.Radius*cellSizePixels
	dist := abs(py - srcY)
	for i, ch := range a.ch {
		if ch == nil || ch.srcX != srcX || ch.srcY != srcY {
			continue
		}
		if dist < rng {
			ch.set(srcX-px, dist)
			ch.player.SetVolume(fromHundredths(ambientFalloff*dist) * c.effectsGain())
		} else {
			ch.player.Close()
			a.ch[i] = nil
		}
		return
	}
	if dist >= rng {
		return
	}
	for i, ch := range a.ch {
		if ch != nil {
			continue
		}
		pcm := c.sfx.PCM(numberedSound(z.Sound))
		if len(pcm) == 0 {
			return
		}
		stream := &panLoop{pcm: pcm, left: 1, right: 1}
		p, err := c.sfx.Context().NewPlayer(stream)
		if err != nil {
			return
		}
		ch = &ambientChannel{srcX: srcX, srcY: srcY, rng: rng, player: p, stream: stream}
		ch.set(srcX-px, dist)
		ch.player.SetVolume(fromHundredths(ambientFalloff*dist) * c.effectsGain())
		p.Play()
		a.ch[i] = ch
		return
	}
}

// numberedSound is sound\wav#### (the number padded to four digits), the
// path of a zone's and of an opening prop's sound.
func numberedSound(n int) string {
	s := strconv.Itoa(n)
	if len(s) < ambientSoundPad {
		s = strings.Repeat("0", ambientSoundPad-len(s)) + s
	}
	return musicDir + ambientSoundStart + s + ".wav"
}

// set applies the channel's DirectSound volume and pan.
func (ch *ambientChannel) set(dx, dist int) {
	ch.player.SetVolume(fromHundredths(ambientFalloff * dist))
	pan := max(-ambientPanLimit, min(ambientPanLimit, dx*ambientPanPerPx))
	left, right := 1.0, 1.0
	if pan > 0 {
		left = fromHundredths(pan)
	} else if pan < 0 {
		right = fromHundredths(-pan)
	}
	ch.stream.setGains(left, right)
}

// fromHundredths turns an attenuation in hundredths of a dB into a gain.
func fromHundredths(h int) float64 { return math.Pow(10, -float64(h)/hundredthsPerBel) }

// stopAmbience stops every ambient channel.
func (c *Client) stopAmbience() {
	for i, ch := range c.ambient.ch {
		if ch != nil {
			ch.player.Close()
			c.ambient.ch[i] = nil
		}
	}
}

// cellSizePixels is the walk grid's cell (20 pixels).
const cellSizePixels = 0x14

// panLoop loops 16-bit stereo samples with a gain per channel.
type panLoop struct {
	pcm         []byte
	pos         int
	mu          sync.Mutex
	left, right float64
}

func (s *panLoop) setGains(l, r float64) {
	s.mu.Lock()
	s.left, s.right = l, r
	s.mu.Unlock()
}

const stereoFrame = 4 // bytes per 16-bit stereo sample pair

func (s *panLoop) Read(b []byte) (int, error) {
	s.mu.Lock()
	l, r := s.left, s.right
	s.mu.Unlock()
	n := len(b) - len(b)%stereoFrame
	usable := len(s.pcm) - len(s.pcm)%stereoFrame
	for i := 0; i < n; i += stereoFrame {
		if s.pos >= usable {
			s.pos = 0
		}
		f := s.pcm[s.pos:]
		lv := int16(binary.LittleEndian.Uint16(f))
		rv := int16(binary.LittleEndian.Uint16(f[2:]))
		binary.LittleEndian.PutUint16(b[i:], uint16(int16(float64(lv)*l)))
		binary.LittleEndian.PutUint16(b[i+2:], uint16(int16(float64(rv)*r)))
		s.pos += stereoFrame
	}
	return n, nil
}
