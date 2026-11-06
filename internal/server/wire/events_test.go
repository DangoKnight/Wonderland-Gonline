package wire

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestEventSpeech24BitGolden(t *testing.T) {
	got := SpeechFrame(3, 0x1234, 0x11234, 7)
	want := []byte{20, 1, 0, 0, 0, 7, 1, 3, 0x34, 0x12, 1, 0, 0, 0, 0, 0x34, 0x12, 1}
	if !bytes.Equal(got, want) {
		t.Fatal(got)
	}
	var raw assets.Operation
	copy(raw.Data[:], []byte{2, 1, 0, 0, 0, 0x34, 0x12, 1, 0})
	op := world.DecodeOp(raw)
	if !op.NPCSpeech() || op.TalkID() != 0x11234 {
		t.Fatal(op)
	}
	if p := EventFrame(1, 3, 1, 1, 0, 10001, 1, 9); p[len(p)-1] != 0 {
		t.Fatal("branch corrupted speech reference", p)
	}
}

func TestBoundXaolanSpeechFacingGolden(t *testing.T) {
	// Native North Island event 46: actor 62, talk 30062, dword2 = 2.
	raw := assets.Operation{Index: 1, Data: [21]byte{2, 62, 0, 1, 0, 110, 117, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0}}
	op := world.DecodeOp(raw)
	got := SpeechFrameWithMode(3, op.D1, op.TalkID(), op.Index, byte(op.DW2))
	want := []byte{20, 1, 0, 0, 0, 1, 1, 3, 62, 0, 2, 0, 0, 0, 0, 110, 117, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("non-turning speech = %v, want %v", got, want)
	}
}
