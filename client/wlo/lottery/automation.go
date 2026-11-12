package lottery

import "wonderland-gonline/client/wlo/seui"

func (f *Form) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "luckyDraw", Actions: []string{}, Value: map[string]any{"state": f.State, "remaining": f.State.Remaining(), "canDraw": f.State.Ready && len(f.State.Rewards) > 0 && f.State.Remaining() > 0 && !f.State.Busy() && (f.CanDraw == nil || f.CanDraw())}}
}
func (p *Preview) AutomationInfo() seui.AutomationInfo {
	return seui.AutomationInfo{Kind: "luckyDrawRewards", Actions: []string{}, Value: p.draw.State.Rewards}
}
