package game

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLuckyDrawUTCAllowanceAndClockRollback(t *testing.T) {
	now := time.Date(2026, 10, 2, 23, 59, 59, 0, time.UTC)
	var state LuckyDrawState
	for i := byte(0); i < 3; i++ {
		if state.Remaining(now) != 3-i || !state.Consume(now) || state.Day != "2026-10-02" || state.Used != i+1 {
			t.Fatal(state)
		}
	}
	saved := state
	if state.Consume(now) || state != saved || state.Remaining(now) != 0 {
		t.Fatal("fourth draw", state)
	}
	if state.Consume(now.Add(-24*time.Hour)) || state != saved {
		t.Fatal("clock rollback reset", state)
	}
	// Same instant expressed in a local zone still uses UTC midnight.
	tomorrow := now.Add(time.Second).In(time.FixedZone("UTC-3", -3*3600))
	if state.Remaining(tomorrow) != 3 || !state.Consume(tomorrow) || state.Day != "2026-10-03" || state.Used != 1 {
		t.Fatal("UTC reset", state)
	}
	if !state.Consume(tomorrow.Add(3*24*time.Hour)) || state.Used != 1 {
		t.Fatal("missed days accrued", state)
	}
}

func TestLuckyDrawStateJSONAndValidation(t *testing.T) {
	for _, bad := range []LuckyDrawState{{Used: 1}, {Day: "invalid"}, {Day: "2026-02-30"}, {Day: "2026-10-02", Used: 4}} {
		if bad.Valid() {
			t.Fatal(bad)
		}
	}
	var older Character
	if err := json.Unmarshal([]byte(`{"id":10001}`), &older); err != nil || older.LuckyDraw.Remaining(time.Now()) != 3 {
		t.Fatal(err)
	}
	c := Character{LuckyDraw: LuckyDrawState{Day: "2026-10-02", Used: 2}}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Character
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded.LuckyDraw != c.LuckyDraw || c.Clone().LuckyDraw != c.LuckyDraw {
		t.Fatal(err)
	}
}
