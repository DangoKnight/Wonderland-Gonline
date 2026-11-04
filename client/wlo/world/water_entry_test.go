package world

import (
	"image"
	"testing"

	"wonderland-gonline/internal/clientassets"
)

func TestDirectWaterEntryRejectsInterveningLand(t *testing.T) {
	cells := make([]byte, 100)
	for x := 2; x < 10; x++ {
		for y := 0; y < 10; y++ {
			cells[x*10+y] = 2
		}
	}
	s := &Scene{Ground: clientassets.GroundPrefix{GridWidth: 10, GridHeight: 10, Cells: cells}}
	entry, ok := s.DirectWaterEntry(22, 55, 142, 55)
	if !ok || entry != image.Pt(42, 55) {
		t.Fatal("clear water entry", entry, ok)
	}
	if _, ok := s.DirectWaterEntry(2, 55, 142, 55); ok {
		t.Fatal("land approach incorrectly permitted boarding")
	}
	cells[4*10+2] = 0
	if _, ok := s.DirectWaterEntry(22, 55, 142, 55); ok {
		t.Fatal("land between player and water target was ignored")
	}
	cells[4*10+2] = 4
	if _, ok := s.DirectWaterEntry(22, 55, 142, 55); ok {
		t.Fatal("obstruction between player and target was ignored")
	}
}
