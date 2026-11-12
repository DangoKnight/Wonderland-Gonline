package app

// The host owns the game connection. Viewers only receive presentation and
// submit input through the same serialized dispatcher as semantic API actions.
import (
	"bytes"
	"fmt"
	"image/jpeg"
	"time"

	"wonderland-gonline/client/wlo/sessionstream"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/clientassets"
)

const (
	agentViewerLease         = 5 * time.Second
	agentViewerFrameInterval = 100 * time.Millisecond
	agentViewerJPEGQuality   = 80
	agentViewerTextLimit     = 1024
)

type AgentHostControl struct {
	Viewers    int    `json:"viewers"`
	Owner      string `json:"owner"`
	Viewer     string `json:"viewer,omitempty"`
	Generation uint64 `json:"generation"`
}
type AgentHostRequest struct {
	Session    int    `json:"session"`
	Operation  string `json:"operation"`
	Viewer     string `json:"viewer"`
	Generation uint64 `json:"generation"`
	Sequence   uint64 `json:"sequence,omitempty"`
	Kind       string `json:"kind,omitempty"`
	X          int    `json:"x,omitempty"`
	Y          int    `json:"y,omitempty"`
	Button     byte   `json:"button,omitempty"`
	Key        uint16 `json:"key,omitempty"`
	Shift      byte   `json:"shift,omitempty"`
	Down       bool   `json:"down,omitempty"`
	Text       string `json:"text,omitempty"`
}
type AgentHostFrame struct {
	Error    string           `json:"error,omitempty"`
	Sequence uint64           `json:"sequence"`
	JPEG     []byte           `json:"jpeg,omitempty"`
	Control  AgentHostControl `json:"control"`
}

func (c *Client) AgentEnableSessionHost() {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hosted {
		return
	}
	r.hosted = true
	if c.Net != nil {
		c.Net.SetOutboundPolicy(c.agentNetworkAccess)
	}
	r.control = AgentHostControl{Owner: "api", Generation: 1}
	c.enableNativeStream()
}
func (c *Client) agentHosted() bool {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hosted
}
func (c *Client) AgentHostState() AgentHostControl {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.control
}
func (c *Client) agentViewerOwns() bool { return c.AgentHostState().Owner == "viewer" }

func (c *Client) AgentHostSubmit(p AgentHostRequest) (AgentCommandResult, error) {
	if p.Viewer == "" || len(p.Viewer) > 128 || len(p.Text) > agentViewerTextLimit {
		return AgentCommandResult{}, fmt.Errorf("invalid viewer request")
	}
	switch p.Operation {
	case "attach", "takeover", "release", "detach", "heartbeat", "input":
	default:
		return AgentCommandResult{}, fmt.Errorf("unknown viewer operation")
	}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.hosted || p.Session < 0 || p.Session != 0 && p.Session != r.session {
		return AgentCommandResult{}, fmt.Errorf("hosted session unavailable")
	}
	if len(r.queue) >= agentQueueLimit {
		return AgentCommandResult{}, fmt.Errorf("command queue full")
	}
	r.serial++
	now := c.Now()
	result := AgentCommandResult{ID: fmt.Sprintf("viewer:%d:%d", r.session, r.serial), Command: "host." + p.Operation, Status: "accepted", QueuedAt: now}
	r.queue = append(r.queue, agentQueuedCommand{host: &p, result: result, deadline: now.Add(agentDefaultTimeout)})
	return result, nil
}

