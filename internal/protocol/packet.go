// Package protocol implements the Wonderland TCP wire format.
// Reference: RCLibrary.Core.Networking/{IncomingPacket,OutgoingPacket}.cs.
package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const Signature uint16 = 0x44f4
const XOR byte = 0xad
const MaxPayload = math.MaxUint16
const FrameHeaderBytes = 4
const MaxStringBytes = math.MaxUint8

var ErrMalformed = errors.New("malformed Wonderland packet")

// Read handles both fragmented frames and multiple frames in one TCP read.
func Read(r io.Reader) ([]byte, error) {
	var h [FrameHeaderBytes]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	for i := range h {
		h[i] ^= XOR
	}
	if binary.LittleEndian.Uint16(h[:2]) != Signature {
		return nil, fmt.Errorf("%w: signature", ErrMalformed)
	}
	n := int(binary.LittleEndian.Uint16(h[2:]))
	if n == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrMalformed)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	for i := range b {
		b[i] ^= XOR
	}
	return b, nil
}

func Encode(payload []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload) > MaxPayload {
		return nil, fmt.Errorf("%w: payload size %d", ErrMalformed, len(payload))
	}
	b := make([]byte, FrameHeaderBytes+len(payload))
	binary.LittleEndian.PutUint16(b, Signature)
	binary.LittleEndian.PutUint16(b[2:], uint16(len(payload)))
	copy(b[FrameHeaderBytes:], payload)
	for i := range b {
		b[i] ^= XOR
	}
	return b, nil
}

func Write(w io.Writer, payload []byte) error {
	b, err := Encode(payload)
	if err != nil {
		return err
	}
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}

// Reader retains the first error, making multi-field decoders fail atomically.
type Reader struct {
	data []byte
	pos  int
	err  error
}

func NewReader(b []byte) *Reader { return &Reader{data: b} }
func (r *Reader) Err() error     { return r.err }
func (r *Reader) Remaining() int { return len(r.data) - r.pos }
func (r *Reader) Bytes(n int) []byte {
	if r.err != nil {
		return nil
	}
	if n < 0 || n > r.Remaining() {
		r.err = io.ErrUnexpectedEOF
		return nil
	}
	b := r.data[r.pos : r.pos+n]
	r.pos += n
	return b
}
func (r *Reader) U8() byte {
	b := r.Bytes(1)
	if b == nil {
		return 0
	}
	return b[0]
}
func (r *Reader) U16() uint16 {
	b := r.Bytes(2)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(b)
}
func (r *Reader) U32() uint32 {
	b := r.Bytes(4)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(b)
}
func (r *Reader) U64() uint64 {
	b := r.Bytes(8)
	if b == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(b)
}
func (r *Reader) F64() float64   { return math.Float64frombits(r.U64()) }
func (r *Reader) String() string { return string(r.Bytes(int(r.U8()))) }
func (r *Reader) Rest() []byte   { return r.Bytes(r.Remaining()) }

type Builder []byte

func (b Builder) U8(v byte) Builder      { return append(b, v) }
func (b Builder) U16(v uint16) Builder   { return binary.LittleEndian.AppendUint16(b, v) }
func (b Builder) U32(v uint32) Builder   { return binary.LittleEndian.AppendUint32(b, v) }
func (b Builder) U64(v uint64) Builder   { return binary.LittleEndian.AppendUint64(b, v) }
func (b Builder) F64(v float64) Builder  { return b.U64(math.Float64bits(v)) }
func (b Builder) Bytes(v []byte) Builder { return append(b, v...) }

// String returns an error rather than silently truncating a byte-length prefix.
func (b Builder) String(v string) (Builder, error) {
	if len(v) > MaxStringBytes {
		return nil, ErrMalformed
	}
	return b.U8(byte(len(v))).Bytes([]byte(v)), nil
}
