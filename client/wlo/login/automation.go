package login

import "wonderland-gonline/client/wlo/seui"

func (c *CreateCharacter) AutomationInfo() seui.AutomationInfo {
	value := map[string]any{"step": c.Step, "selected": c.Selected, "points": c.Points, "stats": c.Stats, "waiting": c.Waiting, "nameAccepted": c.NameAccepted, "part": c.Part}
	roles := []map[string]any{}
	for i, r := range creationRoles {
		if i > 0 {
			roles = append(roles, map[string]any{"index": i, "body": r.Type, "head": r.Head})
		}
	}
	value["roles"] = roles
	if c.Role != nil {
		value["appearance"] = map[string]any{"body": c.Role.Body.Type, "head": c.Role.Body.Head, "element": c.Role.Body.Element, "colors": c.Role.Body.Colors, "edit": c.Role.Body.Edit}
	}
	return seui.AutomationInfo{Kind: "characterCreation", Value: value, Actions: []string{}}
}
