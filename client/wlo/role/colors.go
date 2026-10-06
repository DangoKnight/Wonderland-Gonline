package role

import (
	"wonderland-gonline/client/wlo/surface"
	"wonderland-gonline/internal/clientassets"
)

// Character colours. A THuman keeps 45 colour digits at +0xd5, 0..8 with 4
// neutral; each sprite draw shifts palette ranges by (digit-4)·25 per
// channel (FUN_002fe8e8 → FUN_0030105c).
const (
	colorDigits  = 0x2d
	neutralDigit = 4
	digitStep    = 0x19
	// Channels clamp to 5..250 after the shift.
	channelMin = 5
	channelMax = 0xfa
	// Sprites drawn uncoloured (FUN_002fe888).
	uncoloredSprite1 = 0x178b
	uncoloredSprite2 = 9000
	// allParts is FUN_00444e28's "every group" selector (0xff).
	allParts = -1
)

// Colors is the colour digit block (+0xd5).
type Colors [colorDigits]byte

// NeutralColors is the block FUN_004013d0 starts from (FUN_00012d10(…, 4)).
func NeutralColors() Colors {
	var c Colors
	for i := range c {
		c[i] = neutralDigit
	}
	return c
}

// digitTriple stores three decimal digits of v at i.
func (c *Colors) digitTriple(i, v int) {
	c[i], c[i+1], c[i+2] = byte(v/100), byte(v/10%10), byte(v%10)
}

// Set is FUN_00444e28: value holds nine digits (three groups of three) for
// part 1..5; only selects which groups are written, allParts for all.
func (c *Colors) Set(value int32, only int8, part int) {
	v := int(value)
	hi, mid, lo := v/1000000, v/1000%1000, v%1000
	any := only == allParts
	switch part {
	case 1:
		if any || only == 1 {
			c.digitTriple(0, hi)
		}
		if any {
			c.digitTriple(3, mid)
			c.digitTriple(6, lo)
		}
	case 2:
		if any {
			c.digitTriple(9, hi)
		}
		if any || only == 2 {
			c.digitTriple(0xc, mid)
			c.digitTriple(0xf, lo)
		}
	case 3:
		if any || only == 2 {
			c.digitTriple(0x12, hi)
		}
		if any || only == 5 {
			c.digitTriple(0x15, mid)
			c.digitTriple(0x18, lo)
		}
	case 4:
		if any || only == 3 {
			c.digitTriple(0x1b, hi)
			c.digitTriple(0x1e, mid)
		}
		if any || only == 4 {
			c.digitTriple(0x21, lo)
		}
	case 5:
		if any {
			c.digitTriple(9, hi)
			c.digitTriple(0x27, mid)
		} else if only != 5 {
			return
		}
		c.digitTriple(0x2a, lo)
	}
}

// colorRanges are the palette ranges a player sprite shifts and the
// digits (red, green, blue) each one uses: 0x10..0xcf take digits 0..35
// in order, 0xd0..0xdf digits 39..41.
var colorRanges = func() (r [13]struct{ lo, digit int }) {
	for k := 0; k < 12; k++ {
		r[k].lo, r[k].digit = 0x10*(k+1), 3*k
	}
	r[12].lo, r[12].digit = 0xd0, 0x27
	return
}()

// shiftChannel is one channel of FUN_0030105c.
func shiftChannel(v uint8, delta int) uint8 {
	n := int(v) + delta
	switch {
	case n > channelMax:
		return channelMax
	case n < channelMin:
		return channelMin
	}
	return uint8(n)
}

// palette applies the block to a sprite's palette as the full branch of
// FUN_002fe8e8 does for a player drawn outside battle.
func (c *Colors) palette(id int, src *[256][3]uint8) [256][3]uint8 {
	digits := *c
	if id == uncoloredSprite1 || id == uncoloredSprite2 {
		digits = NeutralColors()
	}
	p := *src
	for _, r := range colorRanges {
		d := digits[r.digit : r.digit+3]
		for i := r.lo; i < r.lo+0x10; i++ {
			for ch := 0; ch < 3; ch++ {
				p[i][ch] = shiftChannel(p[i][ch], (int(d[ch])-neutralDigit)*digitStep)
			}
		}
	}
	return p
}

// lookup is FUN_00301118's palette conversion, as RGB565 with 0 for
// transparent entries.
func lookup(p *[256][3]uint8) (out [256]uint16, opaque [256]bool) {
	for i := range p {
		r, g, b, a := clientassets.NativeSpriteColor(byte(i), *p)
		if a != 0 {
			out[i], opaque[i] = surface.RGB565(r, g, b), true
		}
	}
	return
}

// litSteps is the hover highlight: every colour group is lightened by four
// digit steps (+100 a channel, clamped), measured from the captures of
// a hovered NPC (Highlight.png against Pre-Highlight.png). The
// original's blit (FUN_002fe8e8) is not traced this far.
const litSteps = 4

// lifted is the block with every digit raised by steps.
func (c Colors) lifted(steps int) Colors {
	for i := range c {
		c[i] += byte(steps)
	}
	return c
}