// Called only from the owning game loop, between complete actions.
func (c *Client) agentHostExecute(p AgentHostRequest) error {
	if p.Operation == "takeover" || p.Operation == "input" {
		if err := c.AgentAccessError(); err != nil {
			return err
		}
	}
	r := &c.agent
	r.mu.Lock()
	state := r.control
	if p.Operation == "attach" {
		if r.viewers == nil {
			r.viewers = map[string]bool{}
		}
		r.viewers[p.Viewer] = true
		r.control.Viewers = len(r.viewers)
		r.mu.Unlock()
		return nil
	}
	if p.Operation == "detach" {
		delete(r.viewers, p.Viewer)
		r.control.Viewers = len(r.viewers)
	}
	if p.Operation == "detach" && state.Viewer != p.Viewer {
		r.mu.Unlock()
		return nil
	}
	if p.Generation != state.Generation && p.Operation != "detach" {
		r.mu.Unlock()
		return fmt.Errorf("stale control generation")
	}
	if p.Operation == "takeover" {
		if state.Owner != "api" {
			r.mu.Unlock()
			return fmt.Errorf("another viewer owns input")
		}
		if c.afk.pending || c.afk.resync {
			r.mu.Unlock()
			return fmt.Errorf("AFK state is synchronizing; retry takeover when ready")
		}
		if c.afk.active {
			// Network callbacks may enqueue requests, so never hold the
			// admission mutex while requesting the full scene from the server.
			r.mu.Unlock()
			if err := c.SetAFK(false); err != nil {
				return err
			}
			r.mu.Lock()
		}
	} else if state.Owner != "viewer" || state.Viewer != p.Viewer {
		r.mu.Unlock()
		return fmt.Errorf("viewer does not own input")
	}
	if p.Operation == "heartbeat" {
		r.lease = c.Now().Add(agentViewerLease)
		r.mu.Unlock()
		return nil
	}
	if p.Operation == "input" {
		if p.Sequence <= r.viewerSequence {
			r.mu.Unlock()
			return fmt.Errorf("stale input sequence")
		}
		r.viewerSequence = p.Sequence
		r.mu.Unlock()
		c.recordNativeInput(p)
		if c.native != nil {
			c.native.depth++
			defer func() { c.native.depth-- }()
		}
		return c.agentViewerInput(p)
	}
	r.control.Generation++
	r.control.Owner = "api"
	r.control.Viewer = ""
	if p.Operation == "takeover" {
		r.control.Owner = "viewer"
		r.control.Viewer = p.Viewer
		r.lease = c.Now().Add(agentViewerLease)
	}
	r.viewerSequence = 0
	// Queued API work is never replayed after a human intervention.
	kept := r.queue[:0]
	for _, q := range r.queue {
		if q.host != nil {
			kept = append(kept, q)
		} else {
			q.result.Status = "cancelled"
			q.result.Error = "input ownership changed"
			r.retain(q.result)
		}
	}
	clear(r.queue[len(kept):])
	r.queue = kept
	r.mu.Unlock()
	c.agentClearInput()
	return nil
}
func (c *Client) agentClearInput() {
	c.recordNative(sessionstream.Event{Kind: "clearInput"})
	r := &c.agent
	for key := range r.viewerKeys {
		if !c.SportKey(int(key), false) && c.UI != nil {
			c.UI.KeyUp(key, 0)
		}
	}
	r.viewerKeys = nil
	r.viewerHeld = false
	r.viewerGame = nil
	c.cancelSessionInput()
	c.cancelFieldSelection()
	c.pendingNPC = nil
	c.floorPendingPickup = 0
	c.tentPending = 0
	c.waterTravel.goal = nil
	c.stopRemote()
	c.stopVacuum()
	c.battle.auto = false
	if c.World != nil {
		c.World.StopWalk()
	}
}
func (c *Client) agentHostTick() {
	r := &c.agent
	r.mu.Lock()
	state, expired := r.control, r.control.Owner == "viewer" && !c.Now().Before(r.lease)
	r.mu.Unlock()
	if expired {
		_ = c.agentHostExecute(AgentHostRequest{Operation: "release", Viewer: state.Viewer, Generation: state.Generation})
		return
	}
	if state.Owner == "viewer" || c.replica != nil && c.replaying {
		if r.viewerHeld {
			c.GroundHold(true, r.viewerX, r.viewerY)
		}
		const keyLeft, keyUp, keyRight, keyDown uint16 = 0x25, 0x26, 0x27, 0x28
		c.WalkKeys(r.viewerKeys[keyLeft], r.viewerKeys[keyUp], r.viewerKeys[keyRight], r.viewerKeys[keyDown])
	}
}

