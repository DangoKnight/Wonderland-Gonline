package clientfs

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"
)

// MountPacks mounts several independent ZIP packs as one virtual root. Entry
// names must be unique across packs; metadata files are skipped.
func MountPacks(name string, packs []ZipSource) (string, func() error, error) {
	if name == "" || strings.ContainsAny(name, `\/:`) {
		return "", nil, fmt.Errorf("invalid bundle name %q", name)
	}
	combined := &zip.Reader{}
	seen := map[string]bool{}
	for _, p := range packs {
		zr, err := zip.NewReader(p.R, p.Size)
		if err != nil {
			return "", nil, err
		}
		for _, f := range zr.File {
			if f.Name == "_packs.json" || f.Name == "_bundle.json" {
				continue
			}
			entry := strings.TrimSuffix(f.Name, "/")
			if !fs.ValidPath(entry) || strings.ContainsAny(entry, `\:`) || seen[entry] || f.Mode()&os.ModeSymlink != 0 {
				return "", nil, fmt.Errorf("invalid bundle entry %q", f.Name)
			}
			seen[entry] = true
			combined.File = append(combined.File, f)
		}
	}
	mounts.Lock()
	if mounts.bundles[name] != "" {
		mounts.Unlock()
		return "", nil, fmt.Errorf("bundle already mounted: %s", name)
	}
	mounts.serial++
	root := fmt.Sprintf("/%s.assets-%d", name, mounts.serial)
	mounts.bundles[name] = root
	mounts.roots[root] = combined
	mounts.Unlock()
	var once sync.Once
	return root, func() error {
		once.Do(func() {
			mounts.Lock()
			delete(mounts.roots, root)
			delete(mounts.bundles, name)
			mounts.Unlock()
		})
		return nil
	}, nil
}
