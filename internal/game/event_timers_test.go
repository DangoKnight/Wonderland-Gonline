package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventTimersExpiryCloneAndJSON(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	c := Character{ID: 1, Slot: 1, Name: "Alice", Level: 1, HP: 1, MaxHP: 1, EventTimers: map[uint16]time.Time{11009: now.Add(180 * time.Second)}}
	if !c.EventTimerActive(11009, now) || c.EventTimerActive(11009, now.Add(180*time.Second)) || c.EventTimerActive(11016, now) {
		t.Fatal("timer boundary")
	}
	clone := c.Clone()
	clone.EventTimers[11009] = now
	if !c.EventTimerActive(11009, now) {
		t.Fatal("clone aliased timer map")
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Character
	if err := json.Unmarshal(raw, &decoded); err != nil || !decoded.EventTimerActive(11009, now) {
		t.Fatal(string(raw), err)
	}
	decoded.EventTimers[0] = now
	if decoded.Validate() == nil {
		t.Fatal("zero timer accepted")
	}
	delete(decoded.EventTimers, 0)
	decoded.EventTimers[11009] = time.Time{}
	if decoded.Validate() == nil {
		t.Fatal("zero expiration accepted")
	}
	var legacy Character
	if err := json.Unmarshal([]byte(`{"id":1,"slot":1,"name":"Alice","level":1}`), &legacy); err != nil || legacy.EventTimerActive(11009, now) {
		t.Fatal(err)
	}
}
