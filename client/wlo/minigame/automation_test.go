package minigame

import (
	"encoding/json"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/seui"
)

func TestAutomationMemoryHidesDigitsAndUsesNativeRules(t *testing.T) {
	now := time.Unix(100, 0)
	m := NewMemory(nil)
	m.Rand = func(int) int { return 0 }
	m.Begin(now)
	now = now.Add(arcadeCountdown)
	m.Update(now)
	now = now.Add(memoryPreviewTime)
	m.Update(now)
	info := seui.AutomationDescription(m.AutomationState())
	value := info.Value.(map[string]any)
	for _, card := range value["cards"].([]map[string]any) {
		if _, exists := card["number"]; exists {
			t.Fatal("hidden digit leaked")
		}
	}
	if err := seui.ValidateAutomationAction(info, "choose", json.RawMessage(`0`)); err != nil {
		t.Fatal(err)
	}
	if err := m.AutomationAction("choose", json.RawMessage(`1`), now); err != nil {
		t.Fatal(err)
	}
	if m.Lives != 2 || m.phase != memoryWrong {
		t.Fatal("API bypassed wrong-card penalty")
	}
	if m.AutomationAction("choose", json.RawMessage(`0`), now) == nil {
		t.Fatal("accepted action during feedback")
	}
}

func TestAutomationArcadeDoesNotDuplicatePaidPlay(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewArcade(nil, 1)
	g.Begin(now)
	requests := 0
	g.Send = func([]byte) { requests++ }
	if err := g.AutomationAction("requestPlay", nil, now); err != nil {
		t.Fatal(err)
	}
	if requests != 0 || !g.confirming {
		t.Fatal("skipped payment confirmation")
	}
	if err := g.AutomationAction("confirmPlay", nil, now); err != nil {
		t.Fatal(err)
	}
	if requests != 1 || !g.pending {
		t.Fatal("play not dispatched")
	}
	if g.AutomationAction("confirmPlay", nil, now) == nil || requests != 1 {
		t.Fatal("duplicate paid request")
	}
}

func TestAutomationDodgeballThrowUsesNativeChargeAndCourtAim(t *testing.T) {
	now := time.Unix(100, 0)
	g := NewThrow(nil, []Figure{nil}, 0, []Figure{nil}, false, now)
	g.Begin(now)
	info := seui.AutomationDescription(g.AutomationState())
	if err := seui.ValidateAutomationAction(info, "throw", json.RawMessage(`null`)); err != nil {
		t.Fatal(err)
	}
	if g.AutomationAction("throw", json.RawMessage(`null`), now) == nil {
		t.Fatal("released without charging")
	}
	if err := g.AutomationAction("charge", json.RawMessage(`null`), now); err != nil {
		t.Fatal(err)
	}
	if err := g.AutomationAction("throw", json.RawMessage(`null`), now); err != nil {
		t.Fatal(err)
	}
	if g.down || !g.up || g.me().state != throwCharging {
		t.Fatal("throw did not enter native release sequence")
	}
}
