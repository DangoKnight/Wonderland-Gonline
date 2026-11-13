package login

import (
	"encoding/json"
	"testing"

	"wonderland-gonline/client/wlo/picdb"
	"wonderland-gonline/client/wlo/seui"
)

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
