package world

import (
	"image"
	"testing"
	"wonderland-gonline/client/wlo/housing"
	"wonderland-gonline/client/wlo/login"
)

func TestTentSceneIndependentTerrainAndFurniture(t *testing.T) {
	a, err := LoadScene(login.Assets{}, 63507)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadScene(login.Assets{}, 63507)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Walkable(460, 700) || a.Walkable(20, 20) {
		t.Fatal("native entry or room boundary")
	}
	at := housing.Pixel(35, 43, 1)
	a.SetTentWalkGrid(housing.Room(housing.DefaultRoom), []image.Rectangle{image.Rect(34, 42, 37, 45)})
	if a.Walkable(at.X, at.Y) || !b.Walkable(at.X, at.Y) {
		t.Fatal("furniture collision missing or leaked into other tent")
	}
}
