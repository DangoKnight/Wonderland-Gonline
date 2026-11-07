package clientbundle

import (
	"bytes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wonderland-gonline/internal/clientfs"
)

func TestRuntimeCompanionFailurePreservesPublishedCore(t *testing.T) {
	for _, group := range []string{"world", "sprites", "audio"} {
		for _, existing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing=%v", group, existing), func(t *testing.T) {
				o, source := fixture(t)
				put(t, filepath.Join(source, "item_data.json"), []byte(`{"items":[]}`))
				var before []byte
				if existing {
					if _, err := BuildRuntime(o); err != nil {
						t.Fatal(err)
					}
					var err error
					before, err = os.ReadFile(o.Output)
					if err != nil {
						t.Fatal(err)
					}
				}
				blocked := strings.TrimSuffix(o.Output, filepath.Ext(o.Output)) + "-" + group + ".build.zip"
				if existing {
					if err := os.Remove(blocked); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Mkdir(blocked, 0755); err != nil {
					t.Fatal(err)
				}
				put(t, filepath.Join(source, "item_data.json"), []byte(`{"source_bytes":42,"items":[]}`))
				if _, err := BuildRuntime(o); err == nil {
					t.Fatal("expected companion failure")
				}
				after, err := os.ReadFile(o.Output)
				if !existing {
					if !os.IsNotExist(err) {
						t.Fatalf("failed first build published a core: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(before, after) {
					t.Fatal("failed companion build replaced the published core")
				}
				_, closePacks, err := clientfs.Mount(o.Output)
				if err != nil {
					t.Fatalf("previous complete bundle cannot be mounted: %v", err)
				}
				closePacks()
			})
		}
	}
}
func TestRuntimeWorkerCountsProduceIdenticalPacks(t *testing.T) {
	o, source := fixture(t)
	put(t, filepath.Join(source, "item_data.json"), []byte(`{"items":[]}`))
	for _, dir := range []string{"pictures/a", "pictures/b", "icons"} {
		if err := os.MkdirAll(filepath.Join(source, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	picture(t, filepath.Join(source, "pictures/a", "one.png"), 1024, 1024, color.NRGBA{R: 255, A: 255})
	picture(t, filepath.Join(source, "pictures/b", "two.png"), 1024, 1024, color.NRGBA{R: 255, A: 255})
	picture(t, filepath.Join(source, "icons", "one.png"), 32, 32, color.NRGBA{G: 255, A: 255})
	for i, workers := range []int{1, 4} {
		output := filepath.Join(filepath.Dir(source), fmt.Sprint(i), "assets.zip")
		opts := o
		opts.Output = output
		opts.Contract = ""
		opts.Workers = workers
		result, err := BuildRuntime(opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("workers=%d files=%d", workers, result.Files)
	}
	a, err := filepath.Glob(filepath.Join(filepath.Dir(source), "0", "*.zip"))
	if err != nil || len(a) == 0 {
		t.Fatalf("missing packs: %v", err)
	}
	for _, path := range a {
		left, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		right, err := os.ReadFile(filepath.Join(filepath.Dir(source), "1", filepath.Base(path)))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(left, right) {
			t.Fatalf("different %s", filepath.Base(path))
		}
	}
}
