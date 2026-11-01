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
