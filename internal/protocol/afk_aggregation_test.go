package protocol

import (
	"bytes"
	"testing"
)

func TestAFKAggregationGolden(t *testing.T) {
	binding := AFKStreamBinding{Stream: 1, Carrier: 2, Members: []uint32{2, 3}}
	want := []byte{253, 6, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 2, 2, 0, 0, 0, 3, 0, 0, 0}
	p, err := EncodeAFKBinding(binding)
	if err != nil || !bytes.Equal(p, want) {
		t.Fatal(p, err)
	}
	if b, err := DecodeAFKBinding(want); err != nil || b.Stream != 1 || b.Carrier != 2 || len(b.Members) != 2 {
		t.Fatal(b, err)
	}
	for i := range want {
		if _, err := DecodeAFKBinding(want[:i]); err == nil {
			t.Fatal("truncation accepted", i)
		}
	}
	for _, b := range []AFKStreamBinding{{Stream: 1, Carrier: 2, Members: []uint32{2, 2}}, {Stream: 1, Carrier: 4, Members: []uint32{2, 3}}, {Stream: 1, Carrier: 2, Members: []uint32{2}}, {Carrier: 2}} {
		if _, err := EncodeAFKBinding(b); err == nil {
			t.Fatal("invalid binding accepted", b)
		}
	}
	shared := []byte{253, 7, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0, 253, 2, 0, 0, 0, 0, 0, 0, 0}
	p, err = EncodeAFKShared(AFKSharedState{Stream: 1, Sequence: 2})
	if err != nil || !bytes.Equal(p, shared) {
		t.Fatal(p, err)
	}
	if got, err := DecodeAFKShared(shared); err != nil || got.Stream != 1 || got.Sequence != 2 {
		t.Fatal(got, err)
	}
	for i := range shared {
		if _, err := DecodeAFKShared(shared[:i]); err == nil {
			t.Fatal("shared truncation accepted", i)
		}
	}
}
