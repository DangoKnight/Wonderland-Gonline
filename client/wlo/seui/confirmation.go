package seui

import "wonderland-gonline/client/wlo/surface"

const (
	confirmationMaskRed   = 96
	confirmationMaskGreen = 128
	confirmationMaskBlue  = 192
)

// ConfirmationBackground retains Panel22's native gold frame and dither mask.
// Logout, party invitations and pet dismissal use this same shade conversion.
// The returned surface belongs to the caller and must be closed on dismissal.
func ConfirmationBackground(env *Env, width, height int) *surface.Surface {
	background := surface.New(width, height)
	local := *env
	local.Screen = background
	panel := NewPanel(&local, nil)
	panel.Init("panel22", 0, 168, 324, 0, 0, true, height, width, 0)
	panel.SetMargins(50, 32, 32, 60)
	panel.Paint()
	for at, v := range background.Pix {
		if v != 0 && v&0xf81f == 0 {
			shade := int(v>>5&63) * 255 / 63
			background.Pix[at] = surface.RGB565(uint8(shade*confirmationMaskRed/255), uint8(shade*confirmationMaskGreen/255), uint8(shade*confirmationMaskBlue/255))
		}
	}
	return background
}
