package mall

import (
	"strconv"
	"wonderland-gonline/internal/protocol"
)

const (
	previewStandingFirst    = 8
	previewFacingCount      = 8
	previewDefaultDirection = 13 // Native FUN_002669fc standing action 0x0d.
	previewBattleReadyLeft  = 16
	previewBattleReadyRight = 17
	catalogItemX            = 226
	catalogItemSize         = 40
	nativeItemIconSize      = 32
)

// PreviewAction follows the tab callbacks and FUN_002669fc. Grocery,
// Furniture and Forging request action 0x33 (sitting); Weaponry uses 0x11.
func (f *Form) PreviewAction() int {
	facing := ((f.Direction-previewStandingFirst)%previewFacingCount + previewFacingCount) % previewFacingCount
	switch f.Category {
	case TabWeaponry:
		if facing < previewFacingCount/2 {
			return previewBattleReadyLeft
		}
		return previewBattleReadyRight
	case TabGrocery, TabFurniture, TabForging:
		return protocol.PoseSitFirst + facing
	default:
		return previewStandingFirst + facing
	}
}
func (f *Form) selectTab(category byte) {
	if f.ForgePending || f.State.Busy() {
		return
	}
	f.Category = category
	f.Direction = previewDefaultDirection
	f.Page = 1
	if category == TabForging && f.LoadPictures != nil {
		for _, it := range f.forgeBag() {
			if !it.Empty() {
				f.LoadPictures(strconv.Itoa(int(f.Items[it.ID].Icon)))
			}
		}
	}
	f.Refresh()
}
