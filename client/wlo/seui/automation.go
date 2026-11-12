package seui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/traditionalchinese"
)

// Automation is implemented by controls with domain-specific selections. It
// invokes application operations, never pointer events or hit testing.
type Automation interface {
	AutomationInfo() AutomationInfo
	AutomationInvoke(string, json.RawMessage) error
}

type AutomationInfo struct {
	Payloads  map[string]AutomationSchema `json:"payloads,omitempty"`
	Kind      string                      `json:"kind"`
	Asset     string                      `json:"asset,omitempty"`
	Hint      string                      `json:"hint,omitempty"`
	Text      string                      `json:"text,omitempty"`
	Value     any                         `json:"value,omitempty"`
	Actions   []string                    `json:"actions"`
	Sensitive bool                        `json:"sensitive,omitempty"`
	MaxLength int                         `json:"maxLength,omitempty"`
}

func DecodeAutomation(raw json.RawMessage, dst any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || d.Decode(dst) != nil || d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("invalid action parameters")
	}
	return nil
}

func DecodeText(raw []byte) string {
	s, _ := traditionalchinese.Big5.NewDecoder().String(string(raw))
	return s
}

// EncodeText rejects lossy encoding instead of silently changing names/messages.
func EncodeText(value string) ([]byte, error) {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return nil, fmt.Errorf("invalid text")
	}
	s, err := traditionalchinese.Big5.NewEncoder().String(value)
	if err != nil {
		return nil, fmt.Errorf("text is not representable in the client encoding")
	}
	return []byte(s), nil
}

func (c *Component) AutomationEnabled() bool { return c.Enabled }
func AutomationEnabled(c Control) bool {
	for at := c; at != nil; at = at.Base().Parent {
		if !at.Base().Visible {
			return false
		}
		if e, ok := at.(interface{ AutomationEnabled() bool }); ok && !e.AutomationEnabled() {
			return false
		}
	}
	return c.Base().Env == nil || c.Base().Env.UI == nil || c.Base().Env.UI.Modal == nil || c.Base().Root() == c.Base().Env.UI.Modal
}

func DescribeAutomation(c Control) AutomationInfo {
	info := describeAutomation(c)
	if len(info.Payloads) == 0 {
		info.Payloads = map[string]AutomationSchema{}
		for _, action := range info.Actions {
			var sample any
			switch action {
			case "set":
				sample = ""
			case "select", "open", "remove":
				if info.Kind == "list" || info.Kind == "scroll" {
					sample = 0
				}
			}
			info.Payloads[action] = SchemaOf(sample)
		}
	}
	return info
}
func describeAutomation(c Control) AutomationInfo {
	if a, ok := c.(interface{ AutomationInfo() AutomationInfo }); ok {
		info := a.AutomationInfo()
		info.Payloads = AutomationPayloadSchemas(info.Kind)
		return info
	}
	g := c.Base()
	d := AutomationInfo{Kind: "control", Actions: []string{}, Hint: DecodeText(g.HintText)}
	if g.Env != nil && g.Env.Pics != nil && g.Image >= 0 && g.Image < len(g.Env.Pics.Entries) {
		d.Asset = g.Env.Pics.Entries[g.Image].Name
	}
	if e, ok := c.(interface{ AutomationEditor() *Editor }); ok {
		return editorAutomation(e.AutomationEditor())
	}
	switch v := c.(type) {
	case *ComboBox:
		return editorAutomation(&v.Editor)
	case *Editor:
		return editorAutomation(v)
	case *Memo:
		d.Kind, d.Text, d.MaxLength = "memo", DecodeText(v.Text), v.MaxLen
		if !v.ReadOnly {
			d.Actions = append(d.Actions, "set")
		}
	case *SelectText:
		items := []string{}
		for _, item := range v.Items {
			items = append(items, DecodeText(item))
		}
		d.Kind, d.Value = "list", struct {
			Items    []string `json:"items"`
			Selected int      `json:"selected"`
		}{items, v.Selected}
		d.Actions = append(d.Actions, "select")
		if v.OnDblClick != nil {
			d.Actions = append(d.Actions, "open")
		}
		if v.OnRightUp != nil {
			d.Actions = append(d.Actions, "remove")
		}
	case *ScrollButton:
		d.Kind, d.Value = "scroll", map[string]int{"position": v.Pos, "maximum": max(0, v.Total-v.Visible_)}
		d.Actions = append(d.Actions, "select")
	case *FixedButton:
		d.Kind, d.Text, d.Value = "button", DecodeText(v.Caption), v.Toggled
	case *Button:
		d.Kind, d.Text = "button", DecodeText(v.Caption)
	}
	if g.OnClick != nil || g.OnClickTag != nil {
		d.Actions = append(d.Actions, "activate")
	}
	if g.OnRight != nil || g.OnRightTag != nil {
		d.Actions = append(d.Actions, "alternate")
	}
	if g.OnDblTag != nil {
		d.Actions = append(d.Actions, "open")
	}
	if g.OnClick == nil && g.OnClickTag == nil && (g.OnDown != nil || g.OnDownTag != nil) {
		d.Actions = append(d.Actions, "choose")
	}
	return d
}

