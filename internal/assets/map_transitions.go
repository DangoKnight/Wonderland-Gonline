package assets

// MapTransition describes an explicit portal override and optional one-time exit
// reward. SkipIfOwned retains compatibility with installations that already gave
// an old tent; the quest marker prevents replay after the new gift is consumed.
type MapTransition struct {
	FromMap     uint16            `json:"from_map"`
	Entry       uint16            `json:"entry"`
	Destination TravelDestination `json:"destination"`
	Quest       uint32            `json:"quest"`
	Talk        uint32            `json:"talk"`
	Items       []BoxedItem       `json:"items"`
	SkipIfOwned []uint16          `json:"skip_if_owned"`
}
