package login

import (
	"encoding/binary"
	"encoding/hex"
	"image"
	"math"
	"os"
	"strconv"

	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/game"
)

// Status is the part of TSe_MainStatus (constructor FUN_0025ff84) that the
// character selection borrows to draw a slot's level, job, HP, SP, EXP and
// gold. The original stores the values on a role object (+0x30c) and calls
// the drawers below; the values are passed directly here.
type Status struct {
	Env      *seui.Env
	Formula  *Formula
	WordLv   int // +0x188 Main_Word_Lv
	WordHP   int // +0x18c Main_Word_Hp
	WordSP   int // +0x190 Main_Word_Sp
	WordExp  int // +0x194 Main_Word_Exp
	WordGold int // +0x198 Main_Word_Gold
	ExpBar   int // +0x200 Panel38
	ExpFrame int // +0x204 Panel37
	Digits   int // +0x208 Num_MainInfo: 0-9, "/" and "%", one per row
	LvDigits int // +0x20c Num_White3_1: 0-9, one per row
	// GoldEndX and GoldIconY are +0x320 and +0x324, where the gold
	// drawer leaves the position for a following icon.
	GoldEndX, GoldIconY int
	GoldLarge           bool // +0x328
}

// Rows of Num_MainInfo after the ten digits.
const (
	digitRows    = 0xb // the sheet height is divided by 11
	levelRows    = 10
	percentRow   = 10 // read as 10h..11h by the EXP drawer
	slashRow     = 11 // read as 11h..12h by the HP and SP drawers
	wordGapLevel = 0x1e
	wordGap      = 0x23
)

// jobNames are the tooltips of the job icons (PTR_DAT_004c980c).
var jobNames = [...]string{1: "Killer", 2: "Warrior", 3: "Knight", 4: "Wit", 5: "Priest", 6: "Seer"}

// jobIcons are the icons FUN_002618bc picks, small then large.
var jobIcons = [...][2]string{
	1: {"Icon_Asn", "Icon_Asn_2"}, 2: {"Icon_Samurai", "Icon_Samurai_2"},
	3: {"Icon_Knight", "Icon_Knight_2"}, 4: {"Icon_Sage", "Icon_Sage_2"},
	5: {"Icon_Clergyman", "Icon_Clergyman_2"}, 6: {"Icon_Prophet", "Icon_Prophet_2"},
}

func NewStatus(env *seui.Env, f *Formula) *Status {
	p := env.Pics
	return &Status{
		Env: env, Formula: f,
		WordLv: p.Find("Main_Word_Lv"), WordHP: p.Find("Main_Word_Hp"), WordSP: p.Find("Main_Word_Sp"),
		WordExp: p.Find("Main_Word_Exp"), WordGold: p.Find("Main_Word_Gold"),
		ExpBar: p.Find("Panel38"), ExpFrame: p.Find("Panel37"),
		Digits: p.Find("Num_MainInfo"), LvDigits: p.Find("Num_White3_1"),
	}
}

func (s *Status) draw(img, x, y int) { s.Env.Pics.Draw(s.Env.Screen, img, x, y, true) }

// digit draws row n of a digit sheet with rows of height h and width w.
func (s *Status) digit(img, n, w, h, x, y int) {
	s.Env.Pics.DrawRect(s.Env.Screen, img, x, y, image.Rect(0, n*h, w, n*h+h), true)
}

// Level is FUN_00261730 (through FUN_002627fc): Num_White3_1 digits one
// pixel closer than their width, after the "Lv" word when labelled.
func (s *Status) Level(level byte, at image.Point, labelled bool) {
	x, y := at.X, at.Y
	if labelled {
		s.draw(s.WordLv, x, y)
		x += wordGapLevel
		y++
	}
	w, h := s.Env.Pics.Size(s.LvDigits)
	w &= 0xff
	h /= levelRows
	for i, c := range strconv.Itoa(int(level)) {
		s.digit(s.LvDigits, int(c-'0'), w, h, x+(w-1)*i, y)
	}
}

// number draws decimal digits of Num_MainInfo three pixels closer than
// their width and returns the x after them.
func (s *Status) number(v uint64, x, y int) int {
	w, h := s.Env.Pics.Size(s.Digits)
	h /= digitRows
	step := int(int8(byte(w) - 3))
	str := strconv.FormatUint(v, 10)
	for i, c := range str {
		s.digit(s.Digits, int(c-'0'), w, h, x+i*step, y)
	}
	return x + len(str)*step
}

// pair draws "a/b" (FUN_00262128 and FUN_00262370).
func (s *Status) pair(word int, a, b uint64, at image.Point, labelled bool) {
	x, y := at.X, at.Y
	if labelled {
		s.draw(word, x, y)
		x += wordGap
		y++
	}
	w, h := s.Env.Pics.Size(s.Digits)
	h /= digitRows
	x = s.number(a, x, y)
	s.digit(s.Digits, slashRow, w, h, x, y)
	step := int(int8(byte(w) - 3))
	s.number(b, x+step, y)
}

