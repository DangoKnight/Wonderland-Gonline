package app

import (
	"bytes"
	"image"
	"slices"
	"testing"
	"time"
)

func TestNativeConstellationAckOwnership(t *testing.T) {
	c, now, _ := enteredClient(t)
	sent := wire(t, c)
	var sounds []string
	c.Env.Sound = func(path string) { sounds = append(sounds, path) }
	c.dispatch([]byte{6, 2, 1})
	c.dispatch([]byte{15, 20, 6, 3})
	run := c.constellation
	if run == nil || run.picture < 0 || run.sky < 0 || run.height != 200 || run.width != 200 {
		t.Fatal("native star artwork/lifecycle missing", run)
	}
	if !slices.Equal(sounds, []string{`Sound\WAV0149.wav`}) {
		t.Fatal("constellation sound missing", sounds)
	}
	c.dispatch(eventFrameOf(13, 1, 3, 0, 6))
	c.eventTick()
	if !c.event.done || len(sent()) != 0 {
		t.Fatal("ACK escaped before constellation animation")
	}
	*now = run.started.Add(run.rise + run.flight + constellationHold - time.Millisecond)
	c.constellationTick(*now)
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	c.drawConstellation(*now)
	if !slices.ContainsFunc(c.Screen.Pix, func(pixel uint16) bool { return pixel != 0 }) {
		t.Fatal("constellation scene rendered empty")
	}
	c.eventTick()
	if len(sent()) != 0 {
		t.Fatal("ACK escaped during constellation hold")
	}
	*now = run.started.Add(run.duration() - time.Millisecond)
	c.constellationTick(*now)
	c.eventTick()
	if c.constellation == nil || len(sent()) != 0 {
		t.Fatal("fade ended early")
	}
	*now = run.started.Add(run.duration())
	c.constellationTick(*now)
	c.eventTick()
	c.eventTick()
	packets := sent()
	if len(packets) != 1 || !bytes.Equal(packets[0], []byte{20, 6}) || c.constellation != nil || !c.constellations[6] || !c.held {
		t.Fatal("completion ACK duplicated or movement unlocked", packets)
	}
	c.dispatch([]byte{20, 8})
	if c.held {
		t.Fatal("server resume did not release hold")
	}
}

func TestNativeConstellationSnapshotsAndCancellation(t *testing.T) {
	c, _, _ := enteredClient(t)
	c.dispatch([]byte{15, 19, 3, 1, 6, 20})
	if !c.constellations[1] || !c.constellations[6] || !c.constellations[20] {
		t.Fatal("star snapshot not applied")
	}
	before := c.constellations
	for _, p := range [][]byte{{15, 19, 2, 6}, {15, 19, 1, 89}, {15, 19, 1, 0}} {
		c.dispatch(p)
		if c.constellations != before {
			t.Fatal("malformed star snapshot partially applied")
		}
	}
	c.dispatch([]byte{15, 20, 6, 3})
	if c.constellation == nil {
		t.Fatal("no cutscene")
	}
	c.dispatch([]byte{20, 8})
	if c.constellation != nil {
		t.Fatal("cancelled event retained cutscene")
	}
	for _, p := range [][]byte{{15, 20}, {15, 20, 0, 3}, {15, 20, 89, 3}, {15, 20, 6, 255}} {
		c.dispatch(p)
		if c.constellation != nil {
			t.Fatal("invalid constellation locked event")
		}
	}
	c.dispatch([]byte{15, 19, 0})
	if c.constellations != ([constellationLimit]bool{}) {
		t.Fatal("empty snapshot did not clear collection")
	}
}
