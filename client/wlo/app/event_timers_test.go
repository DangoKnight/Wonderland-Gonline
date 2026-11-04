package app

import (
	"bytes"
	"encoding/binary"
	"image"
	"math"
	"testing"
	"time"
)

func TestCapturedCollectionTimerAndMalformedUpdates(t *testing.T) {
	now := time.Unix(1000, 0)
	bad := 0
	c := &Client{Now: func() time.Time { return now }, Unhandled: func([]byte) { bad++ }}
	// Literal server payload, independent of the server packet builder.
	p := []byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64}
	c.dispatch(p)
	if !c.eventTimers[11020].Equal(now.Add(180*time.Second)) || bad != 0 {
		t.Fatal("captured timer rejected", c.eventTimers, bad)
	}
	for _, kind := range []string{"truncated", "extra", "count", "zero ID", "negative", "nan", "infinite", "overflow", "duplicate"} {
		q := append([]byte(nil), p...)
		switch kind {
		case "truncated":
			q = q[:len(q)-1]
		case "extra":
			q = append(q, 0)
		case "count":
			q[2] = 2
		case "zero ID":
			q[3], q[4] = 0, 0
		case "negative":
			binary.LittleEndian.PutUint64(q[5:], math.Float64bits(-1))
		case "nan":
			binary.LittleEndian.PutUint64(q[5:], math.Float64bits(math.NaN()))
		case "infinite":
			binary.LittleEndian.PutUint64(q[5:], math.Float64bits(math.Inf(1)))
		case "overflow":
			binary.LittleEndian.PutUint64(q[5:], math.Float64bits(1e30))
		case "duplicate":
			binary.LittleEndian.PutUint64(q[5:], math.Float64bits(60))
			q[2] = 2
			q = append(q, q[3:]...)
		}
		before := bad
		c.dispatch(q)
		if bad != before+1 || len(c.eventTimers) != 1 || !c.eventTimers[11020].Equal(now.Add(180*time.Second)) {
			t.Fatal("malformed update partially changed timers", kind)
		}
	}
	clear := append([]byte(nil), p...)
	for i := 5; i < len(clear); i++ {
		clear[i] = 0
	}
	c.dispatch(clear)
	if len(c.eventTimers) != 0 {
		t.Fatal("zero timer did not expire")
	}
}

func TestCapturedManualCollectionAcquisitionAndCompletion(t *testing.T) {
	c, now, _ := enteredClient(t)
	c.dispatch(selfPacket(10001, 2, 60000, 1282, 1315, 0, 444444444, 444444444, nil, "Dango"))
	c.mapReady = true
	sent := wire(t, c)
	n := c.World.NPCs[33]
	if n == nil || n.Template != 19070 || !n.Shown || n.Painter == nil {
		t.Fatal("native collection actor not available", n)
	}
	cx, cy := c.World.Camera()
	r := n.Painter.Bounds(n.X-cx, n.Y-cy+n.Info.SpriteDrop(), n.Action)
	var point image.Point
	found := false
	for y := r.Min.Y; y < r.Max.Y && !found; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if c.World.NPCAt(x, y) == n {
				point, found = image.Pt(x, y), true
				break
			}
		}
	}
	if !found {
		t.Fatal("collection actor has no clickable artwork")
	}
	c.GroundClick(point.X, point.Y)
	if packets := sent(); len(packets) != 1 || !bytes.Equal(packets[0], []byte{20, 1, 33, 0}) {
		t.Fatal("collection click did not send native actor request", packets)
	}
	c.dispatch([]byte{6, 2, 1})
	// The captured Vine Grass acquisition, with no actor-flight flag.
	c.dispatch([]byte{23, 6, 76, 156, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	c.dispatch([]byte{66, 2, 1, 12, 43, 0, 0, 0, 0, 0, 128, 102, 64})
	c.dispatch([]byte{20, 10})
	c.eventTick()
	packets := sent()
	if len(packets) != 2 || !bytes.Equal(packets[1], []byte{20, 6}) || !c.held {
		t.Fatal("collection did not acknowledge while held", packets)
	}
	count := 0
	for _, item := range c.InventoryState.Bag {
		if item.ID == 40012 {
			count += int(item.Count)
		}
	}
	if count != 1 || len(c.worldItemFlights) != 0 || !c.eventTimers[11020].Equal(now.Add(180*time.Second)) {
		t.Fatal("collection item, timer or flight mismatch", count)
	}
	c.dispatch([]byte{20, 8})
	if c.held || c.event.active {
		t.Fatal("collection did not unlock")
	}
	c.eventTick()
	if len(sent()) != 2 {
		t.Fatal("collection completion acknowledged twice")
	}
}
