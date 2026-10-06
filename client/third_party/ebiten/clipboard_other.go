//go:build (!windows && !darwin && !linux && !freebsd && !netbsd) || android || ios

package ebiten

import "errors"

func ClipboardText() (string, error) {
	return "", errors.New("ebiten: clipboard text is unsupported on this platform")
}
