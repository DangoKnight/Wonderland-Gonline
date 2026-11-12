//go:build js && wasm

package userdata

import (
	"bytes"
	"fmt"
	"io/fs"
	"path/filepath"
	"syscall/js"
	"time"
)

const browserDocumentPrefix = "wonderland:user-document:"

type browserFile struct {
	*bytes.Reader
	name string
}

func (f *browserFile) Close() error               { return nil }
func (f *browserFile) Stat() (fs.FileInfo, error) { return browserFileInfo{f.name, f.Size()}, nil }

type browserFileInfo struct {
	name string
	size int64
}

func (f browserFileInfo) Name() string       { return filepath.Base(f.name) }
func (f browserFileInfo) Size() int64        { return f.size }
func (f browserFileInfo) Mode() fs.FileMode  { return 0600 }
func (f browserFileInfo) ModTime() time.Time { return time.Time{} }
func (f browserFileInfo) IsDir() bool        { return false }
func (f browserFileInfo) Sys() any           { return nil }

func Open(path string) (file File, err error) {
	defer func() {
		if recover() != nil {
			file = nil
			err = fmt.Errorf("browser preference storage unavailable")
		}
	}()
	value := js.Global().Get("localStorage").Call("getItem", browserDocumentPrefix+filepath.ToSlash(filepath.Clean(path)))
	if value.IsNull() {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return &browserFile{bytes.NewReader([]byte(value.String())), path}, nil
}
func WriteFile(path string, data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("browser preference storage unavailable or full")
		}
	}()
	js.Global().Get("localStorage").Call("setItem", browserDocumentPrefix+filepath.ToSlash(filepath.Clean(path)), string(data))
	return nil
}
