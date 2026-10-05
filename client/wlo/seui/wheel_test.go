package seui

import "testing"

type wheelForm struct {
	FixedForm
	up, down int
}

func (f *wheelForm) WheelUp()   { f.up++ }
func (f *wheelForm) WheelDown() { f.down++ }

func TestWheelRoutesThroughOwnersAndModal(t *testing.T) {
	env := testEnv()
	f := &wheelForm{}
	f.InitFixedForm(f, env)
	child := NewComponent(env, f)
	env.UI.Input.Hovered = child
	if !env.UI.Wheel(true) || !env.UI.Wheel(false) || f.up != 1 || f.down != 1 {
		t.Fatal("wheel did not reach hovered child's owner")
	}
	modal := NewComponent(env, nil)
	env.UI.Modal = modal
	if env.UI.Wheel(true) || f.up != 1 {
		t.Fatal("wheel escaped modal")
	}
	env.UI.Modal = f
	if !env.UI.Wheel(true) || f.up != 2 {
		t.Fatal("modal's own wheel blocked")
	}
	env.UI.Input.Hovered = nil
	if env.UI.Wheel(true) {
		t.Fatal("empty hover consumed wheel")
	}
	env.UI.Modal = nil
	env.UI.Input.Hovered = modal
	if env.UI.Wheel(false) {
		t.Fatal("non-scrollable control consumed wheel")
	}
}
