package assets

// Deterministic type-17 boxes, distinct from weighted gacha packs.
type BoxedItem struct {
	ItemID   uint16 `json:"item_id"`
	Quantity byte   `json:"quantity"`
}
