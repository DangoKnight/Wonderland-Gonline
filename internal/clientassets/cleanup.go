package clientassets

import (
	"io/fs"
	"os"
	"path/filepath"
)

// PruneEmptyDirectories removes only empty descendants, leaving root and files.
func PruneEmptyDirectories(root string) (int, error) {
	var directories []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && path != root {
			directories = append(directories, path)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	count := 0
	for i := len(directories) - 1; i >= 0; i-- {
		entries, err := os.ReadDir(directories[i])
		if err != nil {
			return count, err
		}
		if len(entries) == 0 {
			if err = os.Remove(directories[i]); err != nil {
				return count, err
			}
			count++
		}
	}
	return count, nil
}
