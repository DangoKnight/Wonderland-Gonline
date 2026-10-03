package assets

import (
	"encoding/binary"
	"testing"
	"wonderland-go/internal/protocol"
)

func TestAnimationTimingNativePath(t *testing.T) {
	record := protocol.Builder(make([]byte, 25)).U8(0).U8(1).U8(0).U8(0).F64(.6).U32(100).U8(1).U32(60).U32(0).U8(0)
	data := append([]byte(nil), record...)
	data = protocol.Builder(data).U16(30074).U32(0).U32(uint32(len(record))).U16(1)
	timings, e := ParseAnimationTiming(data)
	if e != nil {
		t.Fatal(e)
	}
	if timings[30074] != 405 {
		t.Fatal(timings)
	}
	if AnimationDelay(timings, 30074, 1000, 100) != 6200 {
		t.Fatal("missing return/camera pacing budget")
	}
	if AnimationDelay(timings, 1, 1000, 100) != 1000 {
		t.Fatal("missing fallback")
	}
	broken := append([]byte(nil), data...)
	binary.LittleEndian.PutUint32(broken[len(record)+2:], 1)
	if _, e := ParseAnimationTiming(broken); e == nil {
		t.Fatal("invalid index accepted")
	}
}
func TestEVENameBig5(t *testing.T) {
	if got := eveName([]byte{0xa4, 0xa4, 0xa4, 0xe5, 0, 65}); got != "中文" {
		t.Fatalf("%q", got)
	}
}
