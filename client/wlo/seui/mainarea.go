package seui

// MainAreaW and MainAreaH are the main form object's +0x2d0 → (+0x38,
// +0x3c) area, which windows centre in; it is not the 800 × 600 screen.
// Every Social capture puts its window at (200, 31), so the area is
// 756 × 555; the fields themselves are untraced.
const (
	MainAreaW = 756
	MainAreaH = 555
)
