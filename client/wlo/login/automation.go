package login

import "wonderland-gonline/client/wlo/seui"

func (c *CreateCharacter) AutomationInfo() seui.AutomationInfo {
	value := map[string]any{"step": c.Step, "selected": c.Selected, "points": c.Points, "stats": c.Stats, "waiting": c.Waiting, "nameAccepted": c.NameAccepted, "part": c.Part}
	roles := []map[string]any{}
	for i, r := range creationRoles {
		if i > 0 {
			roles = append(roles, map[string]any{"index": i, "body": r.Type, "head": r.Head, "statBonuses": statBonus[RoleIndex(r.Type, r.Head)]})
		}
	}
	value["roles"] = roles
	value["carouselPosition"] = c.Current
	value["carouselMoving"] = c.Remaining != 0
	if c.Role != nil {
		value["appearance"] = map[string]any{"body": c.Role.Body.Type, "head": c.Role.Body.Head, "element": c.Role.Body.Element, "colors": c.Role.Body.Colors, "edit": c.Role.Body.Edit}
		parts := [partCount][3]byte{}
		for i := range parts {
			offset := partDigits[i+1]
			copy(parts[i][:], c.Role.Body.Colors[offset:offset+len(parts[i])])
		}
		value["colorParts"] = parts
	}
	return seui.AutomationInfo{Kind: "characterCreation", Value: value, Actions: []string{}}
}
