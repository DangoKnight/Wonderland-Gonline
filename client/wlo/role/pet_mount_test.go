package role

import (
	"encoding/json"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

func TestNativePetMountPlacements(t *testing.T) {
	raw := []byte(`{"text":"NpcPic=1867\r\nPose=sit\r\nFixedPosOnFirst=true\r\nSex=1\r\nStandPosX=0,12,12,14,4,-12,-12,-12\r\nStandPosY=78,75,71,73,76,73,71,75\r\nMovePosX=0,12,12,14,4,-12,-12,-12\r\nMovePosY=78,75,71,73,76,73,71,75"}`)
	p, err := ParseMountPlacements(raw)
	if err != nil {
		t.Fatal(err)
	}
	mount := p[1867]
	if !mount.Sit || !mount.FixedFirst || mount.Stand[0][3] != [2]int{14, 73} || mount.Move[0][7] != [2]int{-12, 75} {
		t.Fatal(mount)
	}
	if _, err = ParseMountPlacements([]byte(`{"text":"NpcPic=1867\nSex=5"}`)); err == nil {
		t.Fatal("invalid body")
	}
	if _, err = ParseMountPlacements([]byte(`{"text":"NpcPic=1867\nSex=1\nStandPosX=0,12"}`)); err == nil {
		t.Fatal("invalid facing count")
	}
}

func TestPetRideKeepsMountedPoseWhileMoving(t *testing.T) {
	root := t.TempDir()
	writePlacementSprite(t, root, "002", "2000.jmp", color.NRGBA{R: 255, A: 255})
	writePlacementSprite(t, root, "001", "1867.jmp", color.NRGBA{B: 255, A: 255})
	h := NewHuman(NewLibrary(root), nil)
	h.body = 1
	h.colors = NeutralColors()
	h.Now = func() time.Time { return time.Unix(0, 0) }
	h.SetPetMount(NewNPC(h.Lib, 867, [4]uint32{}))
	for _, tc := range []struct {
		name      string
		placement *MountPlacement
		base      int
	}{
		{"native seated default", nil, 46},
		{"explicit ride", &MountPlacement{}, 18},
		{"seated mount", &MountPlacement{Sit: true}, 46},
		{"standing mount", &MountPlacement{StandPose: true}, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.SetPetMountPlacement(tc.placement)
			for facing := int32(0); facing < 8; facing++ {
				for _, movement := range []int32{0, 8} {
					h.DrawBody(surface.New(250, 250), 100, 100, facing+movement)
					if h.lastAction != tc.base+int(facing) {
						t.Fatalf("direction %d: rider action %d, want %d", facing+movement, h.lastAction, tc.base+int(facing))
					}
				}
			}
		})
	}
	h.SetPetMount(nil)
	h.SetPetMountPlacement(nil)
	h.DrawBody(surface.New(250, 250), 100, 100, 6)
	if h.lastAction != 6 {
		t.Fatal("dismount retained riding pose")
	}
}

func TestPetMountNativeRasterOrder(t *testing.T) {
	root := t.TempDir()
	writePlacementSprite(t, root, "002", "2000.jmp", color.NRGBA{R: 255, A: 255})
	writePlacementSprite(t, root, "001", "1867.jmp", color.NRGBA{B: 255, A: 255})
	h := NewHuman(NewLibrary(root), nil)
	defer h.Lib.Sprites.Close()
	h.body = 1
	h.colors = NeutralColors()
	h.Now = func() time.Time { return time.Unix(0, 0) }
	h.SetPetMount(NewNPC(h.Lib, 867, [4]uint32{}))
	// Equal saddle origins make the fixture pixels overlap, exposing layer order.
	p := MountPlacement{}
	for body := range p.Stand {
		for facing := range p.Stand[body] {
			p.Stand[body][facing][1] = -30
		}
	}
	h.SetPetMountPlacement(&p)
	// Native 0x413af1 draws the mount, then 0x413b70 draws the human.
	expected := [8]uint16{0xf800, 0xf800, 0xf800, 0xf800, 0xf800, 0xf800, 0xf800, 0xf800}
	for facing, want := range expected {
		for _, movement := range []int32{0, 8} {
			dst := surface.New(250, 250)
			h.DrawBody(dst, 100, 100, int32(facing)+movement)
			drawn := false
			for _, pixel := range dst.Pix {
				if pixel == 0 {
					continue
				}
				drawn = true
				if pixel != want {
					t.Fatalf("facing %d movement %d: front pixel %04x, want %04x", facing, movement, pixel, want)
				}
			}
			if !drawn {
				t.Fatal("missing mount/rider fixture")
			}
		}
	}
}

