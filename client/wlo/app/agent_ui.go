package app

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

type AgentControlState struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Enabled  bool   `json:"enabled"`
	seui.AutomationInfo
}
type agentUIRuntime struct {
	prefix   string
	serial   uint64
	controls map[string]seui.Control
	ids      map[seui.Control]string
	labels   map[seui.Control]string
}

// Inspect only handles already issued, avoiding a full form snapshot per frame.
// A control hidden between two API polls must not regain its previous handle.
func (c *Client) agentRetireHiddenControls() {
	if len(c.agentUI.ids) == 0 {
		return
	}
	roots := map[seui.Control]bool{}
	if c.UI != nil {
		for _, root := range c.UI.Forms {
			roots[root] = true
		}
	}
	for control, id := range c.agentUI.ids {
		visible := true
		for node := control; node != nil; node = node.Base().Parent {
			if !node.Base().Visible {
				visible = false
				break
			}
		}
		attached := roots[control.Base().Root()]
		if !visible || !attached {
			delete(c.agentUI.ids, control)
			delete(c.agentUI.controls, id)
			delete(c.agentUI.labels, control)
		}
	}
}

// Handles are scoped to the client and retired when the control is hidden or
// removed. Revisions bind an operation to the exact visible selection/content.
func (c *Client) agentDescribe(control seui.Control) AgentControlState {
	r := &c.agentUI
	if r.controls == nil {
		r.controls = map[string]seui.Control{}
		r.ids = map[seui.Control]string{}
	}
	id := r.ids[control]
	if id == "" {
		if r.prefix == "" {
			var token [16]byte
			if _, err := rand.Read(token[:]); err != nil {
				return AgentControlState{Name: "API identity unavailable", Enabled: false}
			}
			r.prefix = hex.EncodeToString(token[:])
		}
		r.serial++
		id = fmt.Sprintf("%s:%d", r.prefix, r.serial)
		r.ids[control], r.controls[id] = id, control
	}
	d := AgentControlState{ID: id, Name: control.Base().Name, Type: reflect.TypeOf(control).String(), Enabled: seui.AutomationEnabled(control), AutomationInfo: seui.DescribeAutomation(control)}
	if label := r.labels[control]; label != "" {
		d.Name = label
	}
	// The login account is a credential, even though the graphical field is not masked.
	if c.Login != nil && (control == c.Login.Account || control == c.Login.Password) {
		d.Sensitive = true
		d.Value = nil
	}
	raw, err := json.Marshal(d)
	if err != nil {
		d.Enabled = false
		d.Actions = []string{}
		d.Value = nil
		d.Text = "control state is unavailable"
		raw, _ = json.Marshal(d)
	}
	hash := sha256.Sum256(raw)
	d.Revision = hex.EncodeToString(hash[:])
	return d
}

func (c *Client) AgentForms() []AgentFormState {
	out := []AgentFormState{}
	if c.UI == nil {
		return out
	}
	seen := map[seui.Control]bool{}
	for _, f := range c.UI.Forms {
		if !f.Base().Visible {
			continue
		}
		if c.agentUI.labels == nil {
			c.agentUI.labels = map[seui.Control]string{}
		}
		agentControlLabels(f, c.agentUI.labels)
		name := f.Base().Name
		for _, module := range agentModuleNames {
			if form, _ := c.agentModule(module); form == f {
				name = module
				break
			}
		}
		state := AgentFormState{Name: name, Modal: c.UI.Modal == f, Buttons: []AgentButtonState{}, Controls: []AgentControlState{}}
		var visit func(seui.Control)
		visit = func(control seui.Control) {
			if !control.Base().Visible {
				return
			}
			seen[control] = true
			d := c.agentDescribe(control)
			if len(d.Actions) > 0 || d.Text != "" || d.Value != nil {
				state.Controls = append(state.Controls, d)
			}
			if d.Kind == "button" {
				state.Buttons = append(state.Buttons, AgentButtonState{Name: d.Name, Text: d.Text})
			}
			for _, child := range control.Base().Children {
				visit(child)
			}
		}
		visit(f)
		out = append(out, state)
	}
	for control, id := range c.agentUI.ids {
		if !seen[control] {
			delete(c.agentUI.ids, control)
			delete(c.agentUI.controls, id)
			delete(c.agentUI.labels, control)
		}
	}
	return out
}

// Exported form fields supply semantic names such as To, Memo, Confirm,
// Quantities[0], and Tabs[2]. Do not traverse other forms or arbitrary state.
func agentControlLabels(root seui.Control, out map[seui.Control]string) {
	var visit func(reflect.Value, string)
	visit = func(value reflect.Value, path string) {
		if !value.IsValid() {
			return
		}
		if value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return
			}
			if value.CanInterface() {
				if control, ok := value.Interface().(seui.Control); ok {
					if control != root && control.Base().Root() == root {
						out[control] = path
					}
					return
				}
			}
			return
		}
		switch value.Kind() {
		case reflect.Array, reflect.Slice:
			for i := 0; i < value.Len(); i++ {
				visit(value.Index(i), fmt.Sprintf("%s[%d]", path, i))
			}
		case reflect.Struct:
			for i := 0; i < value.NumField(); i++ {
				field := value.Type().Field(i)
				if !field.IsExported() || field.Name == "Children" || field.Name == "Parent" || field.Name == "Env" {
					continue
				}
				name := field.Name
				if path != "" {
					name = path + "." + name
				}
				if field.Anonymous {
					visit(value.Field(i), path)
				} else {
					visit(value.Field(i), name)
				}
			}
		}
	}
	value := reflect.ValueOf(root)
	if value.Kind() == reflect.Pointer {
		visit(value.Elem(), "")
	}
}

func (c *Client) agentInvokeControl(id, revision, action string, value json.RawMessage) error {
	c.AgentForms() // Retire stale handles before resolving, including detached prompts.
	control := c.agentUI.controls[id]
	if control == nil {
		return fmt.Errorf("control handle expired or belongs to another client")
	}
	if current := c.agentDescribe(control); current.Revision != revision {
		return fmt.Errorf("control changed; inspect current state before retrying")
	}
	c.recordNativeControl(control, action, value)
	return seui.InvokeAutomation(control, action, value)
}

var agentModuleNames = []string{"inventory", "skills", "compound", "team", "social", "settings", "mall", "worldPK", "notebook", "mailbox", "business", "luckyDraw"}

func (c *Client) agentExtraMenu(name string, open bool) error {
	if !c.canOpenModule() {
		return agentRejected(false)
	}
	switch name {
	case "mall":
		c.toggleMall()
	case "worldPK":
		c.toggleWorldPK()
	case "business":
		c.toggleBusiness()
	case "luckyDraw":
		c.toggleLuckyDraw()
	case "notebook":
		if open {
			owned := false
			for _, it := range c.InventoryState.Bag {
				if it.ID == game.NotepadItemID && !it.Empty() {
					owned = true
					break
				}
			}
			if !owned {
				return fmt.Errorf("notepad is not owned")
			}
			c.Inventory.OpenNotebook()
		} else {
			c.Notebook.CloseAll()
		}
	case "mailbox":
		c.Mailbox.Toggle()
	}
	form, _ := c.agentModule(name)
	return agentRejected(form.Base().Visible == open)
}
