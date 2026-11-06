package app

import "wonderland-gonline/internal/localization"

import (
	"time"

	"wonderland-gonline/client/wlo/login"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

const (
	logoutWidth         = 280
	logoutHeight        = 200
	logoutPreviewAction = 13 // FUN_0034c814: human body mode, fixed first frame.
	logoutNameInk       = 0xffff
	logoutPromptInk     = 0xffe0
	logoutShadeAlpha    = 128
)

type logoutForm struct {
	seui.Form
	c          *Client
	background *surface.Surface
	preview    login.RoleView
}

func (f *logoutForm) Update(in *seui.Input) {
	if !f.Visible {
		return
	}
	f.Form.Update(in)
}

func (f *logoutForm) drawContent() {
	a := f.Abs()
	c := f.c
	if c.World == nil {
		return
	}
	if f.preview != nil {
		f.preview.DrawBody(f.Env.Screen, a.X+40, a.Y+110, logoutPreviewAction)
	}
	name := c.World.Player.Name
	c.Env.Text.Draw(a.X+40-len(name)*4, a.Y+123, 0, false, true, f.Env.Screen, name, 15, 160, 0, logoutNameInk, 0)
}
func (c *Client) logoutConfirmation(exit bool) {
	c.closeSettingsPrompt()
	f := &logoutForm{c: c, preview: c.Inventory.Preview}
	if human, ok := c.Inventory.Preview.(*role.Human); ok {
		preview := *human // Independent animation state; shared immutable sprite library.
		preview.Hold(0, false)
		f.preview = &preview
	}
	f.InitForm(f, c.Env)
	f.AnimateMessage(func() time.Time { return c.Now() })
	f.Dockable = false
	f.Name = localization.UI("ui.app.logout_confirmation_d9d7f0c2")
	f.Init("panel22", 255, 168, 324, 0, 0, true, logoutHeight, logoutWidth, 30)
	f.SetMargins(50, 32, 32, 60)
	text := seui.NewEditor(c.Env, f)
	text.Init("", 80, 0, 0, 0, 0, false, 30, 185, 30)
	text.ReadOnly = true
	text.Color = logoutPromptInk
	message := localization.UI("ui.app.want_to_log_out_ad0a6cfa")
	if exit {
		message = localization.UI("ui.app.want_to_exit_79bf8048")
	}
	text.BindLanguage("text", func() {
		message = localization.UI("ui.app.want_to_log_out_ad0a6cfa")
		if exit {
			message = localization.UI("ui.app.want_to_exit_79bf8048")
		}
		text.SetText([]byte(message))
	})
	for i := range 2 {
		confirm := i == 0
		b := seui.NewFixedButton(c.Env, f)
		asset := "btn_ok_1"
		if !confirm {
			asset = "btn_cancel_1"
		}
		b.Init(asset, 70+i*96, 20, 56, 0, 0, true, 20, 56, 150)
		b.Color = 0xffff
		b.OnClick = func() {
			c.closeSettingsPrompt()
			if confirm {
				c.stopRemote()
				c.Net.Close()
				if exit {
					c.Exit = true
				} else {
					c.clearProfileAccount()
					c.Settings.Hide()
					c.lostPrev()
				}
			}
		}
	}
	c.logoutPrompt = f
	c.settingsPromptForm = &f.Form
	c.UI.Add(f)
	c.UI.Modal = f
	f.Show()
}

// Panel22 supplies the native gold frame and green shade mask. This palette
// is measured from Screenshot_20261005_200128's blue confirmation background;
// preserve the exported dither and frame rather than replacing the asset.
func (f *logoutForm) Paint() {
	if f.background == nil {
		f.background = seui.ConfirmationBackground(f.Env, logoutWidth, logoutHeight)
	}
	f.Env.Screen.DrawStretch(f.MessageRect(), f.background, true)
	f.drawContent()
}
