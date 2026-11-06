package sprites

import (
	"testing"
	"time"
)

func TestCloseCancelsQueuedNativeDecode(t *testing.T) {
	m := NewManager(nil, nil)
	// Occupy every decoding slot so the queued operation cannot open a file.
	for range maxNativeDecoders {
		m.nativeSlots <- struct{}{}
	}
	n := &nativeSource{opening: true, path: "must-not-be-opened"}
	a := &Archive{Name: "queued", m: m, native: n}
	m.archives[a.Name] = a
	done := make(chan struct{})
	go func() { a.openNative(n); close(done) }()
	select {
	case <-done:
		t.Fatal("native decode bypassed the occupied concurrency limit")
	case <-time.After(20 * time.Millisecond):
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("closed manager retained queued decoder")
	}
	m.WaitNative()
	m.mu.Lock()
	failed := n.failed
	m.mu.Unlock()
	if !failed || len(m.nativeSlots) != maxNativeDecoders {
		t.Fatal("queued decoder ran or remained pending")
	}
	if err := m.Close(); err != nil {
		t.Fatal("repeated close", err)
	}
}

func TestNativeProgressTracksPendingAndFailedArchives(t *testing.T) {
	m := NewManager(nil, nil)
	defer m.Close()
	n := &nativeSource{opening: true}
	m.archives["pending"] = &Archive{m: m, native: n}
	m.archives["unused"] = &Archive{m: m, native: &nativeSource{}}
	if settled, total := m.NativeProgress(); settled != 0 || total != 1 {
		t.Fatal("pending archive reported ready", settled, total)
	}
	m.mu.Lock()
	n.failed = true // Missing optional originals retain usable painted frames.
	m.mu.Unlock()
	if settled, total := m.NativeProgress(); settled != 1 || total != 1 {
		t.Fatal("fallback archive never settled", settled, total)
	}
	m.WaitNative()
}
