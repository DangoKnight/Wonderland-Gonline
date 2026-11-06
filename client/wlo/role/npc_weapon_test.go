package role

import (
	"image/color"
	"testing"
	"wonderland-gonline/client/wlo/surface"
)

func TestNPCBattleWeaponPlacement(t *testing.T) {
	for _, tc := range []struct {
		family string
		sprite uint16
	}{
		{"002w", 2001}, {"003w", 3001}, {"004w", 4001}, {"005w", 5001},
	} {
		t.Run(tc.family, func(t *testing.T) {
			root := t.TempDir()
			writePlacementSprite(t, root, "001", "1867.jmp", color.NRGBA{R: 255, A: 255})
			writePlacementSprite(t, root, tc.family, "weapon.jmp", color.NRGBA{G: 255, A: 255})
			n := NewNPC(NewLibrary(root), 1867, [4]uint32{})
			defer n.Lib.Sprites.Close()
			n.SetBattleWeapon(tc.sprite)
			dst := surface.New(250, 250)
			n.Draw(dst, 100, 100, 17)
			// Native weapon canvas origin is 68 pixels below the body origin.
			if dst.Pix[100*dst.W+100] != surface.RGB565(255, 0, 0) || dst.Pix[168*dst.W+100] != surface.RGB565(0, 255, 0) {
				t.Fatal("NPC body and separate weapon layer have incorrect placement")
			}
			n.SetBattleWeapon(0)
			dst = surface.New(250, 250)
			n.Draw(dst, 100, 100, 17)
			if dst.Pix[168*dst.W+100] != 0 {
				t.Fatal("disabled weapon remained visible")
			}
		})
	}
}
