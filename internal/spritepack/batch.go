package spritepack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// IndexFile lists the packs of an output directory.
const IndexFile = "index.json"

// IndexEntry is one pack in IndexFile.
type IndexEntry struct {
	Archive string `json:"archive"`
	Sprites int    `json:"sprites"`
	Frames  int    `json:"frames"`
	Pages   int    `json:"pages"`
}

// BuildOptions select a batch build: exactly one of Editable (the editable
// export, data/sprites) or JMA (original jma/Jxa archives) is the source.
type BuildOptions struct {
	Editable string
	JMA      string
	Output   string
	Archives []string // lower-case names to build; empty builds all
	Workers  int
	Log      func(format string, args ...any)
}

type buildJob struct {
	archive string
	build   func(PageSink) (*Pack, error)
}

// BuildAll builds every selected archive into Output, in parallel, then
// rewrites IndexFile for the whole output directory.
func BuildAll(o BuildOptions) ([]IndexEntry, error) {
	if (o.Editable == "") == (o.JMA == "") {
		return nil, errors.New("set exactly one of Editable or JMA")
	}
	if o.Workers < 1 {
		o.Workers = max(1, runtime.NumCPU()/2)
	}
	want := map[string]bool{}
	for _, a := range o.Archives {
		if a = strings.TrimSpace(strings.ToLower(a)); a != "" {
			want[a] = true
		}
	}
	jobs, err := collectJobs(o.Editable, o.JMA, want)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		return nil, errors.New("no archives found")
	}
	results := make([]IndexEntry, len(jobs))
	errs := make([]error, len(jobs))
	work := make(chan int)
	var wg sync.WaitGroup
	for range o.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				j := jobs[i]
				p, err := WriteDir(o.Output, j.archive, j.build)
				if err != nil {
					errs[i] = fmt.Errorf("%s: %w", j.archive, err)
					continue
				}
				results[i] = indexEntry(p)
				if o.Log != nil {
					e := results[i]
					o.Log("%s: %d sprites, %d frames, %d pages", e.Archive, e.Sprites, e.Frames, e.Pages)
				}
			}
		}()
	}
	for i := range jobs {
		work <- i
	}
	close(work)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return results, WriteIndex(o.Output)
}

func indexEntry(p *Pack) IndexEntry {
	e := IndexEntry{Archive: p.Archive, Sprites: len(p.Sprites), Pages: len(p.Pages)}
	for _, s := range p.Sprites {
		e.Frames += len(s.Frames)
	}
	return e
}

func collectJobs(editable, jma string, want map[string]bool) ([]buildJob, error) {
	var jobs []buildJob
	if editable != "" {
		entries, err := os.ReadDir(editable)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			dir := filepath.Join(editable, e.Name())
			if _, err := os.Stat(filepath.Join(dir, EditableFile)); !e.IsDir() || err != nil {
				continue
			}
			name := strings.ToLower(e.Name())
			if len(want) > 0 && !want[name] {
				continue
			}
			jobs = append(jobs, buildJob{name, func(sink PageSink) (*Pack, error) { return BuildEditable(dir, name, sink) }})
		}
		return jobs, nil
	}
	entries, err := os.ReadDir(jma)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".jma") {
			continue
		}
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		name := strings.ToLower(stem)
		if len(want) > 0 && !want[name] {
			continue
		}
		jmaPath, jxaPath := filepath.Join(jma, e.Name()), findFold(jma, stem+".jxa")
		jobs = append(jobs, buildJob{name, func(sink PageSink) (*Pack, error) {
			j, err := OpenJMA(jmaPath, jxaPath)
			if err != nil {
				return nil, err
			}
			defer j.Close()
			return BuildJMA(j, name, sink)
		}})
	}
	return jobs, nil
}

// findFold finds a file in dir ignoring case.
func findFold(dir, name string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return filepath.Join(dir, name)
}

// WriteIndex lists every pack in dir, including ones built by earlier runs.
func WriteIndex(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var index []IndexEntry
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if p, err := Read(filepath.Join(dir, e.Name())); err == nil {
			index = append(index, indexEntry(p))
		}
	}
	sort.Slice(index, func(i, j int) bool { return index[i].Archive < index[j].Archive })
	data, err := json.MarshalIndent(map[string]any{"version": Version, "archives": index}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, IndexFile), data, 0o644)
}
