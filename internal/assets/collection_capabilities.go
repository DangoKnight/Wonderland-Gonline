package assets

// Native EVE condition codes supported by manual collection.
const (
	collectionConditionAlways          = 0
	collectionConditionEmptyOperands   = 1
	collectionConditionQuest           = 5
	collectionConditionAlwaysAlternate = 6
	collectionConditionTimer           = 14
	collectionConditionFreeSlots       = 15
)

func AvailableCollectionEvents(c *Catalog, reasonFilter string) []uint32 {
	var enabled []uint32
	for key, reason := range c.DisabledEvents {
		if reason != reasonFilter {
			continue
		}
		mapID, eventID := uint16(key>>16), uint16(key)
		var ev *Event
		for i := range c.Maps[mapID].Events {
			if c.Maps[mapID].Events[i].ClickID == eventID {
				copy := c.Maps[mapID].Events[i]
				ev = &copy
				break
			}
		}
		exists := ev != nil
		if !exists {
			continue
		}
		valid, rewards := true, 0
		for i, branch := range ev.Branches {
			switch branch.Condition[0] {
			case collectionConditionAlways, collectionConditionEmptyOperands, collectionConditionQuest, collectionConditionAlwaysAlternate, collectionConditionTimer, collectionConditionFreeSlots:
			default:
				valid = false
			}
			plan, involved := c.PlanEventGathering(ev, i)
			if involved {
				if plan.Timer == 0 || len(plan.Pools) == 0 {
					valid = false
				}
				for _, pool := range plan.Pools {
					for _, id := range pool {
						if _, ok := c.Items[id]; !ok {
							valid = false
						}
					}
				}
				rewards++
				continue
			}
			for _, raw := range branch.Operations {
				op := decodeGatheringOp(raw)
				if op.Code != ActionPlayer || op.D1 != PlayerActionSpeech {
					valid = false
					continue
				}
				// Player speech carries a native word ID; the following event
				// byte can be a flag. Match the client's native-first lookup.
				if _, ok := c.Talks.Resolve(uint32(op.D2)); !ok {
					if _, extended := c.Talks.Resolve(op.TalkID()); !extended {
						valid = false
					}
				}
			}
		}
		if valid && rewards > 0 {
			enabled = append(enabled, key)
		}
	}
	return enabled
}
