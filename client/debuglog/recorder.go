package debuglog

import (
	"log"
	"sync"
)

// Recorder routes diagnostics to their owning session without changing the
// process-wide logger. A background socket may report during a log toggle.
type Recorder struct {
	mu      sync.RWMutex
	current *Log
}

func (r *Recorder) SetLog(current *Log) {
	r.mu.Lock()
	r.current = current
	r.mu.Unlock()
}

func (r *Recorder) Printf(format string, args ...any) {
	log.Printf(format, args...)
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current != nil {
		r.current.Printf(format, args...)
	}
}
