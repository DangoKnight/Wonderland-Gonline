package protocol

import (
	"bytes"
	"testing"
)

func TestConnectionCapabilityGoldens(t *testing.T) {
	for _, sub := range []byte{248, 249, 250} {
		want := []byte{1, sub, 71, 76, 79, 78, 1, 0, 7, 0, 0, 0}
		if !bytes.Equal(ConnectionCapabilities(sub, ConnectionKnownFeatures), want) {
			t.Fatal("connection wire changed", sub)
		}
		if got, ok := ParseConnectionCapabilities(want, sub); !ok || got != 7 {
			t.Fatal(got, ok)
		}
		for i := range len(want) {
			if _, ok := ParseConnectionCapabilities(want[:i], sub); ok {
				t.Fatal("accepted truncated hello", i)
			}
		}
		for _, bad := range [][]byte{{1, sub, 1}, {1, sub, 71, 76, 79, 78, 2, 0, 7, 0, 0, 0}, {1, sub, 71, 76, 66, 82, 1, 0, 7, 0, 0, 0}, append(append([]byte(nil), want...), 0)} {
			if _, ok := ParseConnectionCapabilities(bad, sub); ok {
				t.Fatal("accepted incompatible hello", bad)
			}
		}
	}
}
