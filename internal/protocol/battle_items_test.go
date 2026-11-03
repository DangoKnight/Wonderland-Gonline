package protocol

import (
	"bytes"
	"reflect"
	"testing"
)

func TestBattleItemChoiceAndMixedRoundBatchGoldens(t *testing.T) {
	item := []byte{50, 2, 4, 2, 3, 2, 3, 125, 0, 0, 1, 0}
	if !bytes.Equal(BattleItemChoice(4, 2, 3, 2, 32003, 1), item) {
		t.Fatal("native item layout")
	}
	choices := [][]byte{item, {50, 4, 3, 2, 3, 2, 117, 234}}
	want := []byte{254, 2, 9, 0, 0, 0, 2, 50, 2, 4, 2, 3, 2, 3, 125, 0, 0, 1, 0, 50, 4, 3, 2, 3, 2, 117, 234}
	p, err := EncodeCombatChoices(9, choices)
	if err != nil || !bytes.Equal(p, want) {
		t.Fatal("mixed choice bytes", p, err)
	}
	token, got, err := DecodeCombatChoices(want)
	if err != nil || token != 9 || !reflect.DeepEqual(got, choices) {
		t.Fatal("mixed choice decode", got, err)
	}
	for i := 0; i < len(want); i++ {
		if _, _, err := DecodeCombatChoices(want[:i]); err == nil {
			t.Fatal("truncation", i)
		}
	}
	if _, _, err := DecodeCombatChoices(append(want, 0)); err == nil {
		t.Fatal("trailing bytes accepted")
	}
}
