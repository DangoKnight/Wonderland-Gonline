package game

import "time"

const (
	WaterGatheringTimer11009 uint16 = 11009
	WaterGatheringTimer11016 uint16 = 11016
	WaterGatheringTimer11021 uint16 = 11021
	WaterGatheringTimer11035 uint16 = 11035
	SeaWaterItemID           uint16 = 60001
	FreshWaterItemID         uint16 = 60002
	WaterGatheringCooldown          = 180 * time.Second
)

func IsWaterGatheringTimer(id uint16) bool {
	switch id {
	case WaterGatheringTimer11009, WaterGatheringTimer11016, WaterGatheringTimer11021, WaterGatheringTimer11035:
		return true
	}
	return false
}

func (c Character) EventTimerActive(id uint16, now time.Time) bool {
	return c.EventTimers[id].After(now)
}

// WaterGatheringItem is the native pool-7 outcome for each verified timer.
func WaterGatheringItem(timer uint16) (uint16, bool) {
	switch timer {
	case WaterGatheringTimer11009, WaterGatheringTimer11016:
		return SeaWaterItemID, true
	case WaterGatheringTimer11021, WaterGatheringTimer11035:
		return FreshWaterItemID, true
	}
	return 0, false
}
