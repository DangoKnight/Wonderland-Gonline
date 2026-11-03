package protocol

import (
	"bytes"
	"reflect"
	"testing"
	"time"
)

func TestCombatRoundRawGoldens(t *testing.T) {
	capability := []byte{1, 240, 71, 76, 66, 82, 1, 0, 1, 0, 0, 0}
	if !bytes.Equal(CombatCapabilities(HandshakeCombatOffer, CombatCapabilityRoundChain), capability) {
		t.Fatal("capability bytes")
	}
	flags, ok := ParseCombatCapabilities(capability, HandshakeCombatOffer)
	if !ok || flags != 1 {
		t.Fatal("capability decode")
	}
	if _, ok := ParseCombatCapabilities([]byte{1, 240, 1}, HandshakeCombatOffer); ok {
		t.Fatal("generic echo negotiated")
	}
	steps := []CombatStep{{Delay: 100 * time.Millisecond, Packets: [][]byte{{50, 6, 4, 2, 0}}}, {Delay: 300 * time.Millisecond, Packets: [][]byte{{53, 3, 2, 2}}}}
	golden := []byte{2, 0, 100, 0, 0, 0, 1, 0, 5, 0, 50, 6, 4, 2, 0, 44, 1, 0, 0, 1, 0, 4, 0, 53, 3, 2, 2}
	encoded, err := EncodeCombatRound(steps)
	if err != nil || !bytes.Equal(encoded, golden) {
		t.Fatal("round bytes", encoded, err)
	}
	decoded, err := DecodeCombatRound(golden)
	if err != nil || !reflect.DeepEqual(decoded, steps) {
		t.Fatal("round decode", decoded, err)
	}
	chunks, err := CombatRoundChunks(9, golden)
	want := append([]byte{254, 1, 9, 0, 0, 0, 27, 0, 0, 0, 0, 0, 0, 0}, golden...)
	if err != nil || len(chunks) != 1 || !bytes.Equal(chunks[0], want) {
		t.Fatal("chunk bytes", chunks, err)
	}
	choices := [][]byte{{50, 1, 4, 2, 2, 2, 17, 39}, {50, 4, 3, 2, 3, 2, 117, 234}}
	p, err := EncodeCombatChoices(9, choices)
	choiceGolden := []byte{254, 2, 9, 0, 0, 0, 2, 50, 1, 4, 2, 2, 2, 17, 39, 50, 4, 3, 2, 3, 2, 117, 234}
	if err != nil || !bytes.Equal(p, choiceGolden) {
		t.Fatal("choice bytes", p, err)
	}
	token, got, err := DecodeCombatChoices(choiceGolden)
	if err != nil || token != 9 || !reflect.DeepEqual(got, choices) {
		t.Fatal("choice decode", token, got, err)
	}
	for n := 0; n < len(golden); n++ {
		if _, err := DecodeCombatRound(golden[:n]); err == nil {
			t.Fatal("accepted truncated round", n)
		}
	}
	for n := 0; n < len(choiceGolden); n++ {
		if _, _, err := DecodeCombatChoices(choiceGolden[:n]); err == nil {
			t.Fatal("accepted truncated choices", n)
		}
	}
}

func TestCombatRoundChunkingAndBounds(t *testing.T) {
	steps := []CombatStep{{Packets: [][]byte{bytes.Repeat([]byte{2}, 60000), bytes.Repeat([]byte{2}, 6000)}}}
	data, err := EncodeCombatRound(steps)
	if err != nil {
		t.Fatal(err)
	}
	chunks, err := CombatRoundChunks(4, data)
	if err != nil || len(chunks) != 2 {
		t.Fatal("chunking", len(chunks), err)
	}
	var assembly CombatAssembly
	if done, err := assembly.Add(chunks[0]); done || err != nil {
		t.Fatal("early publication", done, err)
	}
	if done, err := assembly.Add(chunks[1]); !done || err != nil || !bytes.Equal(assembly.Data, data) {
		t.Fatal("reassembly", done, err)
	}
	var outOfOrder CombatAssembly
	if _, err := outOfOrder.Add(chunks[1]); err == nil {
		t.Fatal("accepted gap")
	}
	var duplicate CombatAssembly
	duplicate.Add(chunks[0])
	if _, err := duplicate.Add(chunks[0]); err == nil {
		t.Fatal("accepted overlapping chunk")
	}
	for _, bad := range [][]CombatStep{
		{{Delay: -time.Millisecond}}, {{Delay: time.Minute + time.Millisecond}}, {{Packets: [][]byte{{254, 1}}}}, make([]CombatStep, 513),
	} {
		if _, err := EncodeCombatRound(bad); err == nil {
			t.Fatal("accepted invalid chain")
		}
	}
	if _, err := DecodeCombatRound([]byte{255, 255}); err == nil {
		t.Fatal("unbounded step allocation")
	}
	bad := []byte{254, 1, 4, 0, 0, 0, 255, 255, 255, 255, 0, 0, 0, 0, 2}
	var huge CombatAssembly
	if _, err := huge.Add(bad); err == nil || huge.Data != nil {
		t.Fatal("unbounded assembly")
	}
}
