package world

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"wonderland-gonline/client/wlo/login"
)

// warpsJSON lists, per map, the maps its warps lead to. It is exported from
// the EVE warp records (Eve.emg, category 6).
//
//go:embed warps.json
var warpsJSON []byte

var neighborMaps = func() map[uint16][]uint16 {
	raw := map[string][]uint16{}
	if err := json.Unmarshal(warpsJSON, &raw); err != nil {
		return nil
	}
	out := make(map[uint16][]uint16, len(raw))
	for k, v := range raw {
		id, err := strconv.ParseUint(k, 10, 16)
		if err != nil {
			continue
		}
		out[uint16(id)] = v
	}
	return out
}()

// prefetch holds the neighbor scenes waiting to be decoded. They are decoded one
// per frame, so warming a map never stalls a frame for long.
var prefetch = struct {
	sync.Mutex
	queue  []uint16
	queued map[uint16]bool
}{queued: map[uint16]bool{}}

// prefetchRings is how far from the current map the warm-up reaches: direct
// neighbors first, then their neighbors.
const prefetchRings = 0 // prefetch disabled: only the current map is loaded

// PrefetchNeighbors queues the maps reachable by warps from mapID, up to
// prefetchRings steps away, skipping scenes that are already cached. The queue
// survives map changes, so warming continues in the background while playing.
func PrefetchNeighbors(a login.Assets, mapID uint16) {
	prefetch.Lock()
	defer prefetch.Unlock()
	frontier := []uint16{mapID}
	seen := map[uint16]bool{mapID: true}
	for ring := 0; ring < prefetchRings; ring++ {
		var next []uint16
		for _, from := range frontier {
			for _, n := range neighborMaps[from] {
				if seen[n] {
					continue
				}
				seen[n] = true
				next = append(next, n)
				if prefetch.queued[n] || sceneCached(a, n) {
					continue
				}
				prefetch.queued[n] = true
				prefetch.queue = append(prefetch.queue, n)
			}
		}
		frontier = next
	}
}

// sceneCached reports whether a scene is already decoded.
func sceneCached(a login.Assets, mapID uint16) bool {
	key := fmt.Sprintf("%s:%d", a.Data, mapID)
	sceneCache.Lock()
	defer sceneCache.Unlock()
	return sceneCache.entries[key] != nil
}

// prefetchInterval is the pause between two background decodes, and
// prefetchIdle is how long the player must stand still before one starts.
const (
	prefetchInterval = 5 * time.Second
	prefetchIdle     = 3 * time.Second
)

var prefetchTiming struct {
	lastRun   time.Time
	idleSince time.Time
	moving    bool
}

// PrefetchStep decodes at most one queued neighbor scene, and only while the
// player is standing still and the previous decode was a while ago. Each decode
// blocks the frame it runs in, so this keeps the pauses rare and short-spaced.
// Call it once per frame with the player's walker state and the current time.
func PrefetchStep(a login.Assets, walking bool, now time.Time) {
	if walking {
		prefetchTiming.moving = true
		return
	}
	if prefetchTiming.moving {
		prefetchTiming.moving = false
		prefetchTiming.idleSince = now
	}
	if now.Sub(prefetchTiming.idleSince) < prefetchIdle || now.Sub(prefetchTiming.lastRun) < prefetchInterval {
		return
	}
	prefetch.Lock()
	if len(prefetch.queue) == 0 {
		prefetch.Unlock()
		return
	}
	id := prefetch.queue[0]
	prefetch.queue = prefetch.queue[1:]
	delete(prefetch.queued, id)
	prefetch.Unlock()
	prefetchTiming.lastRun = now
	_, _ = LoadScene(a, id)
}
