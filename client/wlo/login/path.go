package login

import (
	"os"
	"path/filepath"
	"strings"
)

// Path joins parts under root, matching each existing component without
// regard to case as Windows does. Missing components keep the given case.
func Path(root string, parts ...string) string {
	dir := root
	for _, p := range parts {
		next := filepath.Join(dir, p)
		if _, err := os.Stat(next); err != nil {
			if ents, err := os.ReadDir(dir); err == nil {
				for _, e := range ents {
					if strings.EqualFold(e.Name(), p) {
						next = filepath.Join(dir, e.Name())
						break
					}
				}
			}
		}
		dir = next
	}
	return dir
}