func (c *Client) agentViewerInput(p AgentHostRequest) error {
	if err := c.AgentAccessError(); err != nil {
		return err
	}
	r := &c.agent
	if c.afk.active || c.afk.pending || c.afk.resync {
		return fmt.Errorf("waiting for full scene synchronization")
	}
	if c.UI == nil {
		return fmt.Errorf("UI unavailable")
	}
	if r.viewerGame == nil {
		r.viewerGame = &Game{C: c}
	}
	if p.X < 0 || p.X >= ScreenWidth || p.Y < 0 || p.Y >= ScreenHeight {
		if p.Kind == "move" || p.Kind == "down" || p.Kind == "wheel" {
			return fmt.Errorf("pointer outside scene")
		}
	}
	switch p.Kind {
	case "move":
		r.viewerX, r.viewerY = p.X, p.Y
		c.UI.MouseMove(p.Shift, p.X, p.Y, r.viewerHeld)
	case "down":
		r.viewerX, r.viewerY = p.X, p.Y
		if p.Button == 0 {
			r.viewerHeld = true
			r.viewerGame.leftPress(p.Shift|shiftLeft, p.X, p.Y)
		} else if p.Button == 1 {
			r.viewerGame.rightPress(p.Shift|shiftRight, p.X, p.Y)
		} else {
			return fmt.Errorf("invalid button")
		}
	case "up":
		if p.Button == 0 {
			r.viewerHeld = false
			c.GroundHold(false, p.X, p.Y)
			if c.Social != nil && c.Social.Drop(p.X, p.Y) || c.dropHotbar(p.X, p.Y) {
				c.Input.Pressed = nil
				c.Input.Captured = nil
			} else {
				c.UI.MouseUp(seui.ButtonLeft, p.Shift, p.X, p.Y)
			}
		} else if p.Button == 1 {
			c.UI.MouseUp(seui.ButtonRight, p.Shift, p.X, p.Y)
		} else {
			return fmt.Errorf("invalid button")
		}
	case "wheel":
		if !c.scrollWheel(p.Down) {
			c.zoomWorld(p.Down)
		}
	case "key":
		if r.viewerKeys == nil {
			r.viewerKeys = map[uint16]bool{}
		}
		r.viewerKeys[p.Key] = p.Down
		if p.Down {
			if !c.SportKey(int(p.Key), true) && !c.EmoteKey(p.Key, p.Shift) && !c.HotbarKey(p.Key, p.Shift) && !c.TeamKey(p.Key, p.Shift) && !c.CompoundKey(p.Key, p.Shift) && !c.SkillsKey(p.Key, p.Shift) && !c.LuckyDrawKey(p.Key) && !c.WorldPKKey(p.Key) && !c.MallKey(p.Key) && !c.SettingsKey(p.Key) && !c.InventoryKey(p.Key) {
				c.UI.KeyDown(p.Key, p.Shift)
			}
		} else if !c.SportKey(int(p.Key), false) {
			c.UI.KeyUp(p.Key, p.Shift)
		}
	case "text":
		for _, b := range clientassets.Big5Text(p.Text) {
			c.UI.Char(b)
		}
	case "reset":
		c.agentClearInput()
	default:
		return fmt.Errorf("unknown input kind")
	}
	return nil
}

func (c *Client) AgentViewerFrame() AgentHostFrame {
	if err := c.AgentAccessError(); err != nil {
		return AgentHostFrame{Error: err.Error()}
	}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wantFrame = true
	out := r.frame
	out.Control = r.control
	return out
}
func (c *Client) agentFrameRequested() bool {
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wantFrame && !c.Now().Before(r.frameAt.Add(agentViewerFrameInterval))
}
func (c *Client) agentCaptureFrame() {
	if !c.agentFrameRequested() {
		return
	}
	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, c.Screen.RGBA(), &jpeg.Options{Quality: agentViewerJPEGQuality}); err != nil {
		return
	}
	r := &c.agent
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frame.Sequence++
	r.frame.JPEG = buffer.Bytes()
	r.frameAt = c.Now()
	r.wantFrame = false
}
