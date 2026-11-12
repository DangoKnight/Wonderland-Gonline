package seui

import (
	"encoding/json"
	"testing"
	"wonderland-gonline/client/wlo/picdb"
)

func TestAutomationControlGuardsAndText(t *testing.T) {
	env := &Env{Pics: picdb.New()}
	in := &Input{}
	ui := NewManager(env, in)
	form := NewForm(env)
	ui.Add(form)
	form.Show()
	clicks := 0
	b := NewFixedButton(env, form)
	b.OnClick = func() { clicks++ }
	before := *in
	if err := InvokeAutomation(b, "activate", json.RawMessage(`null`)); err != nil || clicks != 1 {
		t.Fatalf("direct callback: %v %d", err, clicks)
	}
	if *in != before {
		t.Fatal("automation synthesized pointer input")
	}
	b.Enabled = false
	if InvokeAutomation(b, "activate", json.RawMessage(`null`)) == nil {
		t.Fatal("disabled callback executed")
	}
	b.Enabled = true
	modal := NewForm(env)
	ui.Add(modal)
	modal.Show()
	ui.SetModal(true, modal)
	if InvokeAutomation(b, "activate", json.RawMessage(`null`)) == nil {
		t.Fatal("background callback escaped modality")
	}
	ui.SetModal(false, modal)
	modal.Hide()
	form.Hide()
	if InvokeAutomation(b, "activate", json.RawMessage(`null`)) == nil {
		t.Fatal("hidden parent callback executed")
	}
	form.Show()
	e := NewEditor(env, form)
	e.SetMaxLen(8)
	e.SetFilter(1)
	changes := 0
	e.OnChange = func() { changes++ }
	if InvokeAutomation(e, "set", json.RawMessage(`"12x"`)) == nil || len(e.Text) != 0 {
		t.Fatal("numeric filter bypassed")
	}
	if err := InvokeAutomation(e, "set", json.RawMessage(`"123"`)); err != nil || string(e.Text) != "123" || changes != 1 {
		t.Fatalf("editor: %s %v", e.Text, err)
	}
	e.Password = true
	info := DescribeAutomation(e)
	if info.Value != nil || !info.Sensitive {
		t.Fatal("password leaked in control state")
	}
	e.ReadOnly = true
	if InvokeAutomation(e, "set", json.RawMessage(`"9"`)) == nil {
		t.Fatal("read-only field changed")
	}
	e.ReadOnly = false
	e.Password = false
	e.SetFilter(3)
	e.SetMaxLen(64)
	raw, _ := json.Marshal("你好世界")
	if err := InvokeAutomation(e, "set", raw); err != nil || DecodeText(e.Text) != "你好世界" {
		t.Fatalf("Big5 edit: %v %q", err, DecodeText(e.Text))
	}
}

func TestAutomationListWithoutHoverAndSchema(t *testing.T) {
	env := &Env{Pics: picdb.New()}
	ui := NewManager(env, &Input{})
	f := NewForm(env)
	ui.Add(f)
	f.Show()
	list := NewSelectText(env, f)
	list.Add([]byte("first"), "", "")
	list.Add([]byte("second"), "", "")
	picked := -1
	list.OnSelect = func(index int) { picked = index }
	if err := InvokeAutomation(list, "select", json.RawMessage(`1`)); err != nil || picked != 1 || list.Selected != 1 {
		t.Fatalf("index selection: %v %d", err, picked)
	}
	for _, raw := range []string{`null`, `-1`, `2`, `1.5`, `"1"`} {
		if InvokeAutomation(list, "select", json.RawMessage(raw)) == nil {
			t.Fatalf("accepted invalid index %s", raw)
		}
	}
	schema := SchemaOf(struct {
		Slot   int
		Item   uint16
		Values [2]int
	}{})
	for _, raw := range []string{`{}`, `{"Slot":1,"Item":2,"Values":[0]}`, `{"Slot":null,"Item":2,"Values":[0,0]}`, `{"Slot":1,"Item":2,"Values":[0,0],"extra":1}`} {
		if ValidateAutomationSchema(json.RawMessage(raw), schema) == nil {
			t.Fatalf("accepted invalid payload: %s", raw)
		}
	}
	if err := ValidateAutomationSchema(json.RawMessage(`{"slot":1,"item":2,"values":[0,0]}`), schema); err != nil {
		t.Fatal(err)
	}
}
