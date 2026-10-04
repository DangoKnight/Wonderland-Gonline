package assets

// TentRules is source-authored content loaded from typed assets SQL tables.
type TentRules struct {
	SpawnX    uint16                 `json:"spawn_x"`
	SpawnY    uint16                 `json:"spawn_y"`
	Floor     uint16                 `json:"floor"`
	Wallpaper uint16                 `json:"wallpaper"`
	Furniture []TentFurnitureDefault `json:"furniture"`
}
type TentFurnitureDefault struct {
	ItemID   uint16 `json:"item_id"`
	X        uint16 `json:"x"`
	Y        uint16 `json:"y"`
	Floor    byte   `json:"floor"`
	Rotation byte   `json:"rotation"`
}
