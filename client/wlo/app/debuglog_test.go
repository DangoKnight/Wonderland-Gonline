package app

import (
	"log"
	"os"
	"strings"
	"testing"
)

func TestSessionDebugLogLifecycle(t *testing.T) {
	f := newClientFixture(t)
	first, second := testClient(t, f), testClient(t, f)
	first.screenSuffix, second.screenSuffix = "-screen1", "-screen2"
	writer := log.Writer()
	first.SyncDebugLog(true, 1000)
	second.SyncDebugLog(true, 1000)
	defer first.stopDebugLog()
	defer second.stopDebugLog()
	if first.debugLog == nil || second.debugLog == nil {
		t.Fatal("session log did not start")
	}
	firstPath, secondPath := first.debugLog.Path(), second.debugLog.Path()
	first.logf("first session diagnostic")
	second.logf("second session diagnostic")
	first.stopDebugLog()
	second.logf("second session after first closes")
	second.stopDebugLog()
	if log.Writer() != writer {
		t.Fatal("session lifecycle changed global logger")
	}
	for _, entry := range []struct{ path, own, other string }{{firstPath, "first session diagnostic", "second session"}, {secondPath, "second session after first closes", "first session"}} {
		raw, err := os.ReadFile(entry.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), entry.own) || strings.Contains(string(raw), entry.other) {
			t.Fatalf("incorrect transcript %s", entry.path)
		}
	}
}
