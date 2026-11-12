package inventory

import (
	"encoding/json"
	"fmt"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/internal/game"
)

func (d *remoteForm) AutomationInfo() seui.AutomationInfo {
	o, err := d.options()
	problem := ""
	if err != nil {
		problem = err.Error()
	}
	return seui.AutomationInfo{Kind: "remote", Actions: []string{"configure", "start", "stop", "advanced"}, Value: map[string]any{"options": o, "validationError": problem}}
}
func (d *remoteForm) AutomationInvoke(action string, raw json.RawMessage) error {
	switch action {
	case "configure":
		var o RemoteOptions
		if seui.DecodeAutomation(raw, &o) != nil || !o.Valid() {
			return fmt.Errorf("invalid Remote configuration")
		}
		d.restoreOptions(o)
	case "start":
		o, err := d.options()
		if err != nil {
			return fmt.Errorf("invalid Remote configuration")
		}
		if d.owner.StartRemote == nil || !d.owner.StartRemote(o) {
			return fmt.Errorf("Remote cannot start")
		}
		d.Hide()
		if d.Plus != nil {
			d.Plus.Hide()
		}
	case "stop":
		d.stop()
	case "advanced":
		d.openSettings()
	default:
		return fmt.Errorf("unsupported Remote action")
	}
	return nil
}
func (d *remotePlusForm) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "remoteAdvanced", Actions: []string{"configure"}, Value: map[string]any{"tab": d.tab, "draft": d.draft}}
}
func (d *remotePlusForm) AutomationInvoke(action string, raw json.RawMessage) error {
	if action != "configure" {
		return fmt.Errorf("unsupported advanced Remote action")
	}
	var advanced [2]RemoteAdvanced
	if seui.DecodeAutomation(raw, &advanced) != nil {
		return fmt.Errorf("invalid advanced Remote configuration")
	}
	o := RemoteOptions{SupplyFirst: 1, SupplyLast: game.BagSize, Advanced: advanced}
	if !o.Valid() {
		return fmt.Errorf("invalid advanced Remote configuration")
	}
	d.draft = advanced
	d.refreshAdvanced()
	return nil
}
