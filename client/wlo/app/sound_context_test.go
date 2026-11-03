package app

import (
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

func TestSoundsShareProcessContextAcrossWorkspaces(t *testing.T) {
	first, second := &Sounds{Shared: &SoundCache{}}, &Sounds{Shared: &SoundCache{}}
	var wg sync.WaitGroup
	contexts := make([]*audio.Context, 8)
	for i := range contexts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sounds := first
			if i%2 != 0 {
				sounds = second
			}
			contexts[i] = sounds.Context()
		}()
	}
	wg.Wait()
	for _, ctx := range contexts {
		if ctx == nil || ctx != contexts[0] {
			t.Fatal("workspaces created separate audio contexts")
		}
	}
	if first.Shared == second.Shared {
		t.Fatal("independent workspace caches were merged")
	}
}