func (e *Editor) AutomationEditor() *Editor { return e }

func editorAutomation(e *Editor) AutomationInfo {
	d := AutomationInfo{Kind: "editor", Text: DecodeText(e.Label), Sensitive: e.Password, MaxLength: e.MaxLen, Actions: []string{}}
	if !e.Password {
		d.Value = DecodeText(e.Text)
	}
	if !e.ReadOnly {
		d.Actions = append(d.Actions, "set")
	}
	if e.OnEnter != nil {
		d.Actions = append(d.Actions, "submit")
	}
	if e.OnBlur != nil {
		d.Actions = append(d.Actions, "commit")
	}
	if e.OnClick != nil || e.OnClickTag != nil {
		d.Actions = append(d.Actions, "activate")
	}
	return d
}

func InvokeAutomation(c Control, action string, value json.RawMessage) error {
	if !AutomationEnabled(c) {
		return fmt.Errorf("control is hidden, disabled or blocked")
	}
	if !slices.Contains(DescribeAutomation(c).Actions, action) {
		return fmt.Errorf("control does not support this action")
	}
	if err := validateAutomationPayload(DescribeAutomation(c), action, value); err != nil {
		return err
	}
	if a, ok := c.(Automation); ok {
		return a.AutomationInvoke(action, value)
	}
	if e, ok := c.(interface{ AutomationEditor() *Editor }); ok {
		return invokeEditor(e.AutomationEditor(), action, value)
	}
	if e, ok := c.(*Editor); ok {
		return invokeEditor(e, action, value)
	}
	if e, ok := c.(*ComboBox); ok {
		return invokeEditor(&e.Editor, action, value)
	}
	if m, ok := c.(*Memo); ok && action == "set" {
		var text string
		if json.Unmarshal(value, &text) != nil {
			return fmt.Errorf("text string required")
		}
		raw, err := EncodeText(text)
		if err != nil {
			return err
		}
		if m.MaxLen > 0 && len(raw) > m.MaxLen {
			return fmt.Errorf("text exceeds field limit")
		}
		m.SetText(raw)
		if m.OnChange != nil {
			m.OnChange()
		}
		return nil
	}
	if l, ok := c.(*SelectText); ok && (action == "select" || action == "open" || action == "remove") {
		var index int
		if json.Unmarshal(value, &index) != nil || index < 0 || index >= len(l.Items) {
			return fmt.Errorf("invalid list index")
		}
		if action == "select" {
			if l.Selected == index && l.Untoggle {
				l.Selected = -1
			} else {
				l.Selected = index
			}
			if l.OnSelect != nil {
				l.OnSelect(index)
			}
		} else if action == "open" {
			l.OnDblClick(index)
		} else {
			l.OnRightUp(index)
		}
		return nil
	}
	if s, ok := c.(*ScrollButton); ok && action == "select" {
		var pos int
		if json.Unmarshal(value, &pos) != nil || pos < 0 || pos > max(0, s.Total-s.Visible_) {
			return fmt.Errorf("invalid scroll position")
		}
		s.SetPos(pos)
		return nil
	}
	return invokeCallback(c, action)
}

func invokeEditor(e *Editor, action string, value json.RawMessage) error {
	switch action {
	case "set":
		var text string
		if json.Unmarshal(value, &text) != nil {
			return fmt.Errorf("text string required")
		}
		raw, err := EncodeText(text)
		if err != nil {
			return err
		}
		if e.Limit && len(raw) > e.MaxLen {
			return fmt.Errorf("text exceeds field limit")
		}
		for i := 0; i < len(raw); i++ {
			b := raw[i]
			if b >= 0x80 && e.Filter != 1 && e.Filter != 2 {
				i++
				continue
			}
			if b < ' ' || b == 0x7f || e.Filter == 1 && !isDigit(b) || e.Filter == 2 && !isAlnum(b) || e.Name == "CreateRoleName" && b == ' ' {
				return fmt.Errorf("text violates field filter")
			}
			if e.Filter == 3 && b < 0x80 && !isAlnum(b) {
				return fmt.Errorf("text violates field filter")
			}
		}
		e.SetText(raw)
		if e.OnChange != nil {
			e.OnChange()
		}
	case "submit":
		e.OnEnter()
	case "commit":
		e.OnBlur()
	default:
		return invokeCallback(e, action)
	}
	return nil
}

func invokeCallback(c Control, action string) error {
	g := c.Base()
	switch action {
	case "choose":
		if g.OnDown != nil {
			g.OnDown()
		}
		if g.OnDownTag != nil {
			g.OnDownTag(g.Tag)
		}
	case "activate":
		if g.OnClick != nil {
			g.OnClick()
		}
		if g.OnClickTag != nil {
			g.OnClickTag(g.Tag)
		}
		if b, ok := c.(*FixedButton); ok && b.Toggle {
			b.Toggled = !b.Toggled
		}
	case "alternate":
		if g.OnRight != nil {
			g.OnRight()
		}
		if g.OnRightTag != nil {
			g.OnRightTag(g.Tag)
		}
	case "open":
		if g.OnDblTag != nil {
			g.OnDblTag(g.Tag)
		}
	default:
		return fmt.Errorf("unsupported action")
	}
	return nil
}
