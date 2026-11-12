package worldpk

import "wonderland-gonline/client/wlo/seui"

// The native client currently implements the World PK presentation, with
// matchmaking/reward buttons disabled. Preserve that boundary in the API.
func (f *Form) AutomationInfo() seui.AutomationInfo {
	entries := []map[string]any{}
	for i, e := range f.State.Entries {
		entries = append(entries, map[string]any{"rank": i + 1, "name": seui.DecodeText(e.Name), "level": e.Level, "wins": e.Wins, "badge": e.Badge})
	}
	return seui.AutomationInfo{Kind: "worldPK", Actions: []string{}, Value: map[string]any{"rank": f.State.Rank, "wins": f.State.Wins, "chances": f.State.Chances, "entries": entries, "tab": f.Tab, "page": f.Page, "allowOffline": f.State.AllowOffline, "draftAllow": f.draftAllow, "canStart": f.Start.Enabled && f.Start.OnClick != nil}}
}
