//go:build !js || !wasm

package userdata

import (
	"os"
	"path/filepath"
)

func Open(path string) (File, error) { return os.Open(path) }

// WriteFile replaces a complete document only after a successful write/sync.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".client-preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closed := f.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	return os.Rename(f.Name(), path)
}
