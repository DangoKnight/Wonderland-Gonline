package app

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"wonderland-gonline/client/wlo/viewer"
	"wonderland-gonline/internal/localization"
)

const spectatorButtonHeight = 18

// AddSpectator creates a regular client session sharing the same renderer,
// settings, artwork, sidebar and window as the rest of the workspace. Only its
// gameplay source and input destination differ. Capabilities remain transient.
func (w *Workspace) AddSpectator(address string) error {
	connection, err := viewer.New(address)
	if err != nil {
		return err
	}
	if err = w.Add(); err != nil {
		connection.Close()
		return err
	}
	c := w.Current()
	c.spectator = connection
	c.replica = &nativeReplica{}
	c.Net.SetPresentationOnly()
	c.Screen.Fill(image.Rect(0, 0, ScreenWidth, ScreenHeight), 0)
	return nil
}

func (c *Client) spectatorFrame(draw bool) { c.renderNativeSpectator(draw) }
func (w *Workspace) spectatorControlRect(row int) image.Rectangle {
	r := w.cardRect(row)
	return image.Rect(r.Min.X+sessionPanelPadding, r.Max.Y-spectatorButtonHeight-2, r.Max.X-sessionPanelPadding, r.Max.Y-2)
}
func (w *Workspace) drawSpectatorControl(row int, c *Client) {
	r := w.spectatorControlRect(row)
	state := c.spectator.State()
	caption := localization.UI("ui.spectator.take_control")
	border := uint16(sessionBorderInk)
	if state.Error != "" {
		caption = localization.UI("ui.spectator.stream_error")
	} else if !state.Connected {
		caption = localization.UI("ui.spectator.connecting")
	} else if !state.Ready {
		caption = localization.UI("ui.spectator.synchronizing")
	} else if state.Owns {
		caption = localization.UI("ui.spectator.release_control")
		border = sessionAccentInk
	}
	w.screen.Fill(r, sessionRowFill)
	w.screen.Frame(r, border)
	w.Current().Env.Text.Draw(r.Min.X+6, r.Min.Y+2, 0, false, true, w.screen, []byte(caption), 15, r.Dx()-12, 0, sessionPanelInk, 0)
}
func (g *Game) updateSpectatorInput(blocked bool) {
	c := g.C
	x, y, inside := g.Workspace.GamePoint(g.workspaceCursor())
	blocked = blocked || !inside || !ebiten.IsFocused() || !c.spectator.State().Ready
	if blocked {
		if !c.spectatorBlocked {
			c.spectator.ResetInput()
		}
		c.spectatorBlocked = true
		return
	}
	c.spectatorBlocked = false
	input := func(kind string, fields map[string]any) {
		if fields == nil {
			fields = map[string]any{}
		}
		fields["x"], fields["y"], fields["shift"] = x, y, g.shift()
		c.spectator.Input(kind, fields)
	}
	if c.Input.X != x || c.Input.Y != y {
		input("move", nil)
		c.Input.X, c.Input.Y = x, y
	}
	for i, button := range []ebiten.MouseButton{ebiten.MouseButtonLeft, ebiten.MouseButtonRight} {
		if inpututil.IsMouseButtonJustPressed(button) {
			input("down", map[string]any{"button": i})
		}
		if inpututil.IsMouseButtonJustReleased(button) {
			input("up", map[string]any{"button": i})
		}
	}
	_, wheel := ebiten.Wheel()
	if wheel != 0 {
		input("wheel", map[string]any{"down": wheel > 0})
	}
	for key, vk := range virtualKeys {
		if g.suppressedKeys[key] {
			if !ebiten.IsKeyPressed(key) {
				delete(g.suppressedKeys, key)
			}
			continue
		}
		duration := inpututil.KeyPressDuration(key)
		if duration == 1 || duration > keyRepeatDelayTicks && (duration-keyRepeatDelayTicks)%keyRepeatEveryTicks == 0 {
			input("key", map[string]any{"key": vk, "down": true})
		}
		if inpututil.IsKeyJustReleased(key) {
			input("key", map[string]any{"key": vk, "down": false})
		}
	}
	if len(g.suppressedKeys) == 0 {
		if text := string(ebiten.AppendInputChars(nil)); text != "" {
			input("text", map[string]any{"text": text})
		}
	}
}

func spectatorTitle(id int) string { return fmt.Sprintf(localization.Text("ui.spectator.title"), id) }
