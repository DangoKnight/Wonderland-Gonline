// Wonderland patch: custom cursor images (see PATCHES.md).

//go:build !android && !ios && !js && !nintendosdk && !playstation5

package ui

func (u *UserInterface) applyCustomCursor() {
	if u.isTerminated() {
		return
	}
	if b := u.runningBackend(); b != nil {
		b.applyCursorShape()
	}
}
