package debuglog

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
)

func TestRecordersIsolateSessionsAndPreserveGlobalOutput(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	first, second := open(t, t.TempDir(), DefaultLimit), open(t, t.TempDir(), DefaultLimit)
	r1, r2 := &Recorder{}, &Recorder{}
	r1.SetLog(first)
	r2.SetLog(second)
	var writers sync.WaitGroup
	for _, entry := range []struct {
		recorder *Recorder
		message  string
	}{{r1, "first session"}, {r2, "second session"}} {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for range 40 {
				entry.recorder.Printf("%s", entry.message)
			}
		}()
	}
	writers.Wait()
	r1.SetLog(nil)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	r2.Printf("second remains active")
	log.Print("process-wide diagnostic")
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if log.Writer() != &output {
		t.Fatal("recorder replaced the global log output")
	}
	for _, entry := range []struct {
		file       *Log
		own, other string
	}{{first, "first session", "second session"}, {second, "second session", "first session"}} {
		text := strings.Join(readLines(t, entry.file.Path()), "\n")
		if !strings.Contains(text, entry.own) || strings.Contains(text, entry.other) || strings.Contains(text, "process-wide diagnostic") {
			t.Fatal("session diagnostics crossed transcripts")
		}
	}
	if !strings.Contains(output.String(), "first session") || !strings.Contains(output.String(), "second remains active") || !strings.Contains(output.String(), "process-wide diagnostic") {
		t.Fatal("original log output lost diagnostics")
	}
}

func TestRecorderToggleDuringBackgroundWrites(t *testing.T) {
	l := open(t, t.TempDir(), DefaultLimit)
	r := &Recorder{}
	var writers sync.WaitGroup
	writers.Add(1)
	go func() {
		defer writers.Done()
		for range 100 {
			r.Printf("background diagnostic")
		}
	}()
	for range 100 {
		r.SetLog(l)
		r.SetLog(nil)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	writers.Wait()
}
