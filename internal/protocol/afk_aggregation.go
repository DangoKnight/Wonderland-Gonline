package protocol

const (
	AFKStreamBind               = 6
	AFKSharedSnapshot           = 7
	AFKGroupTokenBytes          = 16
	AFKModePacketBytes          = 3
	AFKGroupedModePacketBytes   = AFKModePacketBytes + AFKGroupTokenBytes
	AFKStreamBindingHeaderBytes = 15
	AFKSharedHeaderBytes        = 18
)

// Binding is sent on every authenticated subscriber's own socket. A workspace
// may fan out a shared snapshot only after that individual authorization.
type AFKStreamBinding struct {
	Stream  uint64
	Carrier uint32
	Members []uint32
}

func EncodeAFKBinding(b AFKStreamBinding) ([]byte, error) {
	if !validAFKBinding(b) {
		return nil, ErrMalformed
	}
	p := Builder{CommandAFK, AFKStreamBind}.U64(b.Stream).U32(b.Carrier).U8(byte(len(b.Members)))
	for _, id := range b.Members {
		p = p.U32(id)
	}
	return p, nil
}
func DecodeAFKBinding(p []byte) (AFKStreamBinding, error) {
	if len(p) < AFKStreamBindingHeaderBytes || p[0] != CommandAFK || p[1] != AFKStreamBind {
		return AFKStreamBinding{}, ErrMalformed
	}
	r := NewReader(p[2:])
	b := AFKStreamBinding{Stream: r.U64(), Carrier: r.U32()}
	n := int(r.U8())
	if n > TeamMaximumMembers || r.Remaining() != n*4 {
		return AFKStreamBinding{}, ErrMalformed
	}
	for range n {
		b.Members = append(b.Members, r.U32())
	}
	if r.Err() != nil || !validAFKBinding(b) {
		return AFKStreamBinding{}, ErrMalformed
	}
	return b, nil
}
func validAFKBinding(b AFKStreamBinding) bool {
	if b.Stream == 0 {
		return b.Carrier == 0 && len(b.Members) == 0
	}
	if b.Carrier == 0 || len(b.Members) < 2 || len(b.Members) > TeamMaximumMembers {
		return false
	}
	carrier := false
	for i, id := range b.Members {
		if id == 0 {
			return false
		}
		carrier = carrier || id == b.Carrier
		for _, old := range b.Members[:i] {
			if old == id {
				return false
			}
		}
	}
	return carrier
}

type AFKSharedState struct {
	Stream, Sequence uint64
	State            AFKState
}

func EncodeAFKShared(s AFKSharedState) ([]byte, error) {
	if s.Stream == 0 || s.Sequence == 0 {
		return nil, ErrMalformed
	}
	p, err := EncodeAFKState(s.State)
	if err != nil {
		return nil, err
	}
	return EncodeAFKSharedUpdate(s.Stream, s.Sequence, p)
}

// The inner packet may be a full snapshot or, with bit 7 confirmed, a delta.
func EncodeAFKSharedUpdate(stream, sequence uint64, p []byte) ([]byte, error) {
	if stream == 0 || sequence == 0 || len(p) < 2 || p[0] != CommandAFK || (p[1] != AFKSnapshot && p[1] != AFKDelta) {
		return nil, ErrMalformed
	}
	return Builder{CommandAFK, AFKSharedSnapshot}.U64(stream).U64(sequence).Bytes(p), nil
}

func DecodeAFKSharedUpdate(p []byte) (stream, sequence uint64, update []byte, err error) {
	if len(p) < AFKSharedHeaderBytes+2 || p[0] != CommandAFK || p[1] != AFKSharedSnapshot {
		err = ErrMalformed
		return
	}
	r := NewReader(p[2:])
	stream, sequence = r.U64(), r.U64()
	update = r.Rest()
	if stream == 0 || sequence == 0 || update[0] != CommandAFK || (update[1] != AFKSnapshot && update[1] != AFKDelta) {
		err = ErrMalformed
	}
	return
}
func DecodeAFKShared(p []byte) (AFKSharedState, error) {
	if len(p) < AFKSharedHeaderBytes || p[0] != CommandAFK || p[1] != AFKSharedSnapshot {
		return AFKSharedState{}, ErrMalformed
	}
	r := NewReader(p[2:])
	s := AFKSharedState{Stream: r.U64(), Sequence: r.U64()}
	var err error
	s.State, err = DecodeAFKState(r.Rest())
	if err != nil || s.Stream == 0 || s.Sequence == 0 {
		return AFKSharedState{}, ErrMalformed
	}
	return s, nil
}
