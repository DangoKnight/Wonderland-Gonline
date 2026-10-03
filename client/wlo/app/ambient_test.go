package app

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestNumberedSound checks the wav#### paths of zones and props.
func TestNumberedSound(t *testing.T) {
	for n, want := range map[int]string{50: `Sound\wav0050.wav`, 9900: `Sound\wav9900.wav`, 7: `Sound\wav0007.wav`} {
		if got := numberedSound(n); got != want {
			t.Errorf("%d: %q, want %q", n, got, want)
		}
	}
}

// TestPanLoop: the loop wraps and scales each channel; DirectSound's -600
// hundredths of a dB is half the amplitude.
func TestPanLoop(t *testing.T) {
	pcm := make([]byte, 8)
	for i := 0; i < 4; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(1000)))
	}
	s := &panLoop{pcm: pcm, left: 1, right: fromHundredths(600)}
	out := make([]byte, 16)
	if n, _ := s.Read(out); n != 16 {
		t.Fatalf("read %d", n)
	}
	for i := 0; i < 4; i++ {
		l := int16(binary.LittleEndian.Uint16(out[i*4:]))
		r := int16(binary.LittleEndian.Uint16(out[i*4+2:]))
		if l != 1000 || math.Abs(float64(r)-501) > 2 {
			t.Fatalf("frame %d: %d %d", i, l, r)
		}
	}
}
