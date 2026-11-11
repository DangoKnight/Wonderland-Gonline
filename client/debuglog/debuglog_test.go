package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var stamp = time.Date(2026, 11, 10, 18, 39, 20, 0, time.UTC)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := strings.TrimRight(string(raw), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func open(t *testing.T, dir string, maxLines int) *Log {
	t.Helper()
	log, err := Open(dir, maxLines, stamp, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return log
}

func TestOpenNamesTheFileBySessionStamp(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 0)
	defer log.Close()
	want := filepath.Join(dir, "session-20261110-183920.txt")
	if log.Path() != want {
		t.Fatalf("path = %q, want %q", log.Path(), want)
	}
}

// Concurrent windows record into one directory, so the window marker must be
// part of the name and must be reported back for change detection.
func TestSuffixDistinguishesWindows(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir, 0, stamp, "-screen1")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	defer first.Close()
	second, err := Open(dir, 0, stamp, "-screen2")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	defer second.Close()
	if filepath.Base(first.Path()) != "session-20261110-183920-screen1.txt" {
		t.Fatalf("first path = %q", first.Path())
	}
	if filepath.Base(second.Path()) != "session-20261110-183920-screen2.txt" {
		t.Fatalf("second path = %q", second.Path())
	}
	if first.Suffix() != "-screen1" || second.Suffix() != "-screen2" {
		t.Fatalf("suffixes = %q, %q", first.Suffix(), second.Suffix())
	}
}

func TestTwoSessionsInOneSecondDoNotShareAFile(t *testing.T) {
	dir := t.TempDir()
	first := open(t, dir, 0)
	defer first.Close()
	second := open(t, dir, 0)
	defer second.Close()
	if first.Path() == second.Path() {
		t.Fatalf("both sessions wrote %q", first.Path())
	}
	if !strings.HasSuffix(second.Path(), "-2.txt") {
		t.Fatalf("second path = %q, want a -2 suffix", second.Path())
	}
}

func TestUnlimitedKeepsEveryLine(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 0)
	for i := range 500 {
		log.Printf("line %d", i)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if got := len(readLines(t, log.Path())); got != 500 {
		t.Fatalf("lines = %d, want 500", got)
	}
}

// A capped session must still be readable while it runs: lines reach the file
// immediately instead of waiting for a batch flush or a clean shutdown.
func TestCappedLogWritesBeforeClose(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 1000)
	log.Printf("first line")
	lines := readLines(t, log.Path())
	if len(lines) != 1 || !strings.HasSuffix(lines[0], "first line") {
		t.Fatalf("file before close = %q", lines)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestCapKeepsTheLastLines(t *testing.T) {
	dir := t.TempDir()
	const cap = 50
	log := open(t, dir, cap)
	for i := range 300 {
		log.Printf("line %d", i)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	lines := readLines(t, log.Path())
	if len(lines) != cap {
		t.Fatalf("lines = %d, want %d", len(lines), cap)
	}
	if !strings.Contains(lines[0], "line 250") {
		t.Fatalf("first retained line = %q, want line 250", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "line 299") {
		t.Fatalf("last retained line = %q, want line 299", lines[len(lines)-1])
	}
}

func TestPacketRecordsDirectionHeaderAndPayload(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 0)
	log.now = func() time.Time { return stamp }
	log.Packet(Out, []byte{7, 0, 1, 0, 2, 0})
	log.Packet(In, []byte{12, 0, 0xff})
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	lines := readLines(t, log.Path())
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[0], "[packet] > len=6 7:0 070001000200") {
		t.Fatalf("outgoing line = %q", lines[0])
	}
	if !strings.Contains(lines[1], "[packet] < len=3 12:0 0c00ff") {
		t.Fatalf("incoming line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[0], "2026-11-10 18:39:20") {
		t.Fatalf("missing timestamp: %q", lines[0])
	}
}

func TestWriteSplitsLoggerOutputIntoLines(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 0)
	if _, err := log.Write([]byte("first\nsecond\n\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	lines := readLines(t, log.Path())
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if !strings.HasSuffix(lines[0], "first") || !strings.HasSuffix(lines[1], "second") {
		t.Fatalf("lines = %q", lines)
	}
}

func TestLimitStepsOfferAnUnlimitedChoice(t *testing.T) {
	if LimitSteps[0] != 0 {
		t.Fatalf("first step = %d, want 0 for unlimited", LimitSteps[0])
	}
	if NormalizeLimit(-5) != 0 || NormalizeLimit(MaxLimit+1) != MaxLimit {
		t.Fatalf("clamp failed: %d %d", NormalizeLimit(-5), NormalizeLimit(MaxLimit+1))
	}
}

func TestWritesAreIgnoredAfterClose(t *testing.T) {
	dir := t.TempDir()
	log := open(t, dir, 0)
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	log.Printf("after close")
	if got := len(readLines(t, log.Path())); got != 0 {
		t.Fatalf("lines = %d, want 0", got)
	}
}

func TestMultilineDiagnosticsRespectLineCap(t *testing.T) {
	l := open(t, t.TempDir(), 2)
	l.Printf("panic\nstack frame 1\nstack frame 2")
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	lines := readLines(t, l.Path())
	if len(lines) != 2 || !strings.HasSuffix(lines[0], "stack frame 1") || !strings.HasSuffix(lines[1], "stack frame 2") {
		t.Fatalf("incorrect retained lines: %q", lines)
	}
}

func TestCloseReportsWriteAndTrimFailures(t *testing.T) {
	l := open(t, t.TempDir(), 2)
	if err := l.file.Close(); err != nil {
		t.Fatal(err)
	}
	l.Printf("write to unavailable file")
	if err := l.Close(); err == nil {
		t.Fatal("logging failure was silently discarded")
	}
}
