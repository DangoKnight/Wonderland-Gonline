package server

import (
	"bytes"
	"testing"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/world"
)

func TestEventSpeech24BitGolden(t *testing.T) {
	got := speechFrame(3, 0x1234, 0x11234, 7)
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
	if p := eventFrame(1, 3, 1, 1, 0, 10001, 1, 9); p[len(p)-1] != 0 {
		t.Fatal("branch corrupted speech reference", p)
	}
}
