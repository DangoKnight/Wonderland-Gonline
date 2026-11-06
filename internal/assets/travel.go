package assets

type TravelRules struct {
	ReturnScrolls map[uint16]TravelDestination `json:"return_scrolls"`
	BambooItems   []uint16                     `json:"bamboo_items"`
	Scrolls       map[uint16]byte              `json:"scrolls"`
	Destinations  map[byte]TravelDestination   `json:"destinations"`
}
type TravelDestination struct {
	Name string `json:"name"`
	Map  uint16 `json:"map"`
	X    uint16 `json:"x"`
	Y    uint16 `json:"y"`
}
