package game

// Native GachaManager table bounds. Weights are integer slots in a 10,000-slot pool.
const (
	GachaPacksPerOpening   = 1
	GachaWeightTotal       = 10_000
	GachaMaxRewards        = 41
	GachaMaxRewardQuantity = 255
	UnavailableLuckyPackID = 34333
)

// Native fallback IDs from GachaManager.PackIds, including unavailable packs.
// SQL-configured packs are recognized by assets.Catalog.IsGachaPack. The withdrawn Lucky Pack
// remains recognized so using it retains the item and explains unavailability.
func IsGachaPack(id uint16) bool {
	switch id {
	case 34171, 34172, 34174, 34192, 34193, 34199, 34201, 34229,
		34248, 34296, 34297, 34346, 34349, 34365, 34366, 34367,
		34381, 34382, 34383, 34384, 34385, 34386, 34387, 34388, UnavailableLuckyPackID:
		return true
	}
	return false
}
