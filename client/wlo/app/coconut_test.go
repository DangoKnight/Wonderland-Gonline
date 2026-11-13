package app

import (
	"bytes"
	"image"
	"os"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/game"
	"wonderland-gonline/internal/protocol"
)

func TestDroppedCoconutBehindNativePalmLeaves(t *testing.T) {
	c := testClient(t)
	now := time.Unix(100, 0)
	c.Now = func() time.Time { return now }
	c.dispatch(selfPacket(10001, 2, 11016, 1402, 855, 0, 444444444, 444444444, nil, "Dango"))
	c.dispatch([]byte{5, 4})
	now = now.Add(time.Second)
	c.Frame()
	before := append([]uint16(nil), c.Screen.Pix...)
	// Native ground spawn: Coconut at the foreground palm shown in the report.
	c.dispatch([]byte{23, 3, 106, 160, 174, 5, 103, 3, 0, 0, 0, 0, 1})
	c.Frame()
	cx, cy := c.World.Camera()
	icon := surface.New(c.Screen.W, c.Screen.H)
	c.drawGroundItemOn(icon, 41066, image.Pt(1454-cx, 871-cy))
	covered, visible := 0, 0
	for _, o := range c.World.Scene.Objects {
		if o.Resource != 11016004 {
			continue
		}
		for y := range o.Image.H {
			for x := range o.Image.W {
				at := (o.Y-cy+y)*c.Screen.W + o.X - cx + x
				if icon.Pix[at] == 0 {
					continue
				}
				if o.Image.Pix[y*o.Image.W+x] != 0 {
					covered++
					if c.Screen.Pix[at] != before[at] {
						t.Fatalf("ground Coconut painted over native leaf pixel %d,%d", x, y)
					}
				} else if c.Screen.Pix[at] != before[at] {
					visible++
				}
			}
		}
	}
	if covered == 0 || visible == 0 {
		t.Fatalf("fixture must cover both leaves and fruit: covered %d visible %d", covered, visible)
	}
	if path := os.Getenv("COCONUT_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
}

func TestCapturedBeachCoconutPositionAndDepth(t *testing.T) {
	for _, ref := range []struct {
		name             string
		slot             uint16
		playerX, playerY uint16
		x, y             int
		depth            int32
	}{
		{"right", 1, 1322, 895, 1454, 871, 160}, {"left", 2, 562, 635, 462, 538, 170},
	} {
		t.Run(ref.name, func(t *testing.T) {
			c := testClient(t)
			now := time.Unix(100, 0)
			c.Now = func() time.Time { return now }
			c.dispatch(selfPacket(10001, 2, 11016, ref.playerX, ref.playerY, 0, 444444444, 444444444, nil, "Dango"))
			c.dispatch([]byte{5, 4})
			now = now.Add(time.Second)
			c.Frame()
			before := append([]uint16(nil), c.Screen.Pix...)
			// Complete AC23:4 captured from the real server, 2026-11-03.
			c.dispatch([]byte{23, 4, 3, 1, 0, 106, 160, 0, 0, 174, 5, 103, 3, 160, 0, 0, 0, 3, 2, 0, 106, 160, 0, 0, 206, 1, 26, 2, 170, 0, 0, 0})
			c.Frame()
			item := c.floorItems[ref.slot]
			if item.x != ref.x || item.y != ref.y || item.depth != ref.depth {
				t.Fatal("captured placement discarded", item)
			}
			cx, cy := c.World.Camera()
			want := image.Rect(ref.x-cx-16, ref.y-cy-16, ref.x-cx+16, ref.y-cy+16)
			if got := c.groundItemBounds(item); got != want {
				t.Fatalf("native centred canvas %v want %v", got, want)
			}
			icon := surface.New(c.Screen.W, c.Screen.H)
			c.drawGroundItemOn(icon, 41066, image.Pt(ref.x-cx, ref.y-cy))
			visible := 0
			for at, pixel := range icon.Pix {
				if pixel != 0 && c.Screen.Pix[at] != before[at] {
					visible++
				}
			}
			if visible == 0 {
				t.Fatal("native Coconut hidden by background palm")
			}
			if path := os.Getenv("COCONUT_REFERENCE_SNAPSHOT"); path != "" {
				savePNG(t, path+"-"+ref.name+".png", c)
			}
			// Signed backwards offsets must not wrap to a foreground layer.
			c.dispatch([]byte{23, 4, 3, 3, 0, 106, 160, 0, 0, 206, 1, 26, 2, 232, 255, 255, 255})
			if c.floorItems[3].depth != -24 {
				t.Fatal("signed draw depth decoded as unsigned")
			}
		})
	}
}

func TestCoconutFloorPickupArcAndNotice(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{23, 3, 106, 160, 177, 5, 117, 3, 0, 0, 0, 0, 1})
	c.dispatch([]byte{23, 2, 1, 0, 1})
	c.dispatch([]byte{23, 6, 106, 160, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	if len(c.floorItems) != 0 || len(c.worldItemFlights) != 1 || c.worldItemFlights[0].source != image.Pt(1457, 885) {
		t.Fatal("ground pickup failed to start its flight")
	}
	if len(c.Notices.items) != 1 || !bytes.Equal(c.Notices.items[0].text, []byte("Obtain Coconut x1")) {
		t.Fatal("missing ground pickup notice")
	}
	c.dispatch([]byte{23, 3, 106, 160, 177, 5, 117, 3, 0, 0, 0, 0, 1})
	c.dispatch([]byte{23, 2, 1, 0, 0})
	if len(c.worldItemFlights) != 1 {
		t.Fatal("another player's pickup created a local acquisition flight")
	}
}

func TestObservedFloorPickupFliesToCollector(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.connection = connectionRuntime{enabled: true, features: protocol.ConnectionObservedPickups}
	c.dispatch([]byte{23, 3, 106, 160, 177, 5, 117, 3, 0, 0, 0, 0, 1})
	packet := []byte{23, 2, 1, 0, 4, 3, 2, 1, 232, 3, 208, 7}
	before := c.InventoryState.Bag
	c.dispatch(packet)
	if len(c.floorItems) != 0 || len(c.worldItemFlights) != 1 {
		t.Fatal("observer did not remove item/start flight")
	}
	flight := c.worldItemFlights[0]
	if flight.source != image.Pt(1457, 885) || flight.target != image.Pt(1000, 2000-worldItemTargetLift) {
		t.Fatal("flight targeted the observer instead of collector", flight)
	}
	if before != c.InventoryState.Bag || len(c.Notices.items) != 0 {
		t.Fatal("observer received pickup rewards")
	}
	c.dispatch(packet)
	if len(c.worldItemFlights) != 1 {
		t.Fatal("repeated removal duplicated flight")
	}
	c.dispatch([]byte{23, 3, 106, 160, 177, 5, 117, 3, 0, 0, 0, 0, 1})
	c.dispatch([]byte{23, 2, 1, 0})
	if len(c.floorItems) != 0 || len(c.worldItemFlights) != 1 {
		t.Fatal("native silent removal animated")
	}
}

func TestGroundItemUsesNativeCenterAnchor(t *testing.T) {
	c, _, _ := enteredClient(t)
	sent := wire(t, c)
	// Coconut centred at the player's position, native 32-by-32 item canvas.
	c.dispatch([]byte{23, 3, 106, 160, 18, 4, 51, 4, 0, 0, 0, 0, 1})
	item := c.floorItems[1]
	at := c.floorItemPoint(item)
	want := image.Rect(at.X-16, at.Y-16, at.X+16, at.Y+16)
	if got := c.groundItemBounds(item); got != want {
		t.Fatalf("native centred bounds %v, want %v", got, want)
	}
	img := surface.New(c.Screen.W, c.Screen.H)
	c.drawGroundItemOn(img, item.id, at)
	below := 0
	for y := at.Y; y < want.Max.Y; y++ {
		for x := want.Min.X; x < want.Max.X; x++ {
			if img.Pix[y*img.W+x] != 0 {
				below++
			}
		}
	}
	if below == 0 {
		t.Fatal("ground artwork is still entirely above its centre")
	}
	if !c.pickupFloorItem(at.X, at.Y+10) {
		t.Fatal("visible lower half of item cannot be clicked")
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{23, 2, 1, 0}) {
		t.Fatal("wrong pickup target", got)
	}
}

func TestCoconutNativeAcquisitionArcAndNotice(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.InventoryState.Bag = game.Inventory{}
	c.World.NPCs[1].X, c.World.NPCs[1].Y = 950, 950
	if err := c.sendNPCClick(1); err != nil {
		t.Fatal(err)
	}
	if got := sent(); len(got) != 1 || !bytes.Equal(got[0], []byte{20, 1, 1, 0}) {
		t.Fatal("missing native actor click", got)
	}
	// Independent native AC23:6 golden: Coconut, one, actor-flight flag.
	p := []byte{23, 6, 106, 160, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	c.dispatch(p)
	c.dispatch([]byte{20, 8})
	if c.InventoryState.Bag[0].ID != 41066 || c.InventoryState.Bag[0].Count != 1 || len(c.worldItemFlights) != 1 {
		t.Fatal("missing native Coconut receipt/flight")
	}
	if len(c.Notices.items) != 1 || !bytes.Equal(c.Notices.items[0].text, []byte("Obtain Coconut x1")) {
		t.Fatal("missing acquisition notification", c.Notices.items)
	}
	f := c.worldItemFlights[0]
	if f.position(*now) != f.source || f.position(now.Add(600*time.Millisecond)) != f.target {
		t.Fatal("flight endpoints")
	}
	mid := f.position(now.Add(300 * time.Millisecond))
	if mid.X != (f.source.X+f.target.X)/2 || mid.Y >= (f.source.Y+f.target.Y)/2 {
		t.Fatal("pickup does not follow an upward parabola", mid)
	}
	c.drawWorldItemGains(now.Add(600 * time.Millisecond))
	if len(c.worldItemFlights) != 0 {
		t.Fatal("pickup flight did not expire")
	}
	p[32] = 0
	c.dispatch(p)
	if len(c.worldItemFlights) != 0 {
		t.Fatal("unflagged grant flew from stale actor")
	}
	p[4] = 0
	before := c.InventoryState.Bag
	c.dispatch(p)
	if c.InventoryState.Bag != before || len(c.worldItemFlights) != 0 {
		t.Fatal("invalid receipt presented")
	}
}