// HP is FUN_002627c0: the role's +0x1f84, a slash, then +0x1fa8.
func (s *Status) HP(first, second uint32, at image.Point, labelled bool) {
	s.pair(s.WordHP, uint64(first), uint64(second), at, labelled)
}

// SP is FUN_00262830: the role's +0x1f88, a slash, then +0x1fac, both
// 16-bit.
func (s *Status) SP(first, second uint16, at image.Point, labelled bool) {
	s.pair(s.WordSP, uint64(first), uint64(second), at, labelled)
}

// Gold is FUN_002625c0.
func (s *Status) Gold(gold uint32, at image.Point, labelled bool) {
	x, y := at.X, at.Y
	if labelled {
		s.draw(s.WordGold, x, y)
		x += wordGap
		y++
	}
	_, h := s.Env.Pics.Size(s.Digits)
	h /= digitRows
	s.GoldEndX = s.number(uint64(gold), x, y) + 5
	if !s.GoldLarge {
		s.GoldIconY = y - (0x10 - h)
	} else {
		s.GoldIconY = y - (0x12 - h)
	}
}

// Exp is FUN_00261e7c (through FUN_00262778): the frame, the bar filled
// to the share of the level gained, and the percentage.
func (s *Status) Exp(level byte, exp uint32, reborn bool, at image.Point, labelled bool) {
	x, y := at.X, at.Y
	dy := 0
	if labelled {
		s.draw(s.WordExp, x, y)
		x += wordGap
		dy = 1
	}
	// The original adds the label's one-pixel offset to y and then again
	// to the frame and bar.
	y += dy
	ratio := s.Formula.Progress(level, exp, reborn)
	if ratio > 1 {
		ratio = 1
	}
	s.draw(s.ExpFrame, x-2, y+dy)
	bw, bh := s.Env.Pics.Size(s.ExpBar)
	s.Env.Pics.DrawRect(s.Env.Screen, s.ExpBar, x, y+dy+2, image.Rect(0, 0, delphiRound(float64(bw)*ratio), bh), true)
	fw, _ := s.Env.Pics.Size(s.ExpFrame)
	tx := x + fw + 2
	w, h := s.Env.Pics.Size(s.Digits)
	h /= digitRows
	step := int(int8(byte(w) - 3))
	pct := strconv.Itoa(delphiRound(ratio * 100))
	for i, c := range pct {
		s.digit(s.Digits, int(c-'0'), w, h, tx+i*step, y)
	}
	s.digit(s.Digits, percentRow, w, h, tx+len(pct)*step, y)
}

// Job is FUN_002618bc: the job's icon, and its name in a tooltip while
// the pointer is over it.
func (s *Status) Job(job byte, at image.Point, large bool, above bool) {
	if job == 0 || int(job) >= len(jobIcons) {
		return
	}
	size := 0x10
	icon := jobIcons[job][0]
	if large {
		size, icon = 0x12, jobIcons[job][1]
	}
	s.draw(s.Env.Pics.Find(icon), at.X, at.Y)
	in := s.Env.UI.Input
	if !image.Pt(in.X, in.Y).In(image.Rect(at.X, at.Y, at.X+size, at.Y+size)) {
		return
	}
	name := []byte(jobNames[job])
	x, y := at.X, at.Y
	if above {
		y -= 0x14
	} else {
		x += 0x18
	}
	r := image.Rect(x, y, x+len(name)*8+7, y+0x16)
	s.Env.Screen.FillAlpha(r, 0xf98b3d, 200)
	s.Env.Screen.Frame(r, surface.TColor(0x800000))
	s.Env.Text.Draw(x+4, y+3, 0, false, true, s.Env.Screen, name, 0x10, 400, skinTextColor, 0xffff, 2)
}

// delphiRound is Round: banker's rounding.
func delphiRound(v float64) int { return int(math.RoundToEven(v)) }

// Formula is Data\Formula.Dat (FUN_0036e390): one 407-byte record whose
// first byte is the version (2). The fields read here are the experience
// curve.
type Formula struct {
	ExpPower  float64 // record +0xf1
	ExpOffset int32   // record +0x169
	Combat    [5]CombatFormula
	HP, SP    gauge // maximum HP and SP (FUN_0036e674, FUN_0036e704)
}

// gauge is one maximum's constants: level^Power × attribute × PerAttrLevel
// + level × PerLevel + attribute × PerAttr, rounded, plus Base.
type gauge struct {
	PerLevel, PerAttrLevel, Power, PerAttr float64
	Base                                   uint16
}

