package hud

import (
	"fmt"
	"image"
	"math"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/world"
)

const (
	// MainStatus +0x270 is (150,38); the native portrait/EXP extend six
	// pixels left. FUN_0029d3a4 supplies the remaining compact offsets.
	petStatusLeft   = 150 - 6
	petStatusTop    = 38
	petStatusWidth  = 72
	petStatusHeight = 35
	petStatsLeft    = 31
	petLevelRows    = 10
	petDigitStep    = 5
)

type PetInfo struct {
	ID    uint16
	Name  []byte
	Stats world.Stats
}

// PetStatus is the active pet's compact portrait, level, HP/SP and EXP panel.
// Read resolves the current roster each frame, independently of Inventory's
// local selection. No gameplay state or portrait bitmap is owned by this view.
type PetStatus struct {
	seui.Component
	Read         func() (PetInfo, bool)
	DrawPortrait func(id uint16, x, y int)
	current      PetInfo
}

func newPetStatus(env *seui.Env, owner seui.Control) *PetStatus {
	p := &PetStatus{}
	p.InitComponent(p, env, owner)
	p.Name = "PetStatus"
	p.Init("", statusCorner.X+petStatusLeft, 0, 0, 0, 0, false, petStatusHeight, petStatusWidth, statusCorner.Y+petStatusTop)
	p.Visible = false
	return p
}

func (p *PetStatus) Refresh() {
	p.Visible = false
	if p.Read != nil {
		info, ok := p.Read()
		p.current, p.Visible = info, ok && info.ID != 0
	}
}

func (p *PetStatus) digitNumber(value, x, y int) {
	imageID := p.Env.Pics.Find("Pet_Num_Lv_White_1")
	width, height := p.Env.Pics.Size(imageID)
	height /= petLevelRows
	for i, digit := range strconv.Itoa(value) {
		top := int(digit-'0') * height
		p.Env.Pics.DrawRect(p.Env.Screen, imageID, x+i*petDigitStep, y, image.Rect(0, top, width, top+height), true)
	}
}

// Paint follows FUN_0029dbf0 with the dedicated 007-family portrait and the
// original Word_*, Pet_Frame_* and Pet_Bmp_* artwork.
func (p *PetStatus) Paint() {
	a, s := p.Abs(), p.current.Stats
	x := a.X + petStatsLeft
	pics, screen := p.Env.Pics, p.Env.Screen
	if p.DrawPortrait != nil {
		p.DrawPortrait(p.current.ID, a.X+12, a.Y-6)
	}
	pics.Draw(screen, pics.Find("Word_Lv_1"), x, a.Y+2, true)
	p.digitNumber(int(s.Level), x+17, a.Y+2)
	for i, kind := range []string{"Hp", "Sp"} {
		top := a.Y + 10 + i*8
		pics.Draw(screen, pics.Find("Word_"+kind+"_1"), x, top, true)
		pics.Draw(screen, pics.Find("Pet_Frame_"+kind), x+13, top+1, true)
		value, maximum := s.HP, s.MaxHP
		if i == 1 {
			value, maximum = uint32(s.SP), uint32(s.MaxSP)
		}
		bar := pics.Find("Pet_Bmp_" + kind)
		width, height := pics.Size(bar)
		fill := 0
		if maximum > 0 {
			fill = int(uint64(width) * uint64(min(value, maximum)) / uint64(maximum))
		}
		pics.DrawRect(screen, bar, x+14, top+2, image.Rect(0, 0, fill, height), true)
	}
	ratio := s.Formula.Progress(s.Level, s.EXP, s.Rebirth != 0)
	if math.IsNaN(ratio) || ratio < 0 {
		ratio = 0
	}
	ratio = min(1, ratio)
	pics.Draw(screen, pics.Find("Pet_Frame_Exp"), a.X, a.Y+30, true)
	bar := pics.Find("Pet_Bmp_Exp")
	width, height := pics.Size(bar)
	fill := int(float64(width) * math.RoundToEven(ratio*fullGauge) / fullGauge)
	pics.DrawRect(screen, bar, a.X+1, a.Y+31, image.Rect(0, 0, fill, height), true)
	pct := int(ratio * fullGauge)
	p.digitNumber(pct, a.X+21, a.Y+29)
	pics.Draw(screen, pics.Find("Pet_Percent_Exp"), a.X+21+len(strconv.Itoa(pct))*petDigitStep, a.Y+30, true)
}

func (p *PetStatus) Hint() {
	a, in, s := p.Abs(), p.Env.UI.Input, p.current.Stats
	text := string(p.current.Name)
	switch y := in.Y - a.Y; {
	case y >= 10 && y < 18 && in.X >= a.X+petStatsLeft:
		text = fmt.Sprintf("HP: %d / %d", s.HP, s.MaxHP)
	case y >= 18 && y < 26 && in.X >= a.X+petStatsLeft:
		text = fmt.Sprintf("SP: %d / %d", s.SP, s.MaxSP)
	case y >= 28:
		text = fmt.Sprintf("EXP: %d", s.EXP)
	}
	p.SetHint([]byte(text))
	p.GrBasic.Hint()
}
