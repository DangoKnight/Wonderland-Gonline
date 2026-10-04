package game

import (
	"math"
	"reflect"
	"testing"
)

// Raw stat IDs and payloads were transcribed from AC08.HandleStatAllocation.
// Batch amounts greedily consume 4/2/1 bytes of the remaining packet, even
// when that consumes a subsequent apparent stat. Preserve that legacy layout.
func TestMigrationAllocationWidthsAndBatches(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sub     byte
		payload []byte
		target  byte
		want    []StatAllocation
	}{
		{"implicit", 28, nil, 0, []StatAllocation{{28, 1}}},
		{"byte", 29, []byte{7}, 0, []StatAllocation{{29, 7}}},
		{"word", 27, []byte{2, 1}, 0, []StatAllocation{{27, 258}}},
		{"three bytes use word", 33, []byte{2, 1, 255}, 0, []StatAllocation{{33, 258}}},
		{"dword", 30, []byte{4, 3, 2, 1}, 0, []StatAllocation{{30, 16909060}}},
		{"trailing bytes", 28, []byte{7, 0, 0, 0, 99}, 0, []StatAllocation{{28, 7}}},
		{"zero means one", 1, []byte{28, 0}, 0, []StatAllocation{{28, 1}}},
		{"targeted pet", 2, []byte{4, 33, 2, 1}, 4, []StatAllocation{{33, 258}}},
		{"batch", 2, []byte{0, 3, 28, 2, 0, 0, 0, 29, 3, 0, 0, 0, 30}, 0, []StatAllocation{{28, 2}, {29, 3}, {30, 1}}},
		{"pet batch", 1, []byte{2, 2, 27, 3, 0, 0, 0, 33, 4, 0}, 2, []StatAllocation{{27, 3}, {33, 4}}},
		{"batch invalid stat", 2, []byte{0, 2, 99, 3, 0, 0, 0, 28, 2}, 0, []StatAllocation{{28, 2}}},
		{"short batch", 1, []byte{0, 5, 28, 3}, 0, []StatAllocation{{28, 3}}},
		{"zero count", 2, []byte{0, 0, 28}, 0, []StatAllocation{{28, 1}}},
		{"greedy batch", 2, []byte{0, 2, 28, 1, 29, 2}, 0, []StatAllocation{{28, 7425}}},
		{"missing target stat", 2, []byte{3}, 3, nil},
		{"unsupported", 9, []byte{28, 1}, 0, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, got := ParseStatAllocation(tc.sub, tc.payload)
			if target != tc.target || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("target %d allocations %v; want %d %v", target, got, tc.target, tc.want)
			}
		})
	}
}

func TestMigrationAllocationBoundsPreserveBudget(t *testing.T) {
	for _, stat := range []byte{28, 29, 27, 33, 30} {
		c := Character{StatPoints: 7, Base: Attributes{math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16, math.MaxUint16}}
		p := Pet{StatPoints: c.StatPoints, Base: c.Base}
		before := c
		requests := []StatAllocation{{99, 1}, {stat, 0}, {stat, 1}, {stat, math.MaxUint32}}
		if c.Allocate(requests) || !reflect.DeepEqual(c, before) || p.Allocate(requests) || p.Base != before.Base || p.StatPoints != before.StatPoints {
			t.Fatalf("stat %d consumed budget or wrapped an attribute", stat)
		}
	}
	c := Character{StatPoints: 3, Base: Attributes{Strength: math.MaxUint16 - 1}}
	if !c.Allocate([]StatAllocation{{28, 2}, {28, 1}, {29, 3}, {29, 2}}) || c.Base.Strength != math.MaxUint16 || c.Base.Constitution != 2 || c.StatPoints != 0 {
		t.Fatal("rejected entries did not preserve budget for subsequent allocations", c)
	}
}

func TestMigrationInventoryTypeRules(t *testing.T) {
	// Source Item.Stackable/Dropable, independently transcribed. Other metadata
	// enums do not implement reachable wear/trade restrictions in the reference.
	stack := map[byte]bool{10: true, 17: true, 20: true, 21: true, 23: true, 24: true, 25: true, 26: true, 28: true, 30: true, 31: true, 32: true, 33: true, 34: true, 35: true, 36: true, 37: true, 38: true, 40: true, 41: true, 51: true, 52: true, 54: true}
	drop := map[byte]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 8: true, 9: true, 10: true, 12: true, 13: true, 14: true, 15: true, 23: true, 31: true, 32: true, 33: true, 34: true, 35: true, 36: true, 37: true}
	for n := 0; n <= math.MaxUint8; n++ {
		d := ItemDefinition{Type: byte(n)}
		limit := byte(1)
		if stack[byte(n)] {
			limit = 50
		}
		if d.StackLimit() != limit || d.Droppable() != drop[byte(n)] {
			t.Fatalf("type %d: stack=%d drop=%t", n, d.StackLimit(), d.Droppable())
		}
	}
}
