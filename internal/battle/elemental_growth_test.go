package battle

import (
	"testing"
	"wonderland-go/internal/game"
)

func TestPlayerFighterConfiguredGrowth(t *testing.T) {
	c := hero()
	g := game.DefaultElementalGrowth()
	g.Fire.ATK.Level = 10
	g.Fire.ATK.STR = 3
	g.Fire.HP.Base = 300
	g.Fire.SP.Base = 150
	f := PlayerFighter(c, nil, 0, g)
	if f.Atk != 25 || f.MaxHP != 321 || f.MaxSP != 177 || f.HP != 100 || f.SP != 50 {
		t.Fatalf("configured fighter: %+v", f)
	}
}
