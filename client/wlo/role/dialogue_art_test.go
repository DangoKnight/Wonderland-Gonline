package role

import (
	"os"
	"path/filepath"
	"testing"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/sprites"
	"wonderland-gonline/client/wlo/surface"
)

func TestDialogueArtRemainsVisibleWhileBlinking(t *testing.T) {
	root := filepath.Join("..", "..", "..", "data")
	if _, err := os.Stat(root); err != nil {
		t.Skip("client assets not installed")
	}
	root = filepath.Join(root, "sprites")
	lib := NewLibraryWith(sprites.NewManager([]string{root}, []string{root}))
	defer lib.Sprites.Close()
	h := NewHuman(lib, nil)
	checked := 0
	for _, appearance := range [][2]byte{{1, 0}, {2, 0}, {2, 1}, {3, 0}, {3, 1}, {3, 2}, {3, 3}, {4, 0}, {4, 1}, {4, 2}, {4, 3}, {4, 4}, {4, 5}, {4, 6}, {4, 7}} {
		h.SetCharacter(&login.CharacterSlot{Body: appearance[0], Head: appearance[1], Color1: 444444444, Color2: 444444444})
		if h.FaceWidth(TalkArtAction) == 0 {
			continue
		}
		checked++
		var pixels [2]int
		for i, blink := range []bool{false, true} {
			dst := surface.New(500, 600)
			h.DrawFace(dst, 250, 550, TalkArtAction, blink)
			visible := false
			for _, pixel := range dst.Pix {
				visible = visible || pixel != 0
				if pixel != 0 {
					pixels[i]++
				}
			}
			if !visible {
				t.Errorf("body/head %v blink=%v: large dialogue art disappeared", appearance, blink)
			}
		}
		if pixels[1] < pixels[0]/2 {
			t.Errorf("body/head %v: blinking replaced full artwork with an eyelid patch (%v pixels)", appearance, pixels)
		}
	}
	if checked == 0 {
		t.Fatal("no player dialogue art was checked")
	}
}
