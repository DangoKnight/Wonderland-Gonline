package app

import (
	"testing"
	"time"
)

func voiceTestReceive(t *testing.T, started <-chan string, want string) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("decoded %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("decoder did not start %q", want)
	}
}

func TestVoiceDecodeDoesNotBlockUIOrCachedEffects(t *testing.T) {
	started := make(chan string, 4)
	release := make(chan struct{})
	s := &Sounds{decodePCM: func(path string) []byte {
		if path == "cached.wav" {
			return []byte{1, 2}
		}
		started <- path
		if path == "first.ogg" {
			<-release
		}
		return nil
	}}
	t.Cleanup(s.Close)
	s.PCM("cached.wav")
	returned := make(chan struct{})
	go func() { s.PlayVoice("first.ogg"); close(returned) }()
	select {
	case <-returned:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("voice decoding blocked the caller")
	}
	voiceTestReceive(t, started, "first.ogg")
	cacheRead := make(chan []byte, 1)
	go func() { cacheRead <- s.PCM("cached.wav") }()
	select {
	case pcm := <-cacheRead:
		if len(pcm) != 2 {
			t.Fatal("cached effect lost")
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("background voice decode held the shared cache lock")
	}
	s.PlayVoice("stale.ogg")
	s.PlayVoice("latest.ogg")
	close(release)
	voiceTestReceive(t, started, "latest.ogg")
	s.Close()
	select {
	case <-s.voiceWorkerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("voice worker did not stop")
	}
}

func TestVoiceStopAndCloseCancelPendingPlayback(t *testing.T) {
	started, release := make(chan string, 1), make(chan struct{})
	s := &Sounds{decodePCM: func(path string) []byte {
		started <- path
		<-release
		return make([]byte, 16)
	}}
	s.PlayVoice("pending.ogg")
	voiceTestReceive(t, started, "pending.ogg")
	s.StopVoice()
	s.Close() // Must return while the decoder is still blocked.
	s.PlayVoice("after-close.ogg")
	close(release)
	select {
	case <-s.voiceWorkerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("pending decoder leaked at shutdown")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.voice != nil {
		t.Fatal("canceled recording started after shutdown")
	}
	select {
	case got := <-started:
		t.Fatalf("closed session decoded %q", got)
	default:
	}
}

func TestVoiceStopCancelsInFlightWithoutClosingSession(t *testing.T) {
	started := make(chan string, 2)
	releaseFirst, releaseBarrier := make(chan struct{}), make(chan struct{})
	s := &Sounds{decodePCM: func(path string) []byte {
		started <- path
		if path == "pending.ogg" {
			<-releaseFirst
		} else {
			<-releaseBarrier
		}
		return make([]byte, 16)
	}}
	s.PlayVoice("pending.ogg")
	voiceTestReceive(t, started, "pending.ogg")
	s.StopVoice()
	// A same-revision barrier observes completion without canceling the
	// preceding job itself, as another PlayVoice call would do.
	s.mu.Lock()
	s.voiceJobs <- voiceRequest{"barrier.wav", s.voiceRevision}
	s.mu.Unlock()
	close(releaseFirst)
	voiceTestReceive(t, started, "barrier.wav")
	s.mu.Lock()
	played := s.voice != nil
	s.mu.Unlock()
	s.Close()
	close(releaseBarrier)
	select {
	case <-s.voiceWorkerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("voice worker did not stop")
	}
	if played {
		t.Fatal("dismissed line began playing when its decode completed")
	}
}
