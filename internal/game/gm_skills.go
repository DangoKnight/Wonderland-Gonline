package game

// Authored command lists from GmManager.UnlockAllSkills; these deliberately
// exclude quest-only and grade evolution skills, rather than granting all SQL rows.
var gmElementSkills = map[byte][]uint16{
	Fire:  {11016, 15101, 11114, 12039, 15102, 15044, 11166, 11005, 11034, 15109, 15111, 11035, 11056, 11002, 11072, 12045, 11003, 15171},
	Earth: {15085, 11017, 11087, 15083, 15049, 15146, 12006, 15056, 11019, 11031, 15086, 11107, 11057, 12043, 12048, 11055, 15070, 15035},
	Water: {15091, 11001, 11044, 15019, 12007, 15156, 15097, 11040, 11110, 11113, 11024, 15158, 15100, 11042, 15075, 11080, 11051, 11043},
	Wind:  {11007, 15079, 15117, 15002, 11046, 15114, 30002, 11015, 15123, 15125, 15048, 15161, 11052, 11073, 12046, 15032, 15036, 11026},
}
var gmFallbackElementSkills = []uint16{25115, 25116, 25110, 25165, 25169, 25175, 25185, 25246, 25247, 25248}

func (c Character) GMElementSkillIDs() []uint16 {
	ids := gmElementSkills[c.Element]
	if ids == nil {
		ids = gmFallbackElementSkills
	}
	return append([]uint16{StarterStunt(c.Body, c.Head)}, ids...)
}
