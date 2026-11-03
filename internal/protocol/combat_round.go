package protocol

import (
	"bytes"
	"time"
)

// These are versioned Gonline extensions, not native action codes. Native
// AC1 dispatch ignores unknown subcommands (FUN_002dde1c, case 1).
const (
	CombatProtocolVersion              = 1
	CombatCapabilityRoundChain  uint32 = 1 << 0
	CombatRoundChunk                   = 1
	CombatRoundChoices                 = 2
	CombatRoundPrompt                  = 3
	CombatCapabilityPacketBytes        = 12
	CombatChunkHeaderBytes             = 14
	CombatPromptBytes                  = 8
	CombatChoiceBytes                  = 8 // Attack/defend/flee; AC50:2 has BattleItemRequestBytes.
	CombatChoicesHeaderBytes           = 7
	CombatMaxRoundBytes                = 1 << 20
	CombatMaxRoundSteps                = 512
	CombatMaxRoundPackets              = 4096
	CombatMaxStepDelay                 = time.Minute
	CombatMaxRoundDelay                = 30 * time.Minute
	CombatMaxChoices                   = 2 // One character and its active battle pet.
)

const combatSignature = "GLBR"

func CombatCapabilities(sub byte, flags uint32) []byte {
	return Builder{CommandHandshake, sub}.Bytes([]byte(combatSignature)).U16(CombatProtocolVersion).U32(flags)
}

func ParseCombatCapabilities(p []byte, sub byte) (uint32, bool) {
	if len(p) != CombatCapabilityPacketBytes || p[0] != CommandHandshake || p[1] != sub || !bytes.Equal(p[2:6], []byte(combatSignature)) {
		return 0, false
	}
	r := NewReader(p[6:])
	version, flags := r.U16(), r.U32()
	return flags, version == CombatProtocolVersion
}

// CombatStep preserves native packets and their ordering. All multi-byte fields
// are little endian. Delay is a minimum presentation interval after this step.
type CombatStep struct {
	Delay   time.Duration
	Packets [][]byte
}

func EncodeCombatRound(steps []CombatStep) ([]byte, error) {
	if len(steps) == 0 || len(steps) > CombatMaxRoundSteps {
		return nil, ErrMalformed
	}
	p := Builder{}.U16(uint16(len(steps)))
	var total time.Duration
	packets := 0
	for _, step := range steps {
		total += step.Delay
		packets += len(step.Packets)
		if step.Delay < 0 || step.Delay > CombatMaxStepDelay || step.Delay%time.Millisecond != 0 || total > CombatMaxRoundDelay || packets > CombatMaxRoundPackets {
			return nil, ErrMalformed
		}
		p = p.U32(uint32(step.Delay / time.Millisecond)).U16(uint16(len(step.Packets)))
		for _, packet := range step.Packets {
			if len(packet) == 0 || len(packet) > MaxPayload || packet[0] == CommandGonlineCombat {
				return nil, ErrMalformed
			}
			p = p.U16(uint16(len(packet))).Bytes(packet)
			if len(p) > CombatMaxRoundBytes {
				return nil, ErrMalformed
			}
		}
	}
	return p, nil
}

func DecodeCombatRound(p []byte) ([]CombatStep, error) {
	if len(p) > CombatMaxRoundBytes {
		return nil, ErrMalformed
	}
	r := NewReader(p)
	n := int(r.U16())
	if n == 0 || n > CombatMaxRoundSteps {
		return nil, ErrMalformed
	}
	steps := make([]CombatStep, 0, n)
	var total time.Duration
	packets := 0
	for i := 0; i < n; i++ {
		step := CombatStep{Delay: time.Duration(r.U32()) * time.Millisecond}
		count := int(r.U16())
		total += step.Delay
		packets += count
		if step.Delay > CombatMaxStepDelay || total > CombatMaxRoundDelay || packets > CombatMaxRoundPackets {
			return nil, ErrMalformed
		}
		for j := 0; j < count; j++ {
			packet := r.Bytes(int(r.U16()))
			if len(packet) == 0 || packet[0] == CommandGonlineCombat {
				return nil, ErrMalformed
			}
			step.Packets = append(step.Packets, packet)
		}
		steps = append(steps, step)
	}
	if r.Err() != nil || r.Remaining() != 0 {
		return nil, ErrMalformed
	}
	return steps, nil
}

