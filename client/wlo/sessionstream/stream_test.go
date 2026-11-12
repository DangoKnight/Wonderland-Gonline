package sessionstream

import (
	"bytes"
	"compress/gzip"
	"testing"
	"time"
)

func TestJournalCatchupAndResume(t *testing.T) {
	start := time.Unix(100, 0)
	j := New("session", start)
	const total = FramesPerChunk*(ChunksPerBatch+1) + 3
	for i := 1; i <= total; i++ {
		j.Append(start.Add(time.Duration(i)*time.Millisecond), []Event{{Kind: "network", Packet: []byte{byte(i)}}})
	}
	cursor := uint64(0)
	seen := 0
	for cursor < total {
		batch := j.Batch(cursor)
		if batch.Error != "" {
			t.Fatal(batch.Error)
		}
		visit := func(first uint64, frames []Frame) {
			for i, f := range frames {
				seq := first + uint64(i)
				if seq <= cursor {
					continue
				}
				seen++
				if f.Delta != int64(time.Millisecond) || f.Events[0].Packet[0] != byte(seq) {
					t.Fatal("corrupt frame", seq)
				}
			}
		}
		for _, chunk := range batch.Chunks {
			frames, err := Decode(chunk)
			if err != nil {
				t.Fatal(err)
			}
			visit(chunk.First, frames)
		}
		visit(batch.TailFirst, batch.Tail)
		next := Delivered(batch, cursor)
		if next <= cursor {
			t.Fatal("cursor stalled")
		}
		cursor = next
	}
	if seen != total {
		t.Fatalf("got %d frames", seen)
	}
	batch := j.Batch(17)
	if batch.Chunks[0].First != 1 {
		t.Fatal("resume must retain overlapping chunk")
	}
	if j.Batch(total+1).Error == "" {
		t.Fatal("accepted cursor beyond host")
	}
}

func TestDecodeRejectsInvalidOrExcessiveChunks(t *testing.T) {
	var b bytes.Buffer
	z := gzip.NewWriter(&b)
	_, _ = z.Write(bytes.Repeat([]byte(" "), MaximumDecodedChunk+1))
	_ = z.Close()
	for _, chunk := range []Chunk{{Count: 0}, {Count: FramesPerChunk + 1}, {Count: 1, Data: []byte("bad")}, {Count: 1, Data: b.Bytes()}} {
		if _, err := Decode(chunk); err == nil {
			t.Fatal("accepted invalid chunk")
		}
	}
}
