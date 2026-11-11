// Package debuglog records one client session into a text file: the packets
// that cross the login socket in both directions plus session diagnostics.
// Credential-bearing command families are recorded without their payloads.
//
// Recording is off unless the player enables it in the settings. A long session
// with recording on produces a large file, so the settings also cap the file at
// the last N lines; a cap of zero keeps every line.
package debuglog

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"wonderland-gonline/internal/protocol"
)

// Direction markers written before a packet.
const (
	In  byte = '<' // received
	Out byte = '>' // sent
)

const (
	// FilePrefix and FileExtension name a session file.
	FilePrefix    = "session-"
	FileExtension = ".txt"
	// timeFormat is the session timestamp embedded in the file name.
	timeFormat = "20060102-150405"
	// lineTimeFormat is the timestamp written before every line.
	lineTimeFormat = "2006-01-02 15:04:05.000"
)

// LimitSteps are the line caps offered in the settings dialog. Zero is
// unlimited, which is the only value that can grow without bound.
var LimitSteps = []int{0, 1000, 5000, 20000, 100000}

// DefaultLimit is the cap a fresh profile starts with.
const DefaultLimit = 5000

// MaxLimit bounds a hand-edited settings value.
const MaxLimit = 1_000_000

// Log appends timestamped lines to one session file. It is safe for concurrent
// use: the socket reader, the frame loop and the standard logger all write.
type Log struct {
	mu     sync.Mutex
	file   *os.File
	buffer *bufio.Writer
	path   string
	suffix string
	max    int
	ring   []string
	onDisk int
	closed bool
	err    error // first write/trim failure, reported when the recorder is closed
	now    func() time.Time
}

// Open creates the session file in dir and returns a log that writes to it.
// maxLines caps the file at the last maxLines lines; zero keeps every line.
// suffix distinguishes concurrent windows and is written before the extension,
// for example "-screen2".
func Open(dir string, maxLines int, at time.Time, suffix string) (*Log, error) {
	maxLines = NormalizeLimit(maxLines)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path, file, err := createSessionFile(dir, at, suffix)
	if err != nil {
		return nil, err
	}
	return &Log{file: file, buffer: bufio.NewWriter(file), path: path, suffix: suffix, max: maxLines, now: time.Now}, nil
}

// createSessionFile opens a new file for the session stamp, adding a counter
// when two sessions start inside the same second.
func createSessionFile(dir string, at time.Time, suffix string) (string, *os.File, error) {
	stamp := FilePrefix + at.Format(timeFormat) + suffix
	for n := range 100 {
		name := stamp + FileExtension
		if n > 0 {
			name = fmt.Sprintf("%s-%d%s", stamp, n+1, FileExtension)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return path, file, nil
		}
		if !os.IsExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("debuglog: no free session name in %s", dir)
}

// NormalizeLimit clamps a settings value into the documented range.
func NormalizeLimit(maxLines int) int {
	return min(max(0, maxLines), MaxLimit)
}

// WriteCrash stores a panic report beside the session logs, so a crash leaves a
// trace even when the process dies before anything else reaches the disk.
func WriteCrash(dir string, at time.Time, value any, stack []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "crash-"+at.Format(timeFormat)+FileExtension)
	body := fmt.Sprintf("%s panic: %v\n\n%s", at.Format(lineTimeFormat), value, stack)
	return path, os.WriteFile(path, []byte(body), 0o600)
}

// Path is the session file this log writes.
func (l *Log) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// MaxLines is the active line cap; zero means unlimited.
func (l *Log) MaxLines() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.max
}

// Suffix is the window marker this log was opened with.
func (l *Log) Suffix() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.suffix
}

// Printf timestamps each line separately, including multi-line panic reports.
func (l *Log) Printf(format string, args ...any) {
	_, _ = l.Write([]byte(fmt.Sprintf(format, args...)))
}

// Packet writes one socket packet as a direction marker, its command bytes and
// a hex payload. Authentication, character creation/deletion and bank messages
// may contain passwords, secret codes or PINs; omit the entire payload in both
// directions, including malformed or future variants in those command families.
func (l *Log) Packet(direction byte, p []byte) {
	l.entry(describe(direction, p))
}

// Write lets the log stand in for the standard logger's output.
func (l *Log) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			l.entry(line)
		}
	}
	return len(p), nil
}

// Close flushes the file and releases it.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return l.err
	}
	l.closed = true
	if l.max > 0 {
		l.recordError(l.rewriteLocked())
	}
	l.err = errors.Join(l.err, l.buffer.Flush(), l.file.Close())
	return l.err
}

// describe renders a packet as "< len=12 7:0 0700…".
func describe(direction byte, p []byte) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[packet] %c len=%d", direction, len(p))
	if len(p) > 0 {
		if len(p) > 1 {
			fmt.Fprintf(&b, " %d:%d", p[0], p[1])
		} else {
			fmt.Fprintf(&b, " %d", p[0])
		}
	}
	if len(p) > 0 {
		switch p[0] {
		case protocol.CommandLogin, protocol.CommandCharacterCreation, protocol.CommandCharacterSelection, protocol.CommandBank:
			b.WriteString(" [credential payload redacted]")
			return b.String()
		}
		b.WriteString(" ")
		b.WriteString(hex.EncodeToString(p))
	}
	return b.String()
}

func (l *Log) entry(text string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	l.appendLocked(l.now().Format(lineTimeFormat) + " " + text)
}

func (l *Log) appendLocked(line string) {
	if _, err := l.buffer.WriteString(line + "\n"); err != nil {
		l.recordError(err)
		return
	}
	// Flush every line: a transcript is worthless if a crash or a kill loses it,
	// and this is a debugging aid, not a hot path.
	if err := l.buffer.Flush(); err != nil {
		l.recordError(err)
		return
	}
	if l.max == 0 {
		return
	}
	l.ring = append(l.ring, line)
	if len(l.ring) > l.max {
		l.ring = l.ring[len(l.ring)-l.max:]
	}
	l.onDisk++
	// Trim only with slack, so a capped session rewrites rarely instead of on
	// every line past the cap.
	if l.onDisk >= l.max+capSlack(l.max) {
		l.recordError(l.rewriteLocked())
	}
}

// capSlack is how far past the cap the file may grow before it is trimmed.
func capSlack(maxLines int) int {
	return max(16, maxLines/8)
}

// rewriteLocked replaces the file with the retained lines.
func (l *Log) rewriteLocked() error {
	if err := l.buffer.Flush(); err != nil {
		return err
	}
	if err := l.file.Truncate(0); err != nil {
		return err
	}
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	for _, line := range l.ring {
		if _, err := io.WriteString(l.file, line+"\n"); err != nil {
			return err
		}
	}
	l.onDisk = len(l.ring)
	return nil
}

// Caller holds mu. Preserve the first failure even if a later flush succeeds.
func (l *Log) recordError(err error) {
	if l.err == nil {
		l.err = err
	}
}
