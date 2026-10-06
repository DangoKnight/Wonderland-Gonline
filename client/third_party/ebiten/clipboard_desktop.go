//go:build (windows || darwin || linux || freebsd || netbsd) && !android && !ios

package ebiten

import (
	"errors"
	"github.com/hajimehoshi/ebiten/v2/internal/glfw"
	"github.com/hajimehoshi/ebiten/v2/internal/ui"
)

// ClipboardText reads desktop clipboard text on the window thread. Call while
// RunGame is running. This is an extension in Wonderland Gonline's local fork.
func ClipboardText() (string, error) {
	var text string
	err := errors.New("ebiten: clipboard unavailable before the game starts")
	ui.Get().RunOnMainThread(func() { text, err = glfw.GetClipboardString() })
	return text, err
}