func CombatRoundChunks(id uint32, data []byte) ([][]byte, error) {
	if id == 0 || len(data) == 0 || len(data) > CombatMaxRoundBytes {
		return nil, ErrMalformed
	}
	var packets [][]byte
	for offset := 0; offset < len(data); {
		n := min(MaxPayload-CombatChunkHeaderBytes, len(data)-offset)
		p := Builder{CommandGonlineCombat, CombatRoundChunk}.U32(id).U32(uint32(len(data))).U32(uint32(offset)).Bytes(data[offset : offset+n])
		packets = append(packets, p)
		offset += n
	}
	return packets, nil
}

// CombatAssembly accepts only contiguous chunks of one bounded round. Callers
// publish nothing until Done, then validate the complete round before playback.
type CombatAssembly struct {
	ID, Size uint32
	Data     []byte
}

func (a *CombatAssembly) Add(p []byte) (done bool, err error) {
	if len(p) <= CombatChunkHeaderBytes || len(p) > MaxPayload || p[0] != CommandGonlineCombat || p[1] != CombatRoundChunk {
		return false, ErrMalformed
	}
	r := NewReader(p[2:])
	id, size, offset := r.U32(), r.U32(), r.U32()
	data := r.Rest()
	if id == 0 || size == 0 || size > CombatMaxRoundBytes || uint64(offset)+uint64(len(data)) > uint64(size) {
		return false, ErrMalformed
	}
	if a.ID == 0 {
		if offset != 0 {
			return false, ErrMalformed
		}
		a.ID, a.Size = id, size
	}
	if id != a.ID || size != a.Size || offset != uint32(len(a.Data)) {
		return false, ErrMalformed
	}
	a.Data = append(a.Data, data...)
	return len(a.Data) == int(a.Size), nil
}

func EncodeCombatChoices(token uint32, choices [][]byte) ([]byte, error) {
	if token == 0 || len(choices) == 0 || len(choices) > CombatMaxChoices {
		return nil, ErrMalformed
	}
	p := Builder{CommandGonlineCombat, CombatRoundChoices}.U32(token).U8(byte(len(choices)))
	for _, choice := range choices {
		if !ValidCombatChoice(choice) {
			return nil, ErrMalformed
		}
		p = p.Bytes(choice)
	}
	return p, nil
}

// ValidCombatChoice keeps native item requests distinct from skill choices.
// Their subcommands delimit mixed batches without changing existing goldens.
func ValidCombatChoice(p []byte) bool {
	if len(p) < 2 || p[0] != CommandBattleAction {
		return false
	}
	if p[1] == BattleActionItemRequest {
		return len(p) == BattleItemRequestBytes
	}
	return len(p) == CombatChoiceBytes && (p[1] == BattleActionAttackRequest || p[1] == BattleActionDefendRequest || p[1] == BattleActionFleeRequest)
}

func DecodeCombatChoices(p []byte) (uint32, [][]byte, error) {
	if len(p) < CombatChoicesHeaderBytes || p[0] != CommandGonlineCombat || p[1] != CombatRoundChoices {
		return 0, nil, ErrMalformed
	}
	r := NewReader(p[2:])
	token, n := r.U32(), int(r.U8())
	if token == 0 || n == 0 || n > CombatMaxChoices || r.Remaining() < n*CombatChoiceBytes {
		return 0, nil, ErrMalformed
	}
	var choices [][]byte
	for i := 0; i < n; i++ {
		header := r.Bytes(2)
		if len(header) != 2 {
			return 0, nil, ErrMalformed
		}
		size := CombatChoiceBytes
		if header[1] == BattleActionItemRequest {
			size = BattleItemRequestBytes
		}
		choice := append(append([]byte(nil), header...), r.Bytes(size-2)...)
		if !ValidCombatChoice(choice) {
			return 0, nil, ErrMalformed
		}
		choices = append(choices, choice)
	}
	if r.Err() != nil || r.Remaining() != 0 {
		return 0, nil, ErrMalformed
	}
	return token, choices, nil
}
