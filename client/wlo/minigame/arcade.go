package minigame

import (
	"fmt"
	"image"
	"time"

	"wonderland-go/client/wlo/picdb"
	"wonderland-go/client/wlo/surface"
	"wonderland-go/internal/protocol"
)

const (
	ArcadeEgg                = 6
	ArcadeSlots              = 8
	ArcadeSlots2             = 10
	ArcadeSlots3             = 19
	ArcadeEgg2               = 22
	arcadePoints             = 1
	arcadeAccepted           = 1
	arcadeInsufficientPoints = 2
	arcadePrizeCount         = 11
	arcadeReelSymbols        = 16
	arcadeEggButtonFrames    = 8
	arcadeEggOpenFrames      = 14
	arcadeEggButtonStep      = 200 * time.Millisecond
	arcadeEggOpenStep        = 100 * time.Millisecond
	arcadeReelStep           = 60 * time.Millisecond
	arcadeFirstReelStop      = 1800 * time.Millisecond
	arcadeReelStopInterval   = 1200 * time.Millisecond
)

// Arcade handles the native egg and slot machines. AC71 purchases and replies
// remain server-owned. Rand is never used to choose prizes or reel outcomes.
// References: FUN_001743d0/001749f4/00174cd8 (eggs),
// FUN_001772c0/00178730/001788c0 (slots), FUN_001799cc/0017bb14
// (slot variants), and FUN_002c2394 at 002da086..002da1d0 and
// 002da373..002da5e1 (the request serializers missing from the decompile).
type Arcade struct {
	Round
	Kind byte
	Send func([]byte)
	// Item draws an icon from the client's item catalog and returns its name.
	Item                           func(*surface.Surface, uint16, int, int) string
	pending, confirming, animating bool
	replyAt                        time.Time
	prize, quantity                byte
	reels                          [3]byte
	symbols                        [3]int
	status                         string
}

// These prize-index translations are executable compatibility tables from the
// WLRI build ca19ee087b60: PTR_DAT_004c9944, 004ca8e4, 004c9888,
// 004ca80c and 004ca194. AC71 carries indexes into them, rather than item IDs.
// They describe the presentation only; the server grants the actual inventory.
var arcadePrizes = map[byte][arcadePrizeCount]uint16{
	ArcadeEgg:    {0, 32162, 46004, 33044, 34089, 30553, 22156, 30065, 30066, 30556, 22166},
	ArcadeEgg2:   {0, 32162, 51142, 33044, 34122, 33053, 22894, 21601, 21707, 34155, 22115},
	ArcadeSlots:  {30063, 61044, 61039, 61037, 61035, 61033, 61032, 61031, 61030, 61028, 61026},
	ArcadeSlots2: {32095, 34123, 34116, 34115, 34114, 34112, 34111, 34110, 34109, 34108, 34107},
	ArcadeSlots3: {30063, 62011, 62010, 62007, 62006, 62005, 62004, 62003, 62002, 62001, 62000},
}

func NewArcade(pics *picdb.DB, kind byte) *Arcade {
	return &Arcade{Round: newRound(pics), Kind: kind, symbols: [3]int{1, 1, 1}}
}
func (g *Arcade) egg() bool { return g.Kind == ArcadeEgg || g.Kind == ArcadeEgg2 }
func (g *Arcade) backgroundName() string {
	switch g.Kind {
	case ArcadeEgg:
		return "TrunEggBG"
	case ArcadeEgg2:
		return "TrunEggBG2"
	default:
		return "SlotmachBG"
	}
}
func (g *Arcade) buttonName() string {
	if g.Kind == ArcadeEgg2 {
		return "TrunEggButton2"
	}
	if g.egg() {
		return "TrunEggButton"
	}
	return "SlotmachBtnBG"
}
func (g *Arcade) Pictures() []string {
	return []string{g.backgroundName(), g.buttonName(), "TrunEggItemDB", "TrunEggA", "SlotmachItemT", "SlotmachRing"}
}
func (g *Arcade) Prizes() []uint16 {
	table := arcadePrizes[g.Kind]
	return append([]uint16(nil), table[:]...)
}
func (g *Arcade) buttonRect() image.Rectangle {
	x, y, w, h := 697, 265, 64, 64
	rows := 3
	if g.egg() {
		x, y, w, h, rows = 410, 385, 100, 125, arcadeEggButtonFrames
	}
	if g.Pics != nil {
		if i := g.Pics.Find(g.buttonName()); i >= 0 {
			w, h = g.Pics.Size(i)
			h /= rows
		}
	}
	return image.Rect(x, y, x+w, y+h)
}
func (g *Arcade) Click(x, y int, now time.Time) {
	g.Update(now)
	if !g.Started || g.Done() || g.pending || g.animating {
		return
	}
	if !image.Pt(x, y).In(g.buttonRect()) {
		g.confirming = false
		return
	}
	// FUN_001754c4 and FUN_00176b90 ask before spending points. The second
	// click confirms the visible purchase prompt; Start itself never purchases.
	if !g.confirming {
		g.confirming = true
		g.status = "Spend arcade points? Click Play again to confirm."
		return
	}
	g.confirming = false
	g.pending = true
	g.status = "Waiting for server..."
	p := []byte{protocol.CommandArcadeGame, g.Kind, arcadePoints}
	switch g.Kind {
	case ArcadeSlots, ArcadeSlots3:
		p = append(p, 0) // No inventory ticket selected.
	case ArcadeSlots2:
		p = []byte{protocol.CommandArcadeGame, g.Kind, 0}
	}
	if g.Send != nil {
		g.Send(p)
	}
}

