package debuglog

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

// Independent raw wire values keep this test from merely mirroring the switch.
func TestCredentialFamiliesNeverWritePayloads(t *testing.T) {
	for _, command := range []byte{9, 35, 45, 63} {
		for _, direction := range []byte{In, Out} {
			t.Run(fmt.Sprintf("%d/%c", command, direction), func(t *testing.T) {
				l := open(t, t.TempDir(), DefaultLimit)
				secret := []byte("fakeSecret123")
				packet := append([]byte{command, 4}, secret...)
				original := append([]byte(nil), packet...)
				l.Packet(direction, packet)
				l.Packet(direction, []byte{command}) // malformed/truncated variant
				if err := l.Close(); err != nil {
					t.Fatal(err)
				}
				lines := readLines(t, l.Path())
				if len(lines) != 2 {
					t.Fatalf("got %d lines", len(lines))
				}
				for _, line := range lines {
					if !strings.Contains(line, "[credential payload redacted]") || strings.Contains(line, string(secret)) || strings.Contains(line, hex.EncodeToString(secret)) {
						t.Fatalf("unsafe transcript line %q", line)
					}
				}
				if string(packet) != string(original) {
					t.Fatal("logging mutated the transmitted packet")
				}
			})
		}
	}
}
