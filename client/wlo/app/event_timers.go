package app

import (
	"encoding/binary"
	"math"
	"time"
	"wonderland-gonline/internal/protocol"
)

// eventTimerPacket accepts counted AC66:2 records atomically. These are a local
// presentation cache, not permission to grant items or suppress server clicks.
func (c *Client) eventTimerPacket(p []byte) {
	invalid := func() {
		if c.Unhandled != nil {
			c.Unhandled(p)
		}
	}
	if len(p) < protocol.EventTimerHeaderBytes || len(p) != protocol.EventTimerHeaderBytes+int(p[2])*protocol.EventTimerRecordBytes {
		invalid()
		return
	}
	now := c.Now()
	updates := make(map[uint16]time.Time, int(p[2]))
	for at := protocol.EventTimerHeaderBytes; at < len(p); at += protocol.EventTimerRecordBytes {
		id := binary.LittleEndian.Uint16(p[at:])
		seconds := math.Float64frombits(binary.LittleEndian.Uint64(p[at+2:]))
		_, duplicate := updates[id]
		if id == 0 || duplicate || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt64)/float64(time.Second) {
			invalid()
			return
		}
		updates[id] = now.Add(time.Duration(seconds * float64(time.Second)))
	}
	if c.eventTimers == nil {
		c.eventTimers = map[uint16]time.Time{}
	}
	for id, expiry := range updates {
		if expiry.After(now) {
			c.eventTimers[id] = expiry
		} else {
			delete(c.eventTimers, id)
		}
	}
	c.vacuumTimer(updates)
}
