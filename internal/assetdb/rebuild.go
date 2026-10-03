package assetdb

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wonderland-go/internal/store"
)

type RebuildOptions struct {
	DataDirectory string
	AssetsDB      string
	GameplayDB    string // Empty leaves the gameplay database untouched.
	Progress      func(string)
}
type RebuildResult struct {
	Summary
	AssetsDB, GameplayDB string
	Backups              []string
}

// Rebuild prepares both databases before replacing either. Existing files are
// renamed to unique backups; publication errors restore previously replaced
// files. The caller must stop the server and close database clients first.
func Rebuild(options RebuildOptions) (result RebuildResult, err error) {
	root, err := filepath.Abs(options.DataDirectory)
	if err != nil {
		return result, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return result, err
	}
	targets := []string{options.AssetsDB}
	if options.GameplayDB != "" {
		targets = append(targets, options.GameplayDB)
	}
	staged := make([]string, 0, len(targets))
	locks := []string{}
	defer func() {
		for _, path := range staged {
			os.Remove(path)
			os.Remove(path + "-wal")
			os.Remove(path + "-shm")
		}
		for _, path := range locks {
			os.Remove(path)
		}
	}()
	for index, target := range targets {
		target, err = filepath.Abs(target)
		if err != nil {
			return result, err
		}
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return result, err
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(target))
		if err != nil {
			return result, err
		}
		target = filepath.Join(parent, filepath.Base(target))
		relative, err := filepath.Rel(root, target)
		if err != nil {
			return result, err
		}
		if relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return result, fmt.Errorf("database destination must be outside exported data: %s", target)
		}
		if index > 0 && target == targets[0] {
			return result, fmt.Errorf("assets and gameplay destinations must differ")
		}
		targets[index] = target
		if info, e := os.Lstat(target); e == nil {
			if !info.Mode().IsRegular() {
				return result, fmt.Errorf("database target must be a regular file: %s", target)
			}
		} else if !os.IsNotExist(e) {
			return result, e
		}
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, e := os.Stat(target + suffix); !os.IsNotExist(e) {
				return result, fmt.Errorf("close server/database clients first: SQLite sidecar present at %s", target+suffix)
			}
		}
		if e := validateTarget(target, index == 0); e != nil {
			return result, e
		}
		lock := target + ".rebuild.lock"
		file, e := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return result, fmt.Errorf("acquire rebuild lock: %w", e)
		}
		locks = append(locks, lock)
		if e = file.Close(); e != nil {
			return result, e
		}
		temporary, e := os.CreateTemp(parent, ".database-rebuild-*.db")
		if e != nil {
			return result, e
		}
		path := temporary.Name()
		if e = temporary.Close(); e != nil {
			return result, e
		}
		staged = append(staged, path)
		if e = os.Remove(path); e != nil {
			return result, e
		}
	}
	result.Summary, err = Build(root, staged[0], options.Progress)
	if err != nil {
		return result, err
	}
	if options.GameplayDB != "" {
		gameplay, e := store.Open(staged[1])
		if e != nil {
			return result, e
		}
		if e = gameplay.Close(); e != nil {
			return result, e
		}
	}
	for _, path := range staged {
		if e := os.Chmod(path, 0600); e != nil {
			return result, e
		}
		file, e := os.OpenFile(path, os.O_RDWR, 0)
		if e != nil {
			return result, e
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return result, syncErr
		}
		if closeErr != nil {
			return result, closeErr
		}
	}
	backups := make([]string, len(targets))
	published := 0
	rollback := func(cause error) error {
		failures := []string{}
		for index := published - 1; index >= 0; index-- {
			if e := os.Remove(targets[index]); e != nil {
				failures = append(failures, e.Error())
				continue
			}
			if backups[index] != "" {
				if e := os.Rename(backups[index], targets[index]); e != nil {
					failures = append(failures, e.Error())
				}
			}
		}
		if len(failures) > 0 {
			return fmt.Errorf("%w; rollback errors: %s; backups: %v", cause, strings.Join(failures, "; "), backups)
		}
		return cause
	}
	for index, target := range targets {
		// Recheck after the potentially long import; an active server must not have
		// appeared while preparing databases. WAL/SHM journals stay with their owner.
		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			if _, e := os.Stat(target + suffix); !os.IsNotExist(e) {
				return result, rollback(fmt.Errorf("SQLite client became active at %s", target))
			}
		}
		if e := validateTarget(target, index == 0); e != nil {
			return result, rollback(e)
		}
		if _, e := os.Stat(target); e == nil {
			backup, e := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".backup-"+time.Now().UTC().Format("20060102T150405Z")+"-*")
			if e != nil {
				return result, rollback(e)
			}
			name := backup.Name()
			if e = backup.Close(); e != nil {
				return result, rollback(e)
			}
			if e = os.Remove(name); e != nil {
				return result, rollback(e)
			}
			if e = os.Rename(target, name); e != nil {
				return result, rollback(e)
			}
			backups[index] = name
		} else if !os.IsNotExist(e) {
			return result, rollback(e)
		}
		if e := os.Rename(staged[index], target); e != nil {
			if backups[index] != "" {
				if restoreErr := os.Rename(backups[index], target); restoreErr != nil {
					return result, rollback(fmt.Errorf("%w; restore failed: %v; backup %s", e, restoreErr, backups[index]))
				}
			}
			return result, rollback(e)
		}
		published++
	}
	result.AssetsDB = targets[0]
	if len(targets) > 1 {
		result.GameplayDB = targets[1]
	}
	for _, backup := range backups {
		if backup != "" {
			result.Backups = append(result.Backups, backup)
		}
	}
	return result, nil
}

// Refuse accidental replacement of arbitrary files or the other database role.
// On Linux, also catch open readers which may not have a SQLite sidecar.
func validateTarget(path string, assets bool) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if runtime.GOOS == "linux" {
		processes, err := os.ReadDir("/proc")
		if err != nil {
			return err
		}
		for _, process := range processes {
			descriptors, err := os.ReadDir(filepath.Join("/proc", process.Name(), "fd"))
			if err != nil {
				continue
			}
			for _, descriptor := range descriptors {
				openFile, err := os.Stat(filepath.Join("/proc", process.Name(), "fd", descriptor.Name()))
				if err == nil && os.SameFile(info, openFile) {
					return fmt.Errorf("close process %s before replacing %s", process.Name(), path)
				}
			}
		}
	}
	db, err := openDatabase(path, true)
	if err != nil {
		return fmt.Errorf("existing target is not a readable SQLite database: %w", err)
	}
	defer Close(db)
	expected, forbidden := "accounts", "asset_documents"
	if assets {
		expected, forbidden = forbidden, expected
	}
	if !db.Migrator().HasTable(expected) || db.Migrator().HasTable(forbidden) {
		return fmt.Errorf("existing target has an unexpected database role: %s", path)
	}
	return nil
}
