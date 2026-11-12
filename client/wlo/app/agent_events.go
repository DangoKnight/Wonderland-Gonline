package app

import "time"

// Packet observations carry no payload, credentials or inferred success. The
// sequence lets an orchestrator distinguish fresh server traffic from old state.
type AgentEvent struct {
	Sequence uint64    `json:"sequence"`
	At       time.Time `json:"at"`
	Kind     string    `json:"kind"`
	Action   byte      `json:"action"`
	Code     byte      `json:"code"`
	Bytes    int       `json:"bytes"`
}

func (c *Client) agentObservePacket(p []byte) {
	if len(p) == 0 || !c.agent.observe.Load() {
		return
	}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	event := AgentEvent{Sequence: r.sequence, At: c.Now(), Kind: "serverPacket", Action: p[0], Bytes: len(p)}
	if len(p) > 1 {
		event.Code = p[1]
	}
	if len(r.events) == agentHistoryLimit {
		copy(r.events, r.events[1:])
		r.events = r.events[:len(r.events)-1]
	}
	r.events = append(r.events, event)
}
func (c *Client) AgentEvents() []AgentEvent {
	r := &c.agent
	r.observe.Store(true)
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]AgentEvent{}, r.events...)
}
