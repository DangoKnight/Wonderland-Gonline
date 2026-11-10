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

func TestOpenNamesTheFileBySessionStamp(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer log.Close()
	want := filepath.Join(dir, "session-20261110-183920.txt")
	if log.Path() != want {
		t.Fatalf("path = %q, want %q", log.Path(), want)
	}
}

func TestTwoSessionsInOneSecondDoNotShareAFile(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	defer first.Close()
	second, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
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
	log, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
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

func TestCapKeepsTheLastLines(t *testing.T) {
	dir := t.TempDir()
	const cap = 50
	log, err := Open(dir, cap, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
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
	log, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
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
	if !strings.Contains(lines[1], "[packet] < len=3 12:0 ff") && !strings.Contains(lines[1], "12:0") {
		t.Fatalf("incoming line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[0], "2026-11-10 18:39:20") {
		t.Fatalf("missing timestamp: %q", lines[0])
	}
}

func TestWriteSplitsLoggerOutputIntoLines(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
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
	log, err := Open(dir, 0, stamp)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := log.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	log.Printf("after close")
	if got := len(readLines(t, log.Path())); got != 0 {
		t.Fatalf("lines = %d, want 0", got)
	}
}
