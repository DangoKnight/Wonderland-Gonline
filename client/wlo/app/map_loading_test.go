package app

import (
	"bytes"
	"os"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/cursor"
)

func TestMapLoadingWaitsForReadinessWithoutDelayingAcknowledgment(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	c.Frame()
	c.dispatch([]byte{12, 0x11, 0x27, 0, 0, 0x2b, 0x27, 0xa2, 2, 0x2b, 4})
	if !c.mapLoading.active || c.mapReady {
		t.Fatal("map change did not start loading presentation")
	}
	packets := sent()
	if len(packets) == 0 || !bytes.Equal(packets[len(packets)-1], []byte{12, 1}) {
		t.Fatal("loading presentation delayed the native map acknowledgement", packets)
	}
	if c.resources.loadingArtwork == nil {
		t.Fatal("existing illustrated loading artwork was not loaded")
	}
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame() // Final-colour covered frame after background decoding.
	*now = now.Add(3 * time.Second)
	c.Frame()
	if c.Cursors.Current != cursor.ShapeMushroom || c.mapLoading.progress >= 1 {
		t.Fatal("loading cursor/progress ignored pending server snapshot", c.Cursors.Current, c.mapLoading.progress)
	}
	if !c.mapLoading.active || !c.mapLoading.closing.IsZero() {
		t.Fatal("loading screen revealed a map before readiness")
	}
	if path := os.Getenv("MAP_LOADING_SNAPSHOT"); path != "" {
		savePNG(t, path, c)
	}
	c.dispatch([]byte{5, 4})
	c.Frame()
	if c.mapLoading.closing.IsZero() {
		t.Fatal("readiness did not release loading illustration")
	}
	*now = now.Add(mapLoadingHide + mapLoadingReveal)
	c.Frame()
	if c.mapLoading.active || c.mapLoading.departing != nil || c.sceneFrozen() {
		t.Fatal("loading presentation retained its snapshot or interaction lock")
	}
	if c.Cursors.Current == cursor.ShapeMushroom {
		t.Fatal("loading cursor remained after completion")
	}
}

func TestMapLoadingBarFillsOnlyAfterReadiness(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Sprites.WaitNative()
	c.startMapLoading()
	c.mapReady = false
	c.drawMapLoading(*now)
	*now = now.Add(mapLoadingLeave + mapLoadingShow)
	c.drawMapLoading(*now)
	if c.mapLoading.progress >= 1 || !c.mapLoading.closing.IsZero() {
		t.Fatal("server wait completed the bar")
	}
	// Independent RGB565 pixels: blue at the start and dark at the pending end.
	if got := c.Screen.Pix[(ScreenHeight-17)*c.Screen.W+26]; got != 0x4d9f {
		t.Fatalf("bar fill: %x", got)
	}
	if got := c.Screen.Pix[(ScreenHeight-17)*c.Screen.W+ScreenWidth-26]; got != 0x1082 {
		t.Fatalf("pending bar end: %x", got)
	}
	c.mapReady = true
	c.drawMapLoading(*now)
	if c.mapLoading.progress != 1 {
		t.Fatal("completed snapshot did not fill bar")
	}
}

func TestMapLoadingFastMapMinimumAndInput(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.Frame()
	c.dispatch([]byte{12, 0x11, 0x27, 0, 0, 0x2b, 0x27, 0xa2, 2, 0x2b, 4})
	c.dispatch([]byte{5, 4})
	// Loading may have taken time before the first frame can be displayed.
	*now = now.Add(2 * time.Second)
	c.Frame()
	c.Sprites.WaitNative()
	c.Frame()
	c.Frame() // One fully prepared frame precedes completion.
	start := *now
	c.GroundClick(300, 350)
	c.WalkKeys(true, false, false, false)
	if c.World.Walking() || c.World.Marker.Shown() {
		t.Fatal("input moved the character behind the loading screen")
	}
	*now = start.Add(mapLoadingLeave + mapLoadingShow + mapLoadingHold)
	c.Frame()
	*now = start.Add(mapLoadingMinimum - time.Millisecond)
	c.Frame()
	if !c.mapLoading.active {
		t.Fatal("fast map skipped minimum visible loading duration")
	}
	*now = now.Add(time.Millisecond)
	c.Frame()
	if c.mapLoading.active {
		t.Fatal("fast map loading lasted longer than its minimum")
	}
	// Same-map placements do not flash an unnecessary loading screen.
	c.dispatch([]byte{12, 0x11, 0x27, 0, 0, 0x2b, 0x27, 0xa2, 2, 0x2b, 4})
	if c.mapLoading.active {
		t.Fatal("same-map position triggered loading")
	}
	c.startMapLoading()
	c.disconnected()
	if c.mapLoading.active || c.mapLoading.departing != nil {
		t.Fatal("disconnect left loading screen over connection message")
	}
}