// Max is FUN_0036e674 / FUN_0036e704 for an effective level (level plus
// 100 per rebirth, FUN_00485490) and its attribute (CON for HP, WIS for
// SP), plus the role's bonuses (equipment and others).
func (g gauge) Max(level int, attr uint16, bonus int) int {
	l, a := float64(level), float64(attr)
	v := delphiRound(math.Pow(l, g.Power)*(a*g.PerAttrLevel) + l*g.PerLevel + a*g.PerAttr)
	return max(v+int(g.Base)+bonus, 0)
}

const (
	formulaRecordBytes  = 0x197
	formulaVersion      = 2
	formulaCombatOffset = 1
	formulaCombatStride = 32
	formulaExpPower     = 0xf1
	formulaExpOffset    = 0x169
	// Maximum HP and SP constants: per level, per attribute and level,
	// the level power, per attribute (doubles), then the base (word).
	formulaHP     = 0xf9
	formulaHPBase = 0x16d
	formulaSP     = 0x119
	formulaSPBase = 0x16f
	// Reborn characters use fixed constants (FUN_0036e78c).
	rebornExpPower      = 3.3
	rebornExpOffset     = 50
	rebornHighLevel     = 0x96
	rebornHighLevelPow  = 4.9
	maxExperienceLevels = 0xff
)

// formulaExport is the extracted Formula.Dat; its decoded_hex is the
// whole record.
const formulaExport = "formula_data.json"

// LoadFormula reads the decompiled formula export.
func LoadFormula(a Assets) (*Formula, error) {
	h, err := readExportHeader(a.DataPath(formulaExport), "")
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(h.DecodedHex)
	if err != nil {
		return nil, err
	}
	if len(raw) < formulaRecordBytes || raw[0] != formulaVersion {
		return nil, os.ErrInvalid
	}
	f64 := func(o int) float64 { return math.Float64frombits(binary.LittleEndian.Uint64(raw[o:])) }
	g := func(o, base int) gauge {
		return gauge{PerLevel: f64(o), PerAttrLevel: f64(o + 8), Power: f64(o + 16), PerAttr: f64(o + 24),
			Base: binary.LittleEndian.Uint16(raw[base:])}
	}
	f := &Formula{
		ExpPower:  f64(formulaExpPower),
		ExpOffset: int32(binary.LittleEndian.Uint32(raw[formulaExpOffset:])),
		HP:        g(formulaHP, formulaHPBase),
		SP:        g(formulaSP, formulaSPBase),
	}
	// Five 32-byte records begin immediately after the version byte.
	for i := range f.Combat {
		o := formulaCombatOffset + i*formulaCombatStride
		f.Combat[i] = CombatFormula{f64(o), f64(o + 8), f64(o + 16), f64(o + 24)}
	}
	return f, nil
}

// LevelExp is FUN_0036e78c: the experience to go from level-1 to level.
func (f *Formula) LevelExp(level byte, reborn bool) int {
	power, offset := f.ExpPower, float64(f.ExpOffset)
	if reborn {
		power, offset = rebornExpPower, rebornExpOffset
		if level > rebornHighLevel {
			offset = math.Pow(float64(level-rebornHighLevel), rebornHighLevelPow)
		}
	}
	return int(math.Pow(float64(level), power) + offset)
}

// TotalExp is FUN_0036e8ac: the experience needed to reach level+1 from 1.
func (f *Formula) TotalExp(level byte, reborn bool) int {
	total := 0
	for l := 1; l <= int(level); l++ {
		total += f.LevelExp(byte(l), reborn)
	}
	return total
}

// Progress is FUN_00261e04: the share of the next level gained.
func (f *Formula) Progress(level byte, exp uint32, reborn bool) float64 {
	if f == nil {
		return 0
	}
	base := f.TotalExp(level, reborn)
	e := int(int32(exp))
	if e-base < 0 {
		e = base
	}
	return float64(e-base) / float64(f.LevelExp(level+1, reborn))
}

// CombatFormula is the four doubles per stat in Formula.Dat. The element
// term applies to Earth DEF, Water MDF, Fire ATK/MAT, and Wind SPD.
// FUN_004166e4 uses banker's rounding after summing these terms.
type CombatFormula struct{ PerAttr, PerLevel, PerContribution, ElementLevel float64 }

func (f *Formula) CombatValues(level int, element byte, attrs [5]uint16, contribution [5]int32) (out [5]int32) {
	elements := [5]byte{byte(game.Fire), byte(game.Earth), byte(game.Fire), byte(game.Water), byte(game.Wind)}
	if f == nil {
		return
	}
	for i, c := range f.Combat {
		v := float64(attrs[i])*c.PerAttr + float64(level)*c.PerLevel + float64(contribution[i])*c.PerContribution
		if element == elements[i] {
			v += float64(level) * c.ElementLevel
		}
		out[i] = int32(delphiRound(v))
	}
	return
}
