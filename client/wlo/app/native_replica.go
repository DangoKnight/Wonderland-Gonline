package app

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"wonderland-gonline/client/wlo/inventory"
	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/sessionstream"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/protocol"
)

const nativeCatchupBudget = 12 * time.Millisecond
const nativeExtrapolationLimit = 250 * time.Millisecond
const nativeCatchupSilenceFrames = 6

type replicaFrame struct {
	sequence uint64
	frame    sessionstream.Frame
}
type nativeReplica struct {
	epoch             string
	clock, renderTime time.Time
	wall              time.Time
	applied, head     uint64
	pending           []replicaFrame
	err               error
	loading           bool
	reportedError     bool
}

func (c *Client) applyNativeEvent(e sessionstream.Event) error {
	switch e.Kind {
	case "network":
		switch login.EventKind(e.Code) {
		case login.Packet:
			c.dispatch(e.Packet)
		case login.StatusData:
			c.Servers.StatusReceived(e.Packet)
		case login.Disconnected:
			c.agentReset()
			c.disconnected()
		case login.ConnectFailed:
			c.agentReset()
			c.connectFailed()
		}
	case "command":
		if e.Command == "server.connect" {
			c.Servers.Hide()
			c.Login.Show()
			return nil
		}
		if e.Command == "remote.configure" {
			var p struct {
				Options inventory.RemoteOptions `json:"options"`
				Start   bool                    `json:"start"`
			}
			if json.Unmarshal(e.Arguments, &p) != nil {
				return fmt.Errorf("invalid replicated Remote options")
			}
			c.remote.options = p.Options
			c.remote.active = p.Start
			c.Inventory.RestoreRemoteOptions(p.Options)
			return nil
		}
		data, _ := json.Marshal(map[string]any{"command": e.Command, "arguments": e.Arguments})
		if _, err := c.AgentSubmit(data); err != nil {
			return err
		}
		c.agentTick()
	case "control":
		if len(e.Path) == 0 {
			return fmt.Errorf("missing replicated control path")
		}
		var node seui.Control
		children := c.UI.Forms
		for _, index := range e.Path {
			if index < 0 || index >= len(children) {
				return fmt.Errorf("replicated control path no longer exists")
			}
			node = children[index]
			children = node.Base().Children
		}
		var p struct {
			Action string          `json:"action"`
			Value  json.RawMessage `json:"value"`
		}
		if json.Unmarshal(e.Arguments, &p) != nil {
			return fmt.Errorf("invalid control event")
		}
		// The host already applied eligibility and revision checks. Rendering an
		// accepted action uses the local control's normal domain callback.
		_ = seui.InvokeAutomation(node, p.Action, p.Value)
	case "input":
		var p AgentHostRequest
		if json.Unmarshal(e.Arguments, &p) != nil {
			return fmt.Errorf("invalid replicated input")
		}
		_ = c.agentViewerInput(p)
	case "clearInput":
		c.agentClearInput()
	case "walk":
		var p [2]int
		if json.Unmarshal(e.Arguments, &p) != nil {
			return fmt.Errorf("invalid walk event")
		}
		if c.World != nil {
			c.walkWaterAware(p[0], p[1], c.Now())
			c.World.MarkWalk()
		}
	case "choice":
		if len(e.Packet) >= 4 {
			if c.hotbar.submitted == nil {
				c.hotbar.submitted = map[[2]byte]bool{}
			}
			c.hotbar.submitted[[2]byte{e.Packet[2], e.Packet[3]}] = true
		}
	case "flags":
		var flags nativeFlags
		if json.Unmarshal(e.Arguments, &flags) != nil {
			return fmt.Errorf("invalid presentation flags")
		}
		c.battle.auto = flags.Auto
		c.remote.active = flags.Remote
	case "afkState":
		var state protocol.AFKState
		if json.Unmarshal(e.Arguments, &state) != nil {
			return fmt.Errorf("invalid AFK presentation state")
		}
		c.applyAFKState(state)
	default:
		return fmt.Errorf("unknown native presentation event %q", e.Kind)
	}
	return nil
}

func (c *Client) queueNativeBatch(batch sessionstream.Batch) error {
	if batch.Version != sessionstream.Version || batch.Error != "" {
		return fmt.Errorf("native presentation unavailable: %s", batch.Error)
	}
	r := c.replica
	if r.epoch == "" {
		r.epoch = batch.Epoch
		r.clock = batch.Start
		r.renderTime = batch.Start
		c.Now = func() time.Time { return r.renderTime }
	} else if r.epoch != batch.Epoch {
		return fmt.Errorf("hosted presentation session changed; attach with a fresh ticket")
	}
	r.head = batch.Head
	appendFrames := func(first uint64, frames []sessionstream.Frame) {
		for i, frame := range frames {
			sequence := first + uint64(i)
			if sequence > r.applied {
				r.pending = append(r.pending, replicaFrame{sequence, frame})
			}
		}
	}
	for _, chunk := range batch.Chunks {
		frames, err := sessionstream.Decode(chunk)
		if err != nil {
			return err
		}
		appendFrames(chunk.First, frames)
	}
	appendFrames(batch.TailFirst, batch.Tail)
	return nil
}
func (c *Client) renderNativeSpectator(draw bool) {
	r := c.replica
	// Consume one bounded batch at a time. Backpressure retains all actions;
	// reconnection resumes from the last applied frame, never received bytes.
	if len(r.pending) == 0 {
		if batch, ok := c.spectator.PollNative(); ok {
			r.err = c.queueNativeBatch(batch)
		}
	}
	if r.err != nil {
		if !r.reportedError {
			log.Printf("native spectator: %v", r.err)
			r.reportedError = true
		}
		c.spectator.ReportNativeError(r.err.Error())
		return
	}
	deadline := time.Now().Add(nativeCatchupBudget)
	r.loading = r.applied == 0 || r.head > r.applied+nativeCatchupSilenceFrames
	c.replaying = true
	defer func() { c.replaying = false }()
	for len(r.pending) > 0 && time.Now().Before(deadline) {
		next := r.pending[0]
		r.pending = r.pending[1:]
		if next.sequence <= r.applied {
			continue
		}
		if next.sequence != r.applied+1 || next.frame.Delta < 0 {
			r.err = fmt.Errorf("native presentation sequence gap")
			break
		}
		r.clock = r.clock.Add(time.Duration(next.frame.Delta))
		if r.clock.After(r.renderTime) {
			r.renderTime = r.clock
		}
		for _, event := range next.frame.Events {
			if err := c.applyNativeEvent(event); err != nil {
				r.err = err
				break
			}
		}
		if r.err != nil {
			break
		}
		c.frame(false)
		r.applied = next.sequence
		c.spectator.Acknowledge(r.applied, r.epoch)
		r.wall = time.Now()
	}
	if r.err != nil {
		if !r.reportedError {
			log.Printf("native spectator: %v", r.err)
			r.reportedError = true
		}
		c.spectator.ReportNativeError(r.err.Error())
		return
	}
	if r.applied == 0 {
		return
	}
	// The ordinary renderer/animation clocks run every local client tick,
	// independently of network delivery cadence. Bound prediction on a stall.
	elapsed := time.Since(r.wall)
	if elapsed > nativeExtrapolationLimit {
		elapsed = nativeExtrapolationLimit
	}
	if target := r.clock.Add(elapsed); target.After(r.renderTime) {
		r.renderTime = target
	}
	c.frame(draw)
	r.loading = false
}
