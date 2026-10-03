// Command asset-build verifies the decompiled data used directly by the client.
// -copy creates a standalone distribution without repacking editable sprites.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const manifestFile = "asset_manifest.json"

func main() {
	data := flag.String("data", "data", "decompiled data directory")
	output := flag.String("output", filepath.Join("var", "client-distribution"), "standalone distribution directory")
	copyAssets := flag.Bool("copy", false, "copy all decompiled assets for a standalone distribution")
	flag.Parse()
	if err := verify(*data); err != nil {
		log.Fatal(err)
	}
	target := filepath.Join(*output, "data")
	if !*copyAssets {
		log.Printf("verified client data: %s (loaded directly)", *data)
		return
	}
	if info, err := os.Lstat(target); err == nil && info.Mode()&os.ModeSymlink != 0 {
		log.Fatal("distribution output is a symbolic link; use a new -output directory")
	}
	sourceAbs, _ := filepath.Abs(*data)
	targetAbs, _ := filepath.Abs(target)
	if sourceAbs == targetAbs || strings.HasPrefix(targetAbs, sourceAbs+string(filepath.Separator)) {
		log.Fatal("output must be outside the source data tree")
	}
	copied, skipped, removed, err := syncData(*data, target)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("data: %d copied, %d unchanged, %d removed", copied, skipped, removed)
}

// verify checks every export the manifest lists.
func verify(data string) error {
	raw, err := os.ReadFile(filepath.Join(data, manifestFile))
	if err != nil {
		return err
	}
	var m struct {
		Assets []struct {
			Output       string `json:"output"`
			OutputBytes  int64  `json:"output_bytes"`
			OutputSHA256 string `json:"output_sha256"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("%s: %w", manifestFile, err)
	}
	var errs []error
	for _, a := range m.Assets {
		if a.Output == "" || a.OutputSHA256 == "" {
			continue
		}
		if !safeRel(a.Output) {
			return fmt.Errorf("%s: unsafe output %q", manifestFile, a.Output)
		}
		n, sum, err := digest(filepath.Join(data, filepath.FromSlash(a.Output)))
		if err != nil {
			errs = append(errs, err)
		} else if n != a.OutputBytes || sum != a.OutputSHA256 {
			errs = append(errs, fmt.Errorf("%s differs from %s", a.Output, manifestFile))
		}
	}
	return errors.Join(errs...)
}

func safeRel(p string) bool {
	c := filepath.Clean(filepath.FromSlash(p))
	return p != "" && !filepath.IsAbs(c) && c != ".." && !strings.HasPrefix(c, ".."+string(filepath.Separator))
}

func digest(path string) (int64, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	return n, hex.EncodeToString(h.Sum(nil)), err
}

// syncData mirrors all decompiled data including editable sprites into dst. A file is
// copied when its size or modification time differs, through a temporary
// file; files under dst that data no longer has are removed.
func syncData(data, dst string) (copied, skipped, removed int, err error) {
	want := map[string]bool{}
	err = filepath.WalkDir(data, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(data, path)
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s: not a regular file", path)
		}
		want[rel] = true
		info, err := d.Info()
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if t, err := os.Stat(target); err == nil && t.Size() == info.Size() && t.ModTime().Equal(info.ModTime()) {
			skipped++
			return nil
		}
		if err := copyFile(path, target, info); err != nil {
			return err
		}
		copied++
		return nil
	})
	if err != nil {
		return
	}
	err = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dst, path)
		if !want[rel] {
			removed++
			return os.Remove(path)
		}
		return nil
	})
	return
}

func copyFile(src, dst string, info fs.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = io.Copy(tmp, in); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chtimes(tmp.Name(), info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}
