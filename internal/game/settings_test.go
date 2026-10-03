package game

import (
	"bytes"
	"testing"
)

func TestSettingsNativeSnapshotGolden(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings ClientSettings
		want     []byte
	}{
		{"all on", ClientSettings{true, true, true, 31}, []byte{33, 2, 1, 1, 1, 1, 31, 0}},
		{"all off", ClientSettings{}, []byte{33, 2, 2, 2, 2, 1, 0, 0}},
		{"local only", ClientSettings{Channels: ChatChannelLocal}, []byte{33, 2, 2, 2, 2, 1, 1, 0}},
		{"whisper and world", ClientSettings{Channels: ChatChannelWhisper | ChatChannelWorld}, []byte{33, 2, 2, 2, 2, 1, 18, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.settings.Packet(); !bytes.Equal(got, test.want) {
				t.Fatalf("native snapshot: %x != %x", got, test.want)
			}
		})
	}
}

func TestSettingsSnapshotEnablesNativeLocalChat(t *testing.T) {
	// aLogin case 0x21/sub2 uses local_c[5] after stripping AC33:
	// payload byte 6 is decoded by FUN_002823ac with 1-based bit numbers.
	// The old six-byte packet never supplied that byte.
	for bit := byte(0); bit < 5; bit++ {
		settings := ClientSettings{Channels: 1 << bit}
		packet := settings.Packet()
		if len(packet) != 8 {
			t.Fatalf("incomplete native snapshot: %x", packet)
		}
		mask := packet[6]
		for channel := byte(0); channel < 5; channel++ {
			enabled := (mask>>channel)&1 != 0
			if enabled != (channel == bit) {
				t.Fatalf("channel %d decoded incorrectly: %x", channel, packet)
			}
		}
	}
}
