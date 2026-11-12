// Package userdata stores small client-owned preferences and histories.
// Callers must not write passwords, deletion codes or server-owned resources.
package userdata

import (
	"io"
	"io/fs"
)

type File interface {
	io.Reader
	io.ReaderAt
	io.Closer
	Stat() (fs.FileInfo, error)
}

func ReadFile(path string) ([]byte, error) {
	f, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
