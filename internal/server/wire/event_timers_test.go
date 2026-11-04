package wire

import (
	"bytes"
	"testing"
	"time"
)

func TestCapturedCollectionTimer(t *testing.T) {
	// Independent Legacy server capture: North Island actor 33, timer 11020.
	want := []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64}
	if got := EventTimerUpdate(11020, 180*time.Second); !bytes.Equal(got, want) {
		t.Fatalf("timer payload %v, want %v", got, want)
	}
}
