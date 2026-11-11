package app

import (
	"image"
	"time"
	"wonderland-gonline/internal/clientassets"
)

const sessionAFKWidth = 42

func (w *Workspace) afkRect(row int) image.Rectangle {
	r := w.cardRect(row)
	return image.Rect(r.Max.X-sessionCloseWidth-sessionAFKWidth, r.Min.Y+1, r.Max.X-sessionCloseWidth-2, r.Min.Y+sessionPreviewTop)
}

func (w *Workspace) toggleSessionAFK(c *Client) {
	if !c.AFKSupported() || c.World == nil || c.afk.pending || c.afk.resync {
		return
	}
	if err := c.SetAFK(!c.AFKActive()); err != nil {
		c.Notices.Show(clientassets.Big5Text(err.Error()), 3*time.Second, c.Now())
	}
}

func (w *Workspace) drawSessionAFK(row int, c *Client) {
	r := w.afkRect(row)
	fill, border, ink := uint16(sessionRowFill), uint16(sessionBorderInk), uint16(sessionPanelInk)
	enabled := c.AFKSupported() && c.World != nil && !c.afk.pending && !c.afk.resync
	if !enabled {
		ink = sessionBorderInk
	}
	if enabled && w.hover.In(r) {
		fill = sessionHoverFill
	}
	if c.AFKActive() {
		fill, border = sessionActiveFill, sessionAccentInk
	}
	caption := "AFK"
	if c.afk.pending || c.afk.resync {
		caption = "..."
	}
	w.screen.Fill(r, fill)
	w.screen.Frame(r, border)
	w.Current().Env.Text.Draw(r.Min.X+6, r.Min.Y+2, 0, false, true, w.screen, []byte(caption), 15, r.Dx()-8, 0, ink, 0)
}
