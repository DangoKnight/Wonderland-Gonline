package settings

import "wonderland-gonline/client/wlo/seui"

func init() {
	seui.RegisterAutomation("settings", map[string]any{"local": Local{}, "togglePermission": byte(0)})
}
