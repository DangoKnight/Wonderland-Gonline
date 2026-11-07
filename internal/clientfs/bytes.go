package clientfs

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"
)

// ZipSource is a bundle whose bytes are read on demand. In the browser build
// the reader is backed by a Blob, so packs never need to fit in memory.
type ZipSource struct {
	R    io.ReaderAt
	Size int64
}

// MountBytes mounts an in-memory bundle. companions maps each pack name listed
// in _packs.json to its bytes.
func MountBytes(name string, data []byte, companions map[string][]byte) (string, func() error, error) {
	srcs := map[string]ZipSource{}
	for k, v := range companions {
		srcs[k] = ZipSource{R: bytes.NewReader(v), Size: int64(len(v))}
	}
	return MountSources(name, ZipSource{R: bytes.NewReader(data), Size: int64(len(data))}, srcs)
}

// MountSources mounts the main bundle and its companion packs. Validation
// matches Mount.
func MountSources(name string, main ZipSource, companions map[string]ZipSource) (string, func() error, error) {
	if name == "" || strings.ContainsAny(name, `\/:`) {
		return "", nil, fmt.Errorf("invalid bundle name %q", name)
	}
	z, err := zip.NewReader(main.R, main.Size)
	if err != nil {
		return "", nil, err
	}
	combined := &zip.Reader{File: append([]*zip.File(nil), z.File...)}
	for _, f := range z.File {
		if f.Name != "_packs.json" {
			continue
		}
		var descriptor struct {
			Version int      `json:"version"`
			Packs   []string `json:"packs"`
		}
		r, e := f.Open()
		if e != nil {
			return "", nil, e
		}
		e = json.NewDecoder(io.LimitReader(r, maxPackDescriptorBytes)).Decode(&descriptor)
		r.Close()
		if e != nil || descriptor.Version != runtimePackSetVersion || len(descriptor.Packs) > maxRuntimeCompanionPacks {
			return "", nil, fmt.Errorf("invalid asset pack set")
		}
		for _, pack := range descriptor.Packs {
			if !fs.ValidPath(pack) || strings.ContainsAny(pack, `/\:`) || !strings.HasSuffix(pack, ".zip") {
				return "", nil, fmt.Errorf("invalid companion pack %q", pack)
			}
			src, ok := companions[pack]
			if !ok {
				return "", nil, fmt.Errorf("missing companion pack %q", pack)
			}
			pz, e := zip.NewReader(src.R, src.Size)
			if e != nil {
				return "", nil, e
			}
			for _, entry := range pz.File {
				if entry.Name != "_bundle.json" {
					combined.File = append(combined.File, entry)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, f := range combined.File {
		entry := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(entry) || strings.ContainsAny(entry, `\:`) || seen[entry] || f.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("invalid bundle entry %q", f.Name)
		}
		seen[entry] = true
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