// Values independently transcribed from WLRI DAT_004c2a78/004c2b2c and
// the seated saddle table, rather than using the generated Go definitions.
func TestPetMountNativeTablesAndPrecedence(t *testing.T) {
	p := ResolveMountPlacement(1185, map[uint16]MountPlacement{1185: {Stand: [4][8][2]int{{{999, 999}}}}})
	if !p.FixedFirst || p.Stand[0][2] != [2]int{-20, 65} || p.Move[0][2] != [2]int{0, -20} {
		t.Fatalf("compiled saddle or movement correction missing: %+v", p)
	}
	p = ResolveMountPlacement(1043, nil)
	if !p.Sit || p.Stand[3][4] != [2]int{0, 5} {
		t.Fatal("authored seated saddle", p)
	}
	p = ResolveMountPlacement(1195, nil)
	if p.Stand[0][2] != [2]int{5, 0} || p.Stand[3][6] != [2]int{-5, 0} {
		t.Fatal("native humanoid fallback", p)
	}
	p = ResolveMountPlacement(1867, nil)
	if !p.Sit || p.Stand != [4][8][2]int{} {
		t.Fatal("native seated fallback", p)
	}
	p = ResolveMountPlacement(1867, map[uint16]MountPlacement{1867: {FixedFirst: true, Stand: [4][8][2]int{{{12, 78}}}, Move: [4][8][2]int{{{200, 300}}}}})
	if !p.Sit || !p.FixedFirst || p.Stand[0][0] != [2]int{12, 78} || p.Move != [4][8][2]int{} {
		t.Fatal("text fallback", p)
	}
}

func TestPetMountNativeSaddleGeometryAndName(t *testing.T) {
	root := t.TempDir()
	writePlacementSprite(t, root, "001", "1867.jmp", color.NRGBA{B: 255, A: 255})
	path := filepath.Join(root, "sprites", "001", "editable.json")
	raw, _ := os.ReadFile(path)
	var doc clientassets.EditableSprites
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	// Native canvas 180, anchor 40 -> height above ground reference 140.
	doc.Sprites[0].Frames[0].CanvasHeight = 180
	doc.Sprites[0].Frames[0].AnchorY = 40
	doc.Sprites[0].Frames = append(doc.Sprites[0].Frames, doc.Sprites[0].Frames[0])
	doc.Sprites[0].Frames[1].AnchorY = 50
	for i := range doc.Sprites[0].Animations {
		doc.Sprites[0].Animations[i].Frames = []int{0, 1}
	}
	raw, _ = json.Marshal(doc)
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	h := NewHuman(NewLibrary(root), nil)
	defer h.Lib.Sprites.Close()
	h.body = 1
	h.SetPetMount(NewNPC(h.Lib, 867, [4]uint32{}))
	placement := MountPlacement{Stand: [4][8][2]int{{{12, 78}}}, Move: [4][8][2]int{{{2, -3}}}}
	h.SetPetMountPlacement(&placement)
	for _, tc := range []struct {
		scale, preset   byte
		wantY, wantName int
	}{
		{0, 0, 0, 3}, {0, 1, 68, 67}, {1, 0, -108, 71}, {1, 1, 128, 135},
	} {
		h.SetPetMountGeometry(tc.scale, tc.preset)
		if x, y := h.petRiderOffset(8); x != 12 || y != tc.wantY {
			t.Fatalf("flags %d/%d: offset %d,%d want 12,%d", tc.scale, tc.preset, x, y, tc.wantY)
		}
		if x, y := h.petRiderOffset(0); x != 14 || y != tc.wantY-3 {
			t.Fatalf("movement correction: %d,%d", x, y)
		}
		if got := h.NameLift(); got != tc.wantName {
			t.Fatalf("flags %d/%d: name lift %d want %d", tc.scale, tc.preset, got, tc.wantName)
		}
	}
	h.SetPetMountGeometry(0, 0)
	h.petMount.Hold(1, false)
	if _, y := h.petRiderOffset(8); y != 10 {
		t.Fatalf("animated saddle = %d", y)
	}
	placement.FixedFirst = true
	if _, y := h.petRiderOffset(8); y != 0 {
		t.Fatalf("fixed-first saddle = %d", y)
	}
	if frame := h.petMount.mountFrame(8, false); frame.AnchorY != 50 {
		t.Fatal("fixed positioning froze mount animation")
	}
	h.SetPetMount(nil)
	if h.NameLift() != 0 {
		t.Fatal("dismount retained name clearance")
	}
}

func TestPetNativeRasterScaleAndBounds(t *testing.T) {
	root := t.TempDir()
	writePlacementSprite(t, root, "001", "1867.jmp", color.NRGBA{B: 255, A: 255})
	n := NewNPC(NewLibrary(root), 867, [4]uint32{})
	defer n.Lib.Sprites.Close()
	n.SetHeightScale(1)
	dst := surface.New(20, 20)
	n.Draw(dst, 5, 5, 8)
	if got := n.Bounds(5, 5, 8); got.Dx() != 2 || got.Dy() != 2 || got.Min.X != 5 || got.Min.Y != 5 {
		t.Fatal("scaled bounds", got)
	}
	for _, at := range [][2]int{{5, 5}, {6, 5}, {5, 6}, {6, 6}} {
		if dst.Pix[at[1]*dst.W+at[0]] != 0x001f {
			t.Fatal("missing scaled pet pixel", at)
		}
	}
	if dst.Pix[5*dst.W+7] != 0 {
		t.Fatal("scaled pet exceeded bounds")
	}
}
