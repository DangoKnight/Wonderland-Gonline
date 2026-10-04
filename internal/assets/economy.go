package assets

// Economy contains source-authored shared rules loaded only from assets.db.
// Offline SQL initialization seeds missing definitions; gameplay never reads JSON.
type Economy struct {
	Synthesis     SynthesisRules        `json:"synthesis"`
	Marriage      MarriageRules         `json:"marriage"`
	Manufacturing []ManufacturingRecipe `json:"manufacturing"`
	Gathering     []GatheringPool       `json:"gathering"`
}
type MarriageRules struct {
	MinimumLevel uint16    `json:"minimum_level"`
	Fee          uint32    `json:"fee"`
	Rings        [2]uint16 `json:"rings"`
}
type ManufacturingInput struct {
	ItemID uint16 `json:"item_id"`
	Count  byte   `json:"count"`
}
type ManufacturingRecipe struct {
	SuccessPercent *float64              `json:"success_percent,omitempty"`
	Fee            uint32                `json:"fee"`
	Workbench      string                `json:"workbench"`
	Inputs         [2]ManufacturingInput `json:"inputs"`
	Output         ManufacturingInput    `json:"output"`
}
type GatheringPool struct {
	Kind            byte     `json:"kind"`
	IntervalSeconds uint32   `json:"interval_seconds"`
	Items           []uint16 `json:"items"`
}

type SynthesisRules struct {
	FailureItemID         uint16          `json:"failure_item_id"`
	DefaultSuccessPercent float64         `json:"default_success_percent"`
	Rates                 []SynthesisRate `json:"rates"`
}
type SynthesisRate struct {
	Input1         uint16  `json:"input1"`
	Input2         uint16  `json:"input2"`
	Output         uint16  `json:"output"`
	SuccessPercent float64 `json:"success_percent"`
	Fee            uint32  `json:"fee"`
}

const (
	GatheringFishing     byte = 1
	GatheringMining      byte = 2
	GatheringWoodcutting byte = 3
)
