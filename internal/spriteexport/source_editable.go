package spriteexport

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"wonderland-gonline/internal/spritepack"
)

// ensureSpriteOutputOutside accepts absolute paths on different Windows volumes:
// filepath.Rel cannot represent that relationship, but they cannot overlap.
// Same-volume paths still reject the source itself and every descendant.
func ensureSpriteOutputOutside(source, output string) error {
	if !strings.EqualFold(filepath.VolumeName(source), filepath.VolumeName(output)) {
		return nil
	}
	relative, err := filepath.Rel(source, output)
	if err != nil {
		return err
	}
	if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("output must be outside original sprite source directory")
	}
	return nil
}

// exportEditableFromSource makes a fresh native decode in temporary storage.
// Existing data/ files are outputs only; none supply palettes, pixels, actions,
// encryption keys or other inputs for this regeneration.
func exportEditableFromSource(input, executable, output, selected string, overwrite bool) error {
	inputPath, err := filepath.Abs(input)
	if err != nil {
		return err
	}
	executablePath, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	outputPath, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	for _, root := range []string{inputPath, filepath.Dir(executablePath)} {
		if err := ensureSpriteOutputOutside(root, outputPath); err != nil {
			return err
		}
	}
	if !overwrite {
		dirs, err := os.ReadDir(output)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, d := range dirs {
			if !d.IsDir() || selected != "" && !strings.EqualFold(selected, d.Name()) {
				continue
			}
			path := filepath.Join(output, d.Name(), "editable.json")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("editable sprites already exist at %s; export to a new directory or use -overwrite-edits", path)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	temporary, err := os.MkdirTemp("", "wonderland-sprite-source-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	if err = run(input, executable, temporary); err != nil {
		return err
	}
	if err = exportEditable(temporary, selected, false); err != nil {
		return err
	}
	if err = spritepack.OptimizeEditableAll(temporary, []string{selected}, func(format string, args ...any) {
		fmt.Printf(format+"\n", args...)
	}); err != nil {
		return err
	}
	return filepath.WalkDir(temporary, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(temporary, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(output, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0755)
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		target, err := os.CreateTemp(filepath.Dir(destination), ".sprite-publish-*")
		if err != nil {
			return err
		}
		defer os.Remove(target.Name())
		_, copyErr := io.Copy(target, source)
		closeErr := target.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err = os.Chmod(target.Name(), 0644); err != nil {
			return err
		}
		return os.Rename(target.Name(), destination)
	})
}
