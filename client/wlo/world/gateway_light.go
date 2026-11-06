package world

import (
	"image"
	"math"
	"time"
	"wonderland-gonline/client/wlo/surface"
)

// Native DoorLight uses a 120x234 frame, anchored three quarters down its
// height. Retain that footprint and additive light blending without its sprites.
const (
	gatewayWidth      = 120
	gatewayHeight     = 234
	gatewayAnchorY    = 176
	gatewayCenterX    = 60.0
	gatewayGroundY    = 186.0
	gatewayRadiusX    = 26.0
	gatewayRadiusY    = 9.0
	gatewayPeriod     = 3900 * time.Millisecond
	gatewayPillars    = 7
	gatewayParticles  = 9
	gatewayLightLevel = 24
)

type gatewayEnergy struct{ r, g, b float64 }
type gatewayLight struct {
	pixels *surface.Surface
	energy []gatewayEnergy
}

func newGatewayLight() *gatewayLight {
	return &gatewayLight{pixels: surface.New(gatewayWidth, gatewayHeight), energy: make([]gatewayEnergy, gatewayWidth*gatewayHeight)}
}
func (p *gatewayLight) add(x, y int, energy float64, r, g, b float64) {
	i := y*gatewayWidth + x
	p.energy[i].r += energy * r
	p.energy[i].g += energy * g
	p.energy[i].b += energy * b
}

func (p *gatewayLight) render(elapsed time.Duration) {
	clear(p.energy)
	phase := 2 * math.Pi * float64(elapsed%gatewayPeriod) / float64(gatewayPeriod)
	// A faint continuous ground ring connects the scattered pools of light.
	// Keep a steady baseline so its shimmer never breaks the ellipse apart.
	for y := int(gatewayGroundY) - 22; y < int(gatewayGroundY)+23; y++ {
		for x := int(gatewayCenterX) - 40; x < int(gatewayCenterX)+41; x++ {
			dx, dy := float64(x)+.5-gatewayCenterX, float64(y)+.5-gatewayGroundY
			radius := math.Hypot(dx/gatewayRadiusX, dy/gatewayRadiusY)
			angle := math.Atan2(dy/gatewayRadiusY, dx/gatewayRadiusX)
			shimmer := .8 + .2*math.Pow(.5+.5*math.Cos(7*angle-2*phase), 2)
			ring := math.Exp(-math.Pow((radius-1)*7, 2)) * .16 * shimmer
			halo := math.Exp(-(dx*dx/850 + dy*dy/90)) * .025
			p.add(x, y, ring+halo, .18, .62, 1)
		}
	}
	// Uneven blue plumes swell above their feet and trail into fine vertical
	// streaks. Slowly shifting roots and staggered pulses avoid a row of identical
	// white needles while keeping the native gateway footprint.
	for pillar := 0; pillar < gatewayPillars; pillar++ {
		seed := float64(pillar) * 2.399963229728653
		angle := 2*math.Pi*float64(pillar)/gatewayPillars + .16*math.Sin(phase+seed)
		bx := gatewayCenterX + gatewayRadiusX*math.Cos(angle)
		by := gatewayGroundY + gatewayRadiusY*math.Sin(angle)
		height := 82 + 92*(.5+.5*math.Sin(seed+phase))
		brightness := .25 + 1.3*math.Pow(.5+.5*math.Sin(phase*2+seed), 2)
		flareY := by - 5 - 12*(.5+.5*math.Sin(phase*2+seed+1))
		for y := max(0, int(by-height)); y < min(gatewayHeight, int(by)+12); y++ {
			vertical := (by - float64(y)) / height
			taper := math.Pow(max(0.0, 1-vertical), .65)
			center := bx + 2.1*math.Sin(phase*2+seed+vertical*8)*math.Pow(taper, 3)
			coreWidth := .22 + 2.5*math.Exp(-math.Pow((vertical-.13)/.15, 2))
			for x := max(0, int(center)-13); x < min(gatewayWidth, int(center)+14); x++ {
				dx := float64(x) + .5 - center
				core := math.Exp(-dx * dx / (2 * coreWidth * coreWidth))
				halo := math.Exp(-dx*dx/24) * .42
				baseFade := math.Exp(-math.Pow(max(0.0, -vertical)*height/5, 2))
				intensity := core * taper * baseFade * brightness
				lower := max(0.0, 1-vertical)
				p.add(x, y, intensity, .06+.32*math.Pow(lower, 3), .25+.58*lower*lower, 1)
				p.add(x, y, halo*taper*baseFade*brightness, .04, .25, 1)
				flare := math.Exp(-(dx*dx/7 + math.Pow((float64(y)-flareY)/7, 2))) * brightness
				p.add(x, y, flare, .36, .8, 1)
				foot := math.Exp(-(dx*dx/20 + math.Pow((float64(y)-by)/5, 2))) * brightness * .9
				p.add(x, y, foot, .22, .7, 1)
				p.add(x, y, math.Exp(-(dx*dx/48+math.Pow((float64(y)-by)/8, 2)))*.5*brightness, .06, .32, 1)
			}
		}
	}
	// Small motes ascend independently, fading in and out before wrapping so
	// their respawn cannot produce a hard pop or a stepped animation frame.
	for mote := 0; mote < gatewayParticles; mote++ {
		seed := float64(mote) * 2.399963229728653
		life := math.Mod(float64(elapsed%gatewayPeriod)/float64(gatewayPeriod)+float64(mote)/gatewayParticles, 1)
		angle := seed + .18*math.Sin(phase+seed)
		px := gatewayCenterX + (gatewayRadiusX+3*math.Sin(seed))*math.Cos(angle) + 2*math.Sin(phase*2+seed)
		py := gatewayGroundY + gatewayRadiusY*math.Sin(angle) - life*(45+75*(.5+.5*math.Sin(seed)))
		brightness := math.Pow(math.Sin(math.Pi*life), 2) * .35
		for y := max(0, int(py)-3); y < min(gatewayHeight, int(py)+4); y++ {
			for x := max(0, int(px)-3); x < min(gatewayWidth, int(px)+4); x++ {
				dx, dy := float64(x)+.5-px, float64(y)+.5-py
				p.add(x, y, math.Exp(-(dx*dx+dy*dy)*1.2)*brightness, .35, .85, 1)
			}
		}
	}
	for i, e := range p.energy {
		p.pixels.Pix[i] = surface.RGB565(byte(min(255.0, e.r*255)), byte(min(255.0, e.g*255)), byte(min(255.0, e.b*255)))
	}
	p.pixels.Revision++
}

func (p *gatewayLight) draw(dst *surface.Surface, x, y int) {
	dst.DrawLight(x-gatewayWidth/2, y-gatewayAnchorY, image.Rect(0, 0, gatewayWidth, gatewayHeight), p.pixels, gatewayLightLevel)
}
