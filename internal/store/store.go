// Package store owns the Go server database. It never opens the C# runtime DB.
package store

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var ErrCredentials = errors.New("invalid credentials")
var ErrConflict = errors.New("account already exists")
var usernamePattern = regexp.MustCompile(fmt.Sprintf(`^[a-z0-9_]{%d,%d}$`, UsernameMinBytes, UsernameMaxBytes))

type Store struct {
	db  *sql.DB
	orm *gorm.DB
}
type Account struct {
	ID       uint32 `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Banned   bool   `json:"banned"`
	IM       int64  `json:"im"`
	IMBonus  int64  `json:"im_bonus"`
	// GMLevel is granted only by an administrator; any value above zero enables GM commands.
	GMLevel byte `json:"gm_level"`
}

func (a Account) UserID() uint32 { return a.ID + UserIDOffset }
func (a Account) CharacterID(slot byte) uint32 {
	if slot == 2 {
		return a.UserID() + SecondCharacterIDOffset
	}
	return a.UserID()
}
func Open(path string) (*Store, error) {
	abs, e := filepath.Abs(path)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(filepath.Dir(abs), 0700); e != nil {
		return nil, e
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	q := u.Query()
	q.Set("_busy_timeout", "5000")
	q.Set("_journal_mode", "WAL")
	q.Set("_foreign_keys", "on")
	u.RawQuery = q.Encode()
	db, e := sql.Open("sqlite3", u.String())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if e = s.migrate(); e != nil {
		db.Close()
		return nil, e
	}
	orm, e := gorm.Open(sqlite.New(sqlite.Config{Conn: db}), &gorm.Config{
		SkipDefaultTransaction: true,
		DisableAutomaticPing:   true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	})
	if e != nil {
		db.Close()
		return nil, e
	}
	s.orm = orm
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func ValidateCredentials(name, password string) error {
	if !usernamePattern.MatchString(strings.ToLower(name)) {
		return errors.New("username must be 4–14 ASCII letters, digits, or underscores")
	}
	return ValidatePassword(password)
}

func ValidatePassword(password string) error {
	if len(password) < PasswordMinBytes || len(password) > PasswordMaxBytes {
		return errors.New("password must be 4–14 bytes for the game client")
	}
	for _, b := range []byte(password) {
		if b < asciiPrintableMin || b > asciiPrintableMax {
			return errors.New("password must use printable ASCII")
		}
	}
	return nil
}
func hashPassword(password string) (string, error) {
	salt := make([]byte, passwordSaltBytes)
	if _, e := rand.Read(salt); e != nil {
		return "", e
	}
	key, e := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyBytes)
	if e != nil {
		return "", e
	}
	return passwordHashAlgorithm + "$" + strconv.Itoa(passwordIterations) + "$" + hex.EncodeToString(salt) + "$" + hex.EncodeToString(key), nil
}
func verifyPassword(password, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != passwordHashParts || parts[0] != passwordHashAlgorithm || parts[1] != strconv.Itoa(passwordIterations) {
		return false
	}
	salt, e := hex.DecodeString(parts[2])
	if e != nil || len(salt) != passwordSaltBytes {
		return false
	}
	want, e := hex.DecodeString(parts[3])
	if e != nil || len(want) != passwordKeyBytes {
		return false
	}
	got, e := pbkdf2.Key(sha256.New, password, salt, passwordIterations, passwordKeyBytes)
	return e == nil && subtle.ConstantTimeCompare(got, want) == 1
}