// Receive takes the payload after AC71. Unsolicited, mismatched, truncated,
// duplicate and unknown replies cannot settle a purchase. A denial allows retry.
func (g *Arcade) Receive(p []byte, now time.Time) bool {
	if !g.pending || g.Done() || len(p) < 2 || p[0] != g.Kind {
		return false
	}
	if p[1] == arcadeInsufficientPoints {
		g.pending = false
		g.status = "Not enough Points!"
		return true
	}
	required := 7
	if g.egg() {
		required = 4
	}
	if p[1] != arcadeAccepted || len(p) != required || int(p[2]) >= arcadePrizeCount {
		return false
	}
	if !g.egg() {
		for _, symbol := range p[3:6] {
			if symbol < 1 || symbol > arcadeReelSymbols {
				return false
			}
		}
		// Native receive dispatcher at 002ee125 pushes reels [3:6],
		// then quantity [6]; the decompiler drops the fourth stack argument.
		g.reels = [3]byte{p[3], p[4], p[5]}
	}
	g.pending = false
	g.animating = true
	g.replyAt = now
	g.prize = p[2]
	g.quantity = p[required-1]
	g.status = ""
	g.sound("sound\\wav9914.wav")
	return true
}
func (g *Arcade) Update(now time.Time) {
	if !g.Started || g.Done() || !g.animating {
		return
	}
	elapsed := now.Sub(g.replyAt)
	if g.egg() {
		duration := (arcadeEggButtonFrames-2)*arcadeEggButtonStep + arcadeEggOpenFrames*arcadeEggOpenStep
		if elapsed < duration {
			return
		}
	} else {
		complete := true
		for i := range g.symbols {
			stop := arcadeFirstReelStop + time.Duration(i)*arcadeReelStopInterval
			if elapsed >= stop {
				g.symbols[i] = int(g.reels[i])
			} else {
				g.symbols[i] = 1 + int(elapsed/arcadeReelStep)%arcadeReelSymbols
				complete = false
			}
		}
		if !complete {
			return
		}
	}
	g.animating = false
	g.status = fmt.Sprintf("Server reward: %d pcs (prize %d)", g.quantity, g.prize)
	g.sound("sound\\wav0152.wav")
}
func (g *Arcade) Draw(dst *surface.Surface, mx, my int, now time.Time) {
	dst.Fill(image.Rect(0, 0, dst.W, dst.H), arcadePanel)
	bgX, bgY := 70, 25
	if g.egg() {
		bgX, bgY = 100, 15
	}
	g.picture(dst, g.backgroundName(), bgX, bgY)
	if !g.Started {
		return
	}
	table := arcadePrizes[g.Kind]
	hovered := ""
	for index := 1; index < len(table); index++ {
		x, y := 160, 280-(index-6)*50
		if index < 6 {
			x, y = 440, 280-(index-1)*50
		}
		if !g.egg() {
			row := (index - 1) % 5
			x = 278
			if index > 5 {
				x = 491
			}
			y = [...]int{112, 170, 222, 277, 332}[row]
		}
		if g.egg() {
			g.picture(dst, "TrunEggItemDB", x, y)
		}
		label := fmt.Sprintf("Item %d", table[index])
		if g.Item != nil {
			if name := g.Item(dst, table[index], x+6, y+6); name != "" {
				label = name
			}
		}
		if g.egg() {
			arcadeText(dst, x+45, y+12, label)
		} else if image.Pt(mx, my).In(image.Rect(x, y, x+40, y+40)) {
			hovered = label
		}
	}
	if !g.egg() {
		for i, symbol := range g.symbols {
			g.strip(dst, "SlotmachRing", 129+i*178, 410, symbol-1, arcadeReelSymbols)
		}
	}
	r := g.buttonRect()
	frame, rows := 0, 3
	if g.egg() {
		rows = arcadeEggButtonFrames
		if g.animating {
			elapsed := now.Sub(g.replyAt)
			frame = min(arcadeEggButtonFrames-1, 2+int(elapsed/arcadeEggButtonStep))
			openAt := (arcadeEggButtonFrames - 2) * arcadeEggButtonStep
			if elapsed >= openAt {
				g.strip(dst, "TrunEggA", 160, 421, min(arcadeEggOpenFrames-1, int((elapsed-openAt)/arcadeEggOpenStep)), arcadeEggOpenFrames)
			}
		}
	}
	if !g.strip(dst, g.buttonName(), r.Min.X, r.Min.Y, frame, rows) {
		dst.Frame(r, arcadeWhite)
		arcadeText(dst, r.Min.X+5, r.Min.Y+10, "Play")
	}
	if g.confirming {
		dst.Frame(r, arcadeWhite)
	}
	label := g.status
	if label == "" {
		label = hovered
	}
	arcadeText(dst, 150, 550, label)
	if !g.animating && !g.pending && g.quantity > 0 && g.Item != nil {
		g.Item(dst, table[g.prize], 600, 520)
	}
}
