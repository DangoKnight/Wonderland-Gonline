package firstlaunch

import (
	"archive/zip"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"wonderland-gonline/internal/localization"
)

func testAssets(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "pictures", "map"), 0755); err != nil {
		t.Fatal(err)
	}
	// Independent editable terrain fixture, including a real required background.
	raw := `{"entries":[{"name":"10017.map","terrain":{"width":800,"height":600,"grid_width":1,"grid_height":1,"cells_hex":"00","layers":[{"Resource":1,"X":0,"Y":0}]}}]}`
	if err := os.WriteFile(filepath.Join(root, "ground_data.json"), []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wem_data.json"), []byte(`{"entries":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(root, "pictures", "map", "1.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(f, image.NewNRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return root
}

func TestFirstLaunchDecisionAndPreferences(t *testing.T) {
	assets := testAssets(t)
	path := filepath.Join(t.TempDir(), "prefs", "setup.json")
	r, needed := requestFor(path, Options{Assets: assets})
	if !needed || r.NeedAssets {
		t.Fatal("first launch must request language even with valid assets", r, needed)
	}
	w := wizard{r: r}
	w.r.Language = localization.Chinese
	w.finish()
	if !w.finished {
		t.Fatal(w.problem)
	}
	r, needed = requestFor(path, Options{Assets: "missing-default"})
	if needed || r.Assets != assets || r.Language != localization.Chinese {
		t.Fatal("completed setup not remembered", r, needed)
	}
	r, needed = requestFor(path, Options{Assets: assets, Language: localization.English, ExplicitAssets: true, Force: true})
	if !needed || r.NeedAssets || r.Language != localization.English {
		t.Fatal("explicit setup or language ignored", r, needed)
	}
	r, needed = requestFor(path, Options{Assets: "missing-explicit", ExplicitAssets: true})
	wantPath, _ := filepath.Abs("missing-explicit")
	if !needed || !r.NeedAssets || r.Assets != wantPath {
		t.Fatal("saved path overrode explicit assets", r, needed)
	}
	if err := os.Remove(filepath.Join(assets, "pictures", "map", "1.png")); err != nil {
		t.Fatal(err)
	}
	// Use a fresh path to avoid the rendering cache retaining the old artwork.
	r, needed = requestFor(path, Options{Assets: t.TempDir(), ExplicitAssets: true})
	if !needed || !r.NeedAssets || r.Reason == "" {
		t.Fatal("missing assets did not reopen setup", r, needed)
	}
}

func TestFirstLaunchIncompleteAssetsAndCompanionPack(t *testing.T) {
	root := testAssets(t)
	if err := os.Remove(filepath.Join(root, "pictures", "map", "1.png")); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAssets(root); err == nil {
		t.Fatal("terrain without artwork accepted")
	}
	pack := filepath.Join(t.TempDir(), "client-assets.zip")
	f, err := os.Create(pack)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	entry, err := z.Create("_packs.json")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte(`{"version":1,"packs":["missing-pictures.zip"]}`))
	z.Close()
	f.Close()
	if err := ValidateAssets(pack); err == nil || !strings.Contains(err.Error(), "missing-pictures.zip") {
		t.Fatal("missing companion not diagnosed", err)
	}
}

func TestFirstLaunchCancelledOrInvalidPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.json")
	before := State{Language: localization.English, Assets: "existing"}
	if err := saveState(path, before); err != nil {
		t.Fatal(err)
	}
	before, _ = readState(path)
	w := wizard{r: Request{StatePath: path, Language: localization.Chinese}, closing: true}
	if w.finished {
		t.Fatal("cancelled wizard finished")
	}
	after, err := readState(path)
	if err != nil || after != before {
		t.Fatal("cancel changed saved preferences", after, err)
	}
	for _, raw := range []string{`{`, `{"version":99,"language":"en"}`, `{"version":1,"language":"unknown"}`} {
		os.WriteFile(path, []byte(raw), 0600)
		if _, err := readState(path); err == nil {
			t.Fatal("invalid state accepted", raw)
		}
	}
	w = wizard{r: Request{StatePath: filepath.Join(path, "not-a-directory"), Language: localization.English}}
	w.finish()
	if w.finished || w.problem == "" {
		t.Fatal("save failure reported success")
	}
}

func TestFirstLaunchMessagesAndProgress(t *testing.T) {
	var catalogs map[string]map[string]string
	if err := json.Unmarshal(messageData, &catalogs); err != nil {
		t.Fatal(err)
	}
	for key, value := range catalogs[localization.English] {
		if value == "" || catalogs[localization.Chinese][key] == "" {
			t.Fatal("missing setup translation", key)
		}
	}
	var log progressLog
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 100 {
				log.Write([]byte("progress\n"))
				log.snapshot()
			}
		})
	}
	workers.Wait()
	if len(log.snapshot()) != setupProgressLines {
		t.Fatal("progress log is not bounded")
	}
}

func TestFirstLaunchInstalledBundle(t *testing.T) {
	path := os.Getenv("WONDERLAND_TEST_CLIENT_BUNDLE")
	if path == "" {
		t.Skip("set WONDERLAND_TEST_CLIENT_BUNDLE to validate installed packs")
	}
	if err := ValidateAssets(path); err != nil {
		t.Fatal(err)
	}
}

func TestFirstLaunchExtractionFailureRetainsPreviousSetup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "setup.json")
	if err := saveState(path, State{Language: localization.English, Assets: "old-assets"}); err != nil {
		t.Fatal(err)
	}
	w := wizard{r: Request{StatePath: path, Language: localization.English}, step: stepInstall, source: filepath.Join(t.TempDir(), "not-a-client"), sourceLanguage: localization.English}
	w.startImport()
	result := <-w.job
	if result.err == nil {
		t.Fatal("invalid source unexpectedly extracted")
	}
	if w.finished || w.step != stepInstall {
		t.Fatal("failure completed setup")
	}
	s, err := readState(path)
	if err != nil || s.Assets != "old-assets" {
		t.Fatal("failed extraction replaced setup", s, err)
	}
	log, err := os.ReadFile(filepath.Join(filepath.Dir(path), "setup-import.log"))
	if err != nil || len(log) == 0 {
		t.Fatal("extraction error was not logged", err)
	}
}
