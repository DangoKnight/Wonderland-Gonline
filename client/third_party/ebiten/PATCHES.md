# Local patches to Ebitengine v2.10.0

This is a copy of `github.com/hajimehoshi/ebiten/v2` v2.10.0, used through a
`replace` directive in `client/go.mod`. Examples, tests, test data and `cmd/`
were left out.

## Custom cursor images

The client shows the original's animated cursors as real system cursors, as
the Windows client did, instead of drawing them into the frame. Ebitengine
has no public API for that, although its GLFW layer implements
`CreateCursor` for Win32, X11 and Cocoa.

- `cursorimage.go`: `NewCursorImage` and `SetCursorImage`.
- `internal/ui/cursor_image*.go`: the stored custom cursor and its apply.
- `internal/ui/ui_glfw.go`: `cursorToApply`, used where the backend set the
  system cursor (window creation, cursor mode, cursor shape).

To update Ebitengine, copy the new version the same way and reapply these
changes.

`go vet` on client packages now also analyses this copy and repeats an
upstream warning (`internal/ui/api_linbsd.go`, a C pointer from XRandR); the
upstream code is left as it is.
