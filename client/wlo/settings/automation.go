package settings

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
)

func (f *Form) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "settings", Actions: []string{"local", "togglePermission"}, Value: f.State}
}
func (f *Form) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "local":
		var local Local
		if seui.DecodeAutomation(raw, &local) != nil || local.MusicVolume < 0 || local.MusicVolume > VolumeSteps || local.SoundVolume < 0 || local.SoundVolume > VolumeSteps || len(local.Blacklist) > 1000 {
			return fmt.Errorf("invalid settings")
		}
		for _, color := range local.Colors {
			if color < 0 || color >= len(Palette) {
				return fmt.Errorf("invalid chat color")
			}
		}
		for _, name := range local.Blacklist {
			if len(name) > 64 {
				return fmt.Errorf("invalid blacklist name")
			}
		}
		f.State.Local = local
		f.changed()
	case "togglePermission":
		var code byte
		if seui.DecodeAutomation(raw, &code) != nil {
			return fmt.Errorf("permission code required")
		}
		if _, ok := f.Toggles[code]; !ok {
			return fmt.Errorf("unknown permission")
		}
		f.Toggle(code)
	default:
		return fmt.Errorf("unsupported settings action")
	}
	return nil
}
