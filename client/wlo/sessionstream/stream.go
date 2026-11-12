// Package sessionstream carries deterministic client inputs, not rendered pixels.
// It has no rendering, game socket, credentials or API ownership implementation.
package sessionstream

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"time"
)

const (
	Version                     = 1
	FramesPerChunk              = 256
	ChunksPerBatch              = 8
	MaximumDecodedChunk         = 4 << 20
	MaximumJournalBytes         = 128 << 20
	MaximumEventBytes           = 1 << 20
	MaximumBatchCompressedBytes = 8 << 20
)

type Event struct {
	Kind      string          `json:"kind"`
	Code      int             `json:"code,omitempty"`
	Packet    []byte          `json:"packet,omitempty"`
	Command   string          `json:"command,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Path      []int           `json:"path,omitempty"`
}
type Frame struct {
	Delta  int64   `json:"dt"` // nanoseconds since the preceding frame
	Events []Event `json:"events,omitempty"`
}
type Chunk struct {
	First uint64 `json:"first"`
	Count int    `json:"count"`
	Data  []byte `json:"data"`
}
type Batch struct {
	Version   int       `json:"version"`
	Epoch     string    `json:"epoch"`
	Start     time.Time `json:"start"`
	Head      uint64    `json:"head"`
	Chunks    []Chunk   `json:"chunks,omitempty"`
	TailFirst uint64    `json:"tailFirst,omitempty"`
	Tail      []Frame   `json:"tail,omitempty"`
	Error     string    `json:"error,omitempty"`
}
type Journal struct {
	epoch       string
	start, last time.Time
	head        uint64
	chunks      []Chunk
	tail        []Frame
	bytes       int
	tailBytes   int
	err         string
}

func New(epoch string, start time.Time) *Journal {
	return &Journal{epoch: epoch, start: start, last: start}
}
func (j *Journal) Append(now time.Time, events []Event) {
	if j.err != "" {
		return
	}
	delta := now.Sub(j.last)
	if delta < 0 {
		delta = 0
	}
	j.last = now
	j.head++
	j.tail = append(j.tail, Frame{Delta: int64(delta), Events: events})
	raw, _ := json.Marshal(j.tail[len(j.tail)-1])
	j.tailBytes += len(raw)
	if len(raw) > MaximumEventBytes {
		j.err = "native presentation frame exceeds safety limit"
		return
	}
	if len(j.tail) < FramesPerChunk && j.tailBytes < MaximumDecodedChunk/2 {
		return
	}
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	if err := json.NewEncoder(compressed).Encode(j.tail); err != nil {
		j.err = err.Error()
		return
	}
	if err := compressed.Close(); err != nil {
		j.err = err.Error()
		return
	}
	if j.bytes+buffer.Len() > MaximumJournalBytes {
		j.err = "native spectator history limit reached; create a fresh hosted client session"
		return
	}
	j.bytes += buffer.Len()
	j.chunks = append(j.chunks, Chunk{First: j.head - uint64(len(j.tail)) + 1, Count: len(j.tail), Data: buffer.Bytes()})
	j.tail = nil
	j.tailBytes = 0
}
func (j *Journal) Batch(after uint64) Batch {
	out := Batch{Version: Version, Epoch: j.epoch, Start: j.start, Head: j.head, Error: j.err}
	if after > j.head {
		out.Error = "native spectator cursor is ahead of host"
		return out
	}
	compressedBytes := 0
	for _, chunk := range j.chunks {
		if chunk.First+uint64(chunk.Count) <= after+1 {
			continue
		}
		if compressedBytes+len(chunk.Data) > MaximumBatchCompressedBytes && len(out.Chunks) > 0 {
			return out
		}
		out.Chunks = append(out.Chunks, chunk)
		compressedBytes += len(chunk.Data)
		if len(out.Chunks) == ChunksPerBatch {
			return out
		}
	}
	out.TailFirst = j.head - uint64(len(j.tail)) + 1
	start := 0
	if after >= out.TailFirst {
		start = int(after - out.TailFirst + 1)
	}
	out.TailFirst += uint64(start)
	out.Tail = append([]Frame(nil), j.tail[start:]...)
	return out
}
func Decode(chunk Chunk) ([]Frame, error) {
	if chunk.Count < 1 || chunk.Count > FramesPerChunk {
		return nil, errors.New("invalid native frame chunk count")
	}
	reader, err := gzip.NewReader(bytes.NewReader(chunk.Data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, MaximumDecodedChunk+1))
	if err != nil || len(data) > MaximumDecodedChunk {
		return nil, errors.New("native frame chunk too large")
	}
	var frames []Frame
	if json.Unmarshal(data, &frames) != nil || len(frames) != chunk.Count {
		return nil, errors.New("invalid native frame chunk")
	}
	return frames, nil
}
func Delivered(batch Batch, previous uint64) uint64 {
	if len(batch.Tail) > 0 {
		return batch.TailFirst + uint64(len(batch.Tail)) - 1
	}
	if len(batch.Chunks) > 0 {
		last := batch.Chunks[len(batch.Chunks)-1]
		return last.First + uint64(last.Count) - 1
	}
	return previous
}
