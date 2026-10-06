// Package clientfs resolves client assets from directories or mounted bundles.
// Writable preferences never belong to a mount; missing bundled files cannot
// fall back to a developer's loose source tree.
package clientfs

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	runtimePackSetVersion    = 1
	maxRuntimeCompanionPacks = 16
	maxPackDescriptorBytes   = 64 << 10
)

var mounts = struct {
	sync.RWMutex
	roots   map[string]fs.FS
	bundles map[string]string
	serial  uint64
}{roots: map[string]fs.FS{}, bundles: map[string]string{}}

// Mount gives a bundle its own virtual root without extracting files to disk.
func Mount(path string) (string, func() error, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", nil, err
	}
	z, err := zip.OpenReader(path)
	if err != nil {
		return "", nil, err
	}
	readers := []*zip.ReadCloser{z}
	closeReaders := func() error {
		var first error
		for _, r := range readers {
			if e := r.Close(); e != nil && first == nil {
				first = e
			}
		}
		return first
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
			closeReaders()
			return "", nil, e
		}
		e = json.NewDecoder(io.LimitReader(r, maxPackDescriptorBytes)).Decode(&descriptor)
		r.Close()
		if e != nil || descriptor.Version != runtimePackSetVersion || len(descriptor.Packs) > maxRuntimeCompanionPacks {
			closeReaders()
			return "", nil, fmt.Errorf("invalid asset pack set")
		}
		for _, name := range descriptor.Packs {
			if !fs.ValidPath(name) || strings.ContainsAny(name, "/\\:") || !strings.HasSuffix(name, ".zip") || name == filepath.Base(path) {
				closeReaders()
				return "", nil, fmt.Errorf("invalid companion pack %q", name)
			}
			pack, e := zip.OpenReader(filepath.Join(filepath.Dir(path), name))
			if e != nil {
				closeReaders()
				return "", nil, e
			}
			readers = append(readers, pack)
			for _, entry := range pack.File {
				if entry.Name != "_bundle.json" {
					combined.File = append(combined.File, entry)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, f := range combined.File {
		name := strings.TrimSuffix(f.Name, "/")
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") || seen[name] || f.Mode()&os.ModeSymlink != 0 {
			closeReaders()
			return "", nil, fmt.Errorf("invalid bundle entry %q", f.Name)
		}
		seen[name] = true
	}
	mounts.Lock()
	if mounts.bundles[path] != "" {
		mounts.Unlock()
		closeReaders()
		return "", nil, fmt.Errorf("bundle already mounted: %s", path)
	}
	mounts.serial++
	root := fmt.Sprintf("%s.assets-%d", path, mounts.serial)
	mounts.bundles[path] = root
	mounts.roots[root] = combined
	mounts.Unlock()
	var once sync.Once
	var closeErr error
	return root, func() error {
		once.Do(func() {
			mounts.Lock()
			delete(mounts.roots, root)
			delete(mounts.bundles, path)
			mounts.Unlock()
			closeErr = closeReaders()
		})
		return closeErr
	}, nil
}

func resolve(path string) (fs.FS, string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, ""
	}
	mounts.RLock()
	defer mounts.RUnlock()
	for root, system := range mounts.roots {
		rel, err := filepath.Rel(root, abs)
		if err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return system, filepath.ToSlash(rel)
		}
	}
	return nil, ""
}

// File allows existing image decoders to rewind a packed file. Streaming reads
// stay streaming; only Seek materializes that one entry, never the whole pack.
type File interface {
	io.Reader
	io.Seeker
	io.Closer
	Stat() (fs.FileInfo, error)
}
type packedFile struct {
	fs.File
	system   fs.FS
	name     string
	seek     *bytes.Reader
	position int64
}

func (f *packedFile) Read(p []byte) (int, error) {
	if f.seek != nil {
		return f.seek.Read(p)
	}
	n, err := f.File.Read(p)
	f.position += int64(n)
	return n, err
}
func (f *packedFile) Seek(offset int64, whence int) (int64, error) {
	if f.seek == nil {
		data, err := fs.ReadFile(f.system, f.name)
		if err != nil {
			return 0, err
		}
		f.seek = bytes.NewReader(data)
		if _, err = f.seek.Seek(f.position, io.SeekStart); err != nil {
			return 0, err
		}
	}
	return f.seek.Seek(offset, whence)
}
func Open(path string) (File, error) {
	if system, name := resolve(path); system != nil {
		f, err := system.Open(name)
		if err != nil {
			return nil, err
		}
		return &packedFile{File: f, system: system, name: name}, nil
	}
	return os.Open(path)
}
func ReadFile(path string) ([]byte, error) {
	if system, name := resolve(path); system != nil {
		return fs.ReadFile(system, name)
	}
	return os.ReadFile(path)
}
func Stat(path string) (fs.FileInfo, error) {
	if system, name := resolve(path); system != nil {
		return fs.Stat(system, name)
	}
	return os.Stat(path)
}
func ReadDir(path string) ([]fs.DirEntry, error) {
	if system, name := resolve(path); system != nil {
		return fs.ReadDir(system, name)
	}
	return os.ReadDir(path)
}
