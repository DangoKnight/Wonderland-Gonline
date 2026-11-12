package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/sessionstream"
	"wonderland-gonline/client/wlo/seui"
)

type nativeRecording struct {
	mu       sync.Mutex
	journal  *sessionstream.Journal
	events   []sessionstream.Event
	depth    int
	frameAt  time.Time
	flags    nativeFlags
	flagsSet bool
}
type nativeFlags struct {
	Auto   bool `json:"auto"`
	Remote bool `json:"remote"`
}

func redactNativeJSON(data []byte) json.RawMessage {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return nil
	}
	var scrub func(any)
	scrub = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for key, item := range v {
				name := strings.ToLower(key)
				if strings.Contains(name, "password") || strings.Contains(name, "secret") {
					v[key] = "00000000"
				} else {
					scrub(item)
				}
			}
		case []any:
			for _, item := range v {
				scrub(item)
			}
		}
	}
	scrub(value)
	out, _ := json.Marshal(value)
	return out
}

func (c *Client) enableNativeStream() {
	if c.native != nil {
		return
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic("native presentation identity unavailable")
	}
	c.native = &nativeRecording{journal: sessionstream.New(hex.EncodeToString(token[:]), c.Now())}
}
func (c *Client) recordNative(e sessionstream.Event) {
	if c.native == nil || c.replica != nil {
		return
	}
	c.native.events = append(c.native.events, e)
}
func (c *Client) beginNativeFrame() {
	if c.native != nil {
		c.native.frameAt = c.Now()
	}
}
func (c *Client) endNativeFrame() {
	if c.native == nil {
		return
	}
	flags := nativeFlags{Auto: c.battle.auto, Remote: c.remote.active}
	if !c.native.flagsSet || flags != c.native.flags {
		data, _ := json.Marshal(flags)
		c.recordNative(sessionstream.Event{Kind: "flags", Arguments: data})
		c.native.flags = flags
		c.native.flagsSet = true
	}
	r := c.native
	r.mu.Lock()
	r.journal.Append(r.frameAt, r.events)
	r.mu.Unlock()
	r.events = nil
}

// AgentNativeStream is safe to query between frames. All blobs are immutable;
// native clients interpret packets/actions locally and never receive JPEGs.
func (c *Client) AgentNativeStream(after uint64) (sessionstream.Batch, error) {
	if err := c.AgentAccessError(); err != nil {
		return sessionstream.Batch{}, err
	}
	if c.native == nil {
		return sessionstream.Batch{}, fmt.Errorf("native presentation stream unavailable")
	}
	r := c.native
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.journal.Batch(after), nil
}
func (c *Client) recordNativeCommand(a agentAction, args map[string]any) {
	if c.native == nil || c.replica != nil || strings.HasPrefix(a.definition.Name, "workspace.") || strings.HasPrefix(a.definition.Name, "display.") {
		return
	}
	if a.definition.Name == "ui.invoke" {
		return
	} // Recorded by the control dispatcher with a stable path.
	copyArgs := make(map[string]any, len(args))
	for k, v := range args {
		copyArgs[k] = v
	}
	for _, field := range a.definition.Arguments {
		if field.Sensitive {
			copyArgs[field.Name] = "00000000"
		}
	}
	data, err := json.Marshal(copyArgs)
	if err != nil {
		return
	}
	c.recordNative(sessionstream.Event{Kind: "command", Command: a.definition.Name, Arguments: redactNativeJSON(data)})
}
func controlPath(roots []seui.Control, target seui.Control) []int {
	for i, node := range roots {
		if node == target {
			return []int{i}
		}
		if path := controlPath(node.Base().Children, target); path != nil {
			return append([]int{i}, path...)
		}
	}
	return nil
}
func (c *Client) recordNativeControl(control seui.Control, action string, value json.RawMessage) {
	if c.native == nil || c.replica != nil {
		return
	}
	if c.agentDescribe(control).Sensitive {
		value = json.RawMessage(`"00000000"`)
	}
	payload, _ := json.Marshal(map[string]any{"action": action, "value": value})
	c.recordNative(sessionstream.Event{Kind: "control", Path: controlPath(c.UI.Forms, control), Arguments: redactNativeJSON(payload)})
}
func (c *Client) recordNativeInput(p AgentHostRequest) {
	if c.native == nil || c.replica != nil {
		return
	}
	p.Viewer = ""
	p.Session = 0
	sensitive := c.Input.Focused != nil && c.agentDescribe(c.Input.Focused).Sensitive
	if sensitive {
		p.Text = strings.Repeat("0", len(p.Text))
		const keyEnter, keyTab uint16 = 0x0d, 0x09
		if p.Key != keyEnter && p.Key != keyTab {
			p.Key = 0
		}
	}
	data, _ := json.Marshal(p)
	c.recordNative(sessionstream.Event{Kind: "input", Arguments: data})
}
func (c *Client) recordNativeNet(event login.Event) {
	if c.native == nil || c.replica != nil {
		return
	}
	c.recordNative(sessionstream.Event{Kind: "network", Code: int(event.Kind), Packet: append([]byte(nil), event.Data...)})
}
