package dbmigration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Target is one configured database to upgrade.
type Target struct {
	Path   string
	Assets bool
}

// sidecars are the SQLite files that travel with a database.
var sidecars = []string{"", "-wal", "-shm", "-journal"}

// stagingDir and backupDir are created next to each database, with stamp
// appended, so every move stays on one filesystem.
const (
	stagingDir = ".migrate-"
	backupDir  = "backups"
	backupName = "pre-migrate-"
)

// UpgradeInPlace leaves the upgraded databases at their configured paths.
// It first builds and verifies every copy (Copy) in a staging directory
// beside its database; only when all succeed does it move each original,
// with its WAL and shared-memory files, into backups/pre-migrate-<stamp>/
// and the copy into its place. Any failure restores the originals and
// removes the staging files. It returns the backup directories used.
func UpgradeInPlace(targets []Target, stamp string) (backups []string, err error) {
	type staged struct {
		target, copy, backup string
	}
	var plan []staged
	stages := map[string]bool{}
	defer func() {
		for dir := range stages {
			os.RemoveAll(dir)
		}
	}()
	for _, t := range targets {
		path, err := filepath.Abs(t.Path)
		if err != nil {
			return nil, err
		}
		dir := filepath.Dir(path)
		stage := filepath.Join(dir, stagingDir+stamp)
		if !stages[stage] {
			if err = os.Mkdir(stage, 0700); err != nil {
				return nil, err
			}
			stages[stage] = true
		}
		copy := filepath.Join(stage, filepath.Base(path))
		fmt.Println("Preparing", t.Path)
		if err = Copy(path, copy, t.Assets); err != nil {
			return nil, fmt.Errorf("%s: %w", t.Path, err)
		}
		fmt.Println("Verified", t.Path)
		plan = append(plan, staged{path, copy, filepath.Join(dir, backupDir, backupName+stamp)})
	}
	// moved records each completed rename so a failure can undo them.
	type move struct{ from, to string }
	var moved []move
	rename := func(from, to string) error {
		// Rename replaces silently; never overwrite a backup or database.
		if _, err := os.Lstat(to); !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s already exists", to)
		}
		if err := os.Rename(from, to); err != nil {
			return err
		}
		moved = append(moved, move{from, to})
		return nil
	}
	defer func() {
		if err == nil {
			return
		}
		for i := len(moved) - 1; i >= 0; i-- {
			if e := os.Rename(moved[i].to, moved[i].from); e != nil {
				err = errors.Join(err, fmt.Errorf("restore %s: %w", moved[i].from, e))
			}
		}
		backups = nil
	}()
	seen := map[string]bool{}
	for _, p := range plan {
		if err = os.MkdirAll(p.backup, 0700); err != nil {
			return nil, err
		}
		if !seen[p.backup] {
			seen[p.backup] = true
			backups = append(backups, p.backup)
		}
		for _, s := range sidecars {
			from := p.target + s
			if _, e := os.Lstat(from); errors.Is(e, os.ErrNotExist) {
				continue
			}
			if err = rename(from, filepath.Join(p.backup, filepath.Base(from))); err != nil {
				return nil, err
			}
		}
		for _, s := range sidecars {
			from := p.copy + s
			if _, e := os.Lstat(from); errors.Is(e, os.ErrNotExist) {
				continue
			}
			if err = rename(from, p.target+s); err != nil {
				return nil, err
			}
		}
	}
	return backups, nil
}
