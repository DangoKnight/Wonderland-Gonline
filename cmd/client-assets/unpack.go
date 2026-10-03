package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

// unpackAssets uses the repository manifests as the authority, not ZIP filenames
// or metadata supplied by downloads. Every file is checked before replacement.
func unpackAssets(downloads, dest, manifestPath, packagesPath string) error {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var m manifest
	if err = json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Version != 1 || len(m.Files) == 0 {
		return fmt.Errorf("unsupported or empty asset manifest")
	}
	expected := map[string]entry{}
	for _, e := range m.Files {
		if _, err = safePath(dest, e.Path); err != nil {
			return err
		}
		if _, exists := expected[e.Path]; exists {
			return fmt.Errorf("duplicate manifest path: %s", e.Path)
		}
		if e.Size < 0 {
			return fmt.Errorf("negative resource size: %s", e.Path)
		}
		expected[e.Path] = e
	}
	catalogData, err := os.ReadFile(packagesPath)
	if err != nil {
		return err
	}
	var catalog struct {
		Version  int            `json:"version"`
		Packages []packageEntry `json:"packages"`
	}
	if err = json.Unmarshal(catalogData, &catalog); err != nil {
		return err
	}
	if catalog.Version != 1 || len(catalog.Packages) == 0 {
		return fmt.Errorf("unsupported or empty package catalog")
	}
	packageNames := map[string]bool{}
	// Check every archive before making any changes to the destination.
	for _, p := range catalog.Packages {
		if strings.Contains(p.File, "/") || !strings.HasSuffix(p.File, ".zip") || packageNames[p.File] {
			return fmt.Errorf("invalid or duplicate package: %s", p.File)
		}
		path, err := safePath(downloads, p.File)
		if err != nil {
			return err
		}
		n, sum, err := digest(path)
		if err != nil {
			return err
		}
		if n != p.Size || sum != p.SHA256 {
			return fmt.Errorf("package checksum mismatch: %s", p.File)
		}
		packageNames[p.File] = true
	}
	if err = rejectLinks(dest); err != nil {
		return err
	}
	if err = os.MkdirAll(dest, 0755); err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, p := range catalog.Packages {
		z, err := zip.OpenReader(filepath.Join(downloads, p.File))
		if err != nil {
			return err
		}
		err = func() error {
			for _, f := range z.File {
				if !f.Mode().IsRegular() {
					return fmt.Errorf("non-regular ZIP entry: %s", f.Name)
				}
				if f.Name == "manifest.json" {
					in, err := f.Open()
					if err != nil {
						return err
					}
					embedded, err := io.ReadAll(io.LimitReader(in, int64(len(data))*2+4096))
					in.Close()
					if err != nil {
						return err
					}
					var embeddedManifest manifest
					if json.Unmarshal(embedded, &embeddedManifest) != nil || !reflect.DeepEqual(embeddedManifest, m) {
						return fmt.Errorf("embedded manifest mismatch: %s", p.File)
					}
					continue
				}
				e, ok := expected[f.Name]
				if !ok || seen[f.Name] {
					return fmt.Errorf("unexpected or duplicate ZIP entry: %s", f.Name)
				}
				if f.UncompressedSize64 != uint64(e.Size) {
					return fmt.Errorf("ZIP size mismatch: %s", f.Name)
				}
				path, err := safePath(dest, f.Name)
				if err != nil {
					return err
				}
				if err = rejectLinks(path); err != nil {
					return err
				}
				if n, sum, err := digest(path); err == nil && n == e.Size && sum == e.SHA256 {
					seen[f.Name] = true
					continue
				}
				if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
				if err = extractFile(f, path, e); err != nil {
					return err
				}
				seen[f.Name] = true
			}
			return nil
		}()
		closeErr := z.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		fmt.Println("Installed", p.File)
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("incomplete package set: installed %d of %d resources", len(seen), len(expected))
	}
	if err = rejectLinks(filepath.Join(dest, "manifest.json")); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dest, "manifest.json"), data, 0644); err != nil {
		return err
	}
	return run("", dest, manifestPath, "", true)
}

// Reject existing symlinks in the destination and its ancestors so unpacking
// cannot follow a pre-existing directory link out of the selected asset tree.
func rejectLinks(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for {
		info, err := os.Lstat(path)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination symlink rejected: %s", path)
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(path)
		if parent == path {
			return nil
		}
		path = parent
	}
}

func extractFile(f *zip.File, path string, e entry) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(path), ".asset-*")
	if err != nil {
		return err
	}
	temp := out.Name()
	defer os.Remove(temp)
	h := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(out, h), io.LimitReader(in, e.Size+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != e.Size || hex.EncodeToString(h.Sum(nil)) != e.SHA256 {
		return fmt.Errorf("resource checksum mismatch: %s", e.Path)
	}
	return os.Rename(temp, path)
}
