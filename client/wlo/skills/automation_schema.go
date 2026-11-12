package skills

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("skills", map[string]any{"actor": byte(0), "tab": 0, "select": uint16(0), "use": uint16(0)})
}
