package assets

import (
	"testing"
	"wonderland-gonline/internal/game"
)

func TestPetFoodNativeTargetGolden(t *testing.T) {
	i := NativeItem{Definition: game.ItemDefinition{Type: 20}}
	// Independent decoded Orchid Candy target bytes at Item.dat +413.
	i.Record[413], i.Record[414] = 0x4c, 0x37
	if i.PetFoodTarget() != 14156 {
		t.Fatal("pet-food target")
	}
	i.Definition.Type = 0
	if i.PetFoodTarget() != 0 {
		t.Fatal("unrelated item classified as food")
	}
}
