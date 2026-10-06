package login

import (
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/clientfs"
	"wonderland-gonline/internal/clientimage"
)

// Path joins parts under root, matching each existing component without
// regard to case as Windows does. Missing components keep the given case.
func Path(root string, parts ...string) string {
	dir := root
	for _, p := range parts {
		next := filepath.Join(dir, p)
		if _, err := clientfs.Stat(next); err != nil {
			if ents, err := clientfs.ReadDir(dir); err == nil {
				for _, e := range ents {
					if strings.EqualFold(strings.TrimSuffix(e.Name(), clientimage.DescriptorSuffix), p) {
						next = filepath.Join(dir, strings.TrimSuffix(e.Name(), clientimage.DescriptorSuffix))
						break
					}
				}
			}
		}
		dir = next
	}
	return dir
}
