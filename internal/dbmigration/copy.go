// Package dbmigration prepares upgraded database copies without modifying sources.
package dbmigration

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"wonderland-go/internal/assetsql"
	"wonderland-go/internal/store"
)

// Copy upgrades one snapshot into a new destination. VACUUM INTO includes
// committed WAL data, unlike a filesystem copy. The source is opened read-only.
// Stop the server before taking both snapshots to retain their common epoch.
func Copy(source, destination string, assets bool) (err error) {
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	if source == destination {
		return fmt.Errorf("migration destination must differ from source")
	}
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source must be a regular database file")
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return err
	}
	// Reserve the destination so an existing database can never be overwritten.
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(destination)
			os.Remove(destination + "-wal")
			os.Remove(destination + "-shm")
		}
	}()
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(source)}
	q := address.Query()
	q.Set("mode", "ro")
	q.Set("_busy_timeout", "5000")
	address.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", address.String())
	if err != nil {
		return err
	}
	// SQLite accepts an existing empty destination; values are bound, never interpolated.
	_, err = db.Exec("VACUUM INTO ?", destination)
	closeErr := db.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if assets {
		err = assetsql.MigrateDatabase(destination)
	} else {
		var gameplay *store.Store
		gameplay, err = store.Open(destination)
		if err == nil {
			err = gameplay.Close()
		}
	}
	if err != nil {
		return err
	}
	return Check(destination)
}

func Check(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	address := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	q := address.Query()
	q.Set("mode", "ro")
	address.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", address.String())
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if strings.ToLower(result) != "ok" {
			rows.Close()
			return fmt.Errorf("SQLite integrity check: %s", result)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("SQLite foreign key check failed")
	}
	return rows.Err()
}
