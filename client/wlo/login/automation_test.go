package login

import (
	"encoding/json"
	"testing"
	"time"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
)

func TestCreationAutomationCarouselReachesOffscreenPortrait(t *testing.T) {
	env := &seui.Env{Pics: picdb.New()}
	seui.NewManager(env, &seui.Input{})
	c := NewCreateCharacter(env, NewGlobals(), NewNet(), NewAssets(t.TempDir()), 0)
	env.UI.Add(c)
	now := time.Unix(100, 0)
	c.Now = func() time.Time { return now }
	// Supply portrait dimensions without requiring installed image assets.
	for _, button := range c.RolePics[1:] {
		button.Width, button.Height = 64, 64
	}
	c.Open(false)
	if c.RolePics[2].Visible {
		t.Fatal("fixture portrait should start outside the carousel viewport")
	}
	for steps := 0; steps < rolePicCount && !c.RolePics[2].Visible; steps++ {
		if err := seui.InvokeAutomation(c.NextChar, "activate", json.RawMessage("null")); err != nil {
			t.Fatal(err)
		}
		if c.AutomationInfo().Value.(map[string]any)["carouselMoving"] != (c.Remaining != 0) {
			t.Fatal("missing carousel motion state")
		}
		for ticks := 0; ticks < 20 && c.Remaining != 0; ticks++ {
			now = now.Add(time.Second)
			env.UI.Tick() // Headless sessions tick forms without drawing them.
		}
	}
	if !c.RolePics[2].Visible {
		t.Fatal("native carousel failed to reveal the requested portrait")
	}
	if err := seui.InvokeAutomation(c.RolePics[2], "activate", json.RawMessage("null")); err != nil {
		t.Fatal(err)
	}
	if c.Selected != 2 {
		t.Fatal("revealed portrait was not selected")
	}
}

func TestCreationAutomationChoicesReflectNativeCallbacks(t *testing.T) {
	env := &seui.Env{Pics: picdb.New()}
	c := NewCreateCharacter(env, NewGlobals(), NewNet(), NewAssets(t.TempDir()), 0)
	c.SetVisible(true)
	activate := func(control seui.Control) {
		control.Base().SetVisible(true)
		if err := seui.InvokeAutomation(control, "activate", json.RawMessage("null")); err != nil {
			t.Fatal(err)
		}
	}
	c.Step = 1
	activate(c.RolePics[6])
	c.Step = 2
	activate(c.StatArrows[2])
	activate(c.StatArrows[10])
	c.Step = 3
	activate(c.Elements[4])
	c.Step = 4
	activate(c.Parts[1])
	activate(c.ColorArrows[1])
	activate(c.ColorArrows[4])
	activate(c.ColorArrows[5])
	info := c.AutomationInfo().Value.(map[string]any)
	roles := info["roles"].([]map[string]any)
	if len(roles) != 14 || roles[5]["body"] != byte(3) || roles[5]["head"] != byte(0) || roles[5]["statBonuses"] != ([5]uint16{1, 2, 0, 0, 0}) {
		t.Fatal("native portrait catalog or bonuses changed", roles[5])
	}
	if c.Selected != 6 || c.Role.Body.Element != 4 || c.Stats[1] != 1 || c.Stats[5] != 1 || c.Points != 3 {
		t.Fatal("semantic callbacks did not apply chosen character settings")
	}
	parts := info["colorParts"].([4][3]byte)
	if parts != ([4][3]byte{{5, 3, 5}, {4, 4, 4}, {4, 4, 4}, {4, 4, 4}}) {
		t.Fatal("native tint state was not exposed in wizard part order", parts)
	}
	if c.Role.Body.Colors != c.Role.Portrait.Colors {
		t.Fatal("portrait colors differ from body preview")
	}
}
