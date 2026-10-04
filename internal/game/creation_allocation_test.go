package game

import "testing"

func TestCreationAllocationBudget(t *testing.T) {
	for _, a := range []Attributes{{5, 0, 0, 0, 0}, {0, 5, 0, 0, 0}, {0, 0, 5, 0, 0}, {0, 0, 0, 5, 0}, {0, 0, 0, 0, 5}, {1, 1, 1, 1, 1}} {
		if err := (Appearance{Base: a}).ValidateCreationAllocation(); err != nil {
			t.Fatal(a, err)
		}
	}
	for _, a := range []Attributes{{}, {1, 1, 1, 1, 0}, {5, 5, 5, 5, 5}, {255, 255, 255, 255, 255}, {65535, 1, 0, 0, 0}} {
		if err := (Appearance{Base: a}).ValidateCreationAllocation(); err == nil {
			t.Fatalf("accepted invalid allocation %+v", a)
		}
	}
}
