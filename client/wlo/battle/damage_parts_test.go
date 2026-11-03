package battle

import (
	"reflect"
	"testing"
)

func TestDamagePartsConserveWideTotals(t *testing.T) {
	for _, test := range []struct {
		name   string
		motion Motion
		amount uint64
		want   []uint64
	}{
		{"single", Motion{}, 91, []uint64{91}},
		{"authored", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 33}, {Percent: 33}, {Percent: 34}}}, 101, []uint64{33, 33, 35}},
		{"repeated weights", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 100}, {Percent: 100}, {Percent: 100}}}, 101, []uint64{34, 34, 33}},
		{"missing weights", Motion{DamageMode: 2, Events: []MotionEvent{{}, {}, {}}}, 100, []uint64{33, 33, 34}},
		{"ties even", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 50}, {Percent: 50}}}, 5, []uint64{2, 3}},
		{"small", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 33}, {Percent: 33}, {Percent: 34}}}, 1, []uint64{0, 0, 1}},
		{"wide combo", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 50}, {Percent: 50}}}, 8589934590, []uint64{4294967295, 4294967295}},
		{"maximum total", Motion{DamageMode: 2, Events: []MotionEvent{{Percent: 100}, {Percent: 100}, {Percent: 100}}}, ^uint64(0), []uint64{6148914691236517205, 6148914691236517205, 6148914691236517205}},
	} {
		t.Run(test.name, func(t *testing.T) {
			parts := test.motion.DamageParts64(test.amount)
			if !reflect.DeepEqual(parts, test.want) {
				t.Fatalf("parts %v, want %v", parts, test.want)
			}
			remaining := test.amount
			for _, part := range parts {
				if part > remaining {
					t.Fatal("damage exceeds total")
				}
				remaining -= part
			}
			if remaining != 0 {
				t.Fatal("lost damage", remaining)
			}
		})
	}
}

func TestActionPacketsHaveIndependentLocalBatches(t *testing.T) {
	s := fixture(t)
	// Two actor records in one native AC50:1. No local batch field is on wire.
	one := []byte{17, 0, 4, 2, 17, 39, 0, 1, 2, 2, 1, 0, 1, 25, 10, 0, 0, 0, 1}
	packet := append(append([]byte{50, 1}, one...), one...)
	if _, valid := s.Apply(packet); !valid {
		t.Fatal("valid native batch rejected")
	}
	if len(s.Actions) != 2 || s.Actions[0].Batch == 0 || s.Actions[0].Batch != s.Actions[1].Batch {
		t.Fatal("packet batch lost")
	}
	previous := s.Actions[0].Batch
	if _, valid := s.Apply([]byte{50, 1, 1}); valid {
		t.Fatal("malformed batch accepted")
	}
	if _, valid := s.Apply(append([]byte{50, 1}, one...)); !valid {
		t.Fatal("second native packet rejected")
	}
	if s.Actions[2].Batch != previous+1 {
		t.Fatal("packet boundary or invalid-packet atomicity lost")
	}
}
