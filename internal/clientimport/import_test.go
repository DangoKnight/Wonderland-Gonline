package clientimport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wonderland-gonline/internal/clientbundle"
	"wonderland-gonline/internal/clientfs"
)

func sourceFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "Legacy source")
	for _, dir := range []string{"data", "jma", "pic"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "aLogin.exe"), []byte("source must remain unchanged"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func extractedFixture(ctx context.Context, o Options, root, dest string) error {
	for _, name := range []string{"tools/data_export/regenerate.py", "tools/data_export/text_languages.py", "data/server/item_mall.json", "internal/assets/lucky_draw_rules.json", "client"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Join(dest, "media"), 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dest, "media", "example.json"), []byte(`{"source":"native","value":42}`), 0644)
}

func TestImportPublishesMountablePacks(t *testing.T) {
	source := sourceFixture(t)
	out := filepath.Join(t.TempDir(), "imported assets")
	err := run(context.Background(), Options{Source: source, Output: out}, extractedFixture, clientbundle.BuildRuntime)
	if err != nil {
		t.Fatal(err)
	}
	root, closeBundle, err := clientfs.Mount(filepath.Join(out, BundleName))
	if err != nil {
		t.Fatal(err)
	}
	defer closeBundle()
	raw, err := clientfs.ReadFile(filepath.Join(root, "media", "example.json"))
	if err != nil || !strings.Contains(string(raw), `"value":42`) {
		t.Fatal(string(raw), err)
	}
	if _, err := os.Stat(filepath.Join(out, EditableDirectory, "media", "example.json")); err != nil {
		t.Fatal(err)
	}
	if caches, err := filepath.Glob(filepath.Join(out, ".client-runtime-*")); err != nil || len(caches) != 0 {
		t.Fatal("unusable staging compiler cache was published", caches, err)
	}
	if tools, err := filepath.Glob(filepath.Join(out, ".extract-tools-*")); err != nil || len(tools) != 0 {
		t.Fatal("temporary extraction tools were published", tools, err)
	}
	if builds, err := filepath.Glob(filepath.Join(out, "*.build.zip")); err != nil || len(builds) != 0 {
		t.Fatal("intermediate development packs were published", builds, err)
	}
	raw, err = os.ReadFile(filepath.Join(source, "aLogin.exe"))
	if err != nil || string(raw) != "source must remain unchanged" {
		t.Fatal("original installation modified", err)
	}
	if err := run(context.Background(), Options{Source: source, Output: out}, extractedFixture, clientbundle.BuildRuntime); err == nil {
		t.Fatal("existing installation overwritten")
	}
}

func TestImportFailureDoesNotPublish(t *testing.T) {
	for _, phase := range []string{"extract", "compile", "destination-created", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			source := sourceFixture(t)
			out := filepath.Join(t.TempDir(), "result")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			extract := func(ctx context.Context, o Options, root, dest string) error {
				if phase == "extract" {
					return errors.New("invalid source archive")
				}
				if phase == "cancel" {
					cancel()
				}
				return extractedFixture(ctx, o, root, dest)
			}
			build := func(o clientbundle.Options) (clientbundle.Result, error) {
				if phase == "destination-created" {
					return clientbundle.Result{}, os.Mkdir(out, 0755)
				}
				return clientbundle.Result{}, errors.New("invalid image dimensions")
			}
			if err := run(ctx, Options{Source: source, Output: out}, extract, build); err == nil {
				t.Fatal("failed import reported success")
			}
			if _, err := os.Stat(filepath.Join(out, BundleName)); !os.IsNotExist(err) {
				t.Fatal("partial bundle published", err)
			}
			stages, err := filepath.Glob(filepath.Join(filepath.Dir(out), ".client-import-*"))
			if err != nil || len(stages) != 0 {
				t.Fatal("staging files leaked", stages, err)
			}
		})
	}
}

func TestImportRejectsUnsafeDestinationsAndLanguages(t *testing.T) {
	source := sourceFixture(t)
	for _, o := range []Options{
		{Source: source, Output: filepath.Join(source, "new-assets")},
		{Source: source, Output: source},
		{Source: source, Output: filepath.Join(t.TempDir(), "out"), Language: "other"},
		{Source: source, Output: filepath.Join(t.TempDir(), "out"), UILanguage: "both"},
		{Source: source, Output: filepath.Join(t.TempDir(), "out"), Language: "zh-TW", ChineseSource: source},
	} {
		if err := run(context.Background(), o, func(context.Context, Options, string, string) error {
			t.Fatal("invalid request reached extraction")
			return nil
		}, clientbundle.BuildRuntime); err == nil {
			t.Fatal("invalid import accepted", o)
		}
	}
	alias := filepath.Join(t.TempDir(), "source-alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Log("symlink test unavailable:", err)
		return
	}
	if err := run(context.Background(), Options{Source: source, Output: filepath.Join(alias, "assets")}, extractedFixture, clientbundle.BuildRuntime); err == nil {
		t.Fatal("symlink allowed writes into source")
	}
}

func TestImportMissingPrerequisitesDoesNotPublish(t *testing.T) {
	out := filepath.Join(t.TempDir(), "assets")
	err := Run(context.Background(), Options{Source: sourceFixture(t), Output: out, Python: filepath.Join(t.TempDir(), "missing-python")})
	if err == nil || !strings.Contains(err.Error(), "Python 3.11+") {
		t.Fatal("missing extraction prerequisite was not explained", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("failed preflight published an installation", err)
	}
}
