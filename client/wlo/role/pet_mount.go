package role

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// MountPlacement is AdjustRidePetPos.txt: four bodies, eight facings.
// Positive Y moves the rider down in raster coordinates. Stand offsets apply
// while walking too; Move holds the two compiled native movement corrections.
type MountPlacement struct {
	Sit         bool
	StandPose   bool
	FixedFirst  bool
	Stand, Move [4][8][2]int
}

func ParseMountPlacements(raw []byte) (map[uint16]MountPlacement, error) {
	var doc struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	out := map[uint16]MountPlacement{}
	var id uint16
	body := -1
	for _, line := range strings.Split(doc.Text, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "NpcPic":
			n, err := strconv.ParseUint(value, 10, 16)
			if err != nil || n == 0 {
				return nil, fmt.Errorf("invalid mount picture")
			}
			id = uint16(n)
			body = -1
		case "Sex":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 || n > 4 {
				return nil, fmt.Errorf("invalid mount body")
			}
			body = n - 1
		case "Pose", "FixedPosOnFirst":
			p := out[id]
			if key == "Pose" {
				p.Sit = value == "sit"
				p.StandPose = value == "stand"
			} else {
				p.FixedFirst = value == "true"
			}
			out[id] = p
		case "StandPosX", "StandPosY", "MovePosX", "MovePosY":
			if id == 0 || body < 0 {
				return nil, fmt.Errorf("mount offsets without picture/body")
			}
			parts := strings.Split(value, ",")
			if len(parts) != nativeFacingDirections {
				return nil, fmt.Errorf("mount requires eight facings")
			}
			p := out[id]
			axis := 0
			if strings.HasSuffix(key, "Y") {
				axis = 1
			}
			for i, v := range parts {
				n, err := strconv.Atoi(v)
				if err != nil {
					return nil, err
				}
				if strings.HasPrefix(key, "Stand") {
					p.Stand[body][i][axis] = n
				} else {
					p.Move[body][i][axis] = n
				}
			}
			out[id] = p
		}
	}
	return out, nil
}
func (h *Human) SetPetMountPlacement(p *MountPlacement) { h.petPlacement = p }

// FUN_0040f1dc selects a directional riding action even while the mount
// walks. AdjustRidePetPos can explicitly request standing or sitting.
const petRiderActionBase = 18

func (h *Human) petRiderAction(direction int32) int {
	base := petRiderActionBase
	p := h.petPlacement
	if p == nil && h.petMount != nil {
		fallback := ResolveMountPlacement(uint16(h.petMount.Sprite()), nil)
		p = &fallback
	}
	if p != nil {
		if p.Sit {
			base = waterRiderActionBase
		} else if p.StandPose {
			base = standingFirst
		}
	}
	return base + int(direction)%nativeFacingDirections
}

// SetPetMountGeometry copies the two Npc.dat height flags used by
// FUN_002fe8e8 (saddle) and FUN_004265a4 (rider name clearance).
func (h *Human) SetPetMountGeometry(scale, preset byte) {
	h.petHeightScale, h.petHeightPreset = scale, preset
	if h.petMount != nil {
		h.petMount.SetHeightScale(scale)
	}
}

// ResolveMountPlacement preserves native precedence: authored saddle tables
// override the text-file offsets; authored seated poses/fixed-frame flags also
// override their respective file fields. Unlisted pets use the native +/-5 X.
func ResolveMountPlacement(sprite uint16, exported map[uint16]MountPlacement) MountPlacement {
	p, supplied := exported[sprite]
	native, known := nativeMountPlacements[sprite]
	if native.HasOffsets {
		p.Stand, p.Move = native.Placement.Stand, native.Placement.Move
	} else {
		// The native text parser reads MovePos, but FUN_0040e4b4 only uses
		// compiled movement corrections for sprites 1185 and 1456.
		p.Move = [4][8][2]int{}
		if !supplied {
			for body := range p.Stand {
				p.Stand[body][2][0], p.Stand[body][3][0] = 5, 5
				p.Stand[body][5][0], p.Stand[body][6][0] = -5, -5
			}
		}
	}
	if known {
		if native.Placement.Sit {
			p.Sit, p.StandPose = true, false
		}
		p.FixedFirst = p.FixedFirst || native.Placement.FixedFirst
	}
	if !supplied && !native.HasOffsets && p.Sit {
		p.Stand = [4][8][2]int{}
	}
	return p
}

const (
	petNormalSaddleBase       = 62
	petTallSaddleBase         = 130
	petScaledNormalSaddleBase = 94
	petScaledTallSaddleBase   = 330
	petCanvasGroundOffset     = 32
	petTallSpriteDrop         = 68
	petScaledSpriteDrop       = 100
	petNameClearanceBase      = 2
	petTallNameClearanceBase  = 66
	petScaledNameClearance    = 68
	petNameHeightNumerator    = 7
	petNameHeightDenominator  = 10
)

func petSpriteDrop(scale, preset byte) int {
	if preset != 1 {
		return 0
	}
	if scale == 1 {
		return petTallSpriteDrop + petScaledSpriteDrop
	}
	return petTallSpriteDrop
}

func (h *Human) petRiderOffset(action int) (int, int) {
	if h.petMount == nil || h.body < 1 || h.body > 4 {
		return 0, 0
	}
	p := h.petPlacement
	if p == nil {
		fallback := ResolveMountPlacement(uint16(h.petMount.Sprite()), nil)
		p = &fallback
	}
	frame := h.petMount.mountFrame(action, p.FixedFirst)
	if frame == nil {
		return 0, 0
	}
	// CanvasHeight - AnchorY, retained by editable and packed frame offsets.
	height := petCanvasGroundOffset - frame.OffsetY
	dy := 0
	switch {
	case h.petHeightScale == 0 && h.petHeightPreset == 0:
		dy = petNormalSaddleBase - height
	case h.petHeightScale == 0 && h.petHeightPreset == 1:
		dy = petTallSaddleBase - height
	case h.petHeightScale == 1 && h.petHeightPreset == 0:
		dy = petScaledNormalSaddleBase - 2*height
	case h.petHeightScale == 1 && h.petHeightPreset == 1:
		dy = petScaledTallSaddleBase - 2*height
	}
	offset := p.Stand[h.body-1][action%nativeFacingDirections]
	if action < nativeFacingDirections {
		delta := p.Move[h.body-1][action%nativeFacingDirections]
		offset[0], offset[1] = offset[0]+delta[0], offset[1]+delta[1]
	}
	return offset[0], dy + offset[1]
}

// NameLift is the mounted character's +0x2094 name clearance. Native names
// remain centred on the owner's world X, independent of facing/saddle X.
func (h *Human) NameLift() int {
	if h.petMount == nil {
		return 0
	}
	frame := h.petMount.mountFrame(0, true)
	if frame == nil {
		return 0
	}
	base := petNameClearanceBase
	factor := petNameHeightNumerator
	if h.petHeightPreset == 1 {
		base, factor = petTallNameClearanceBase, 1
	}
	if h.petHeightScale == 1 {
		base += petScaledNameClearance
	}
	return max(0, base+(frame.Height*factor+petNameHeightDenominator-1)/petNameHeightDenominator)
}
