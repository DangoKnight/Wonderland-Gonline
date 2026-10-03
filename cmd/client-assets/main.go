// client-assets stages the original client's resources into
// var/client-original-assets for offline native archive inspection and
// creates independently uploadable ZIPs. Paths in the manifests and ZIPs are
// relative to that directory. The normal client never reads it: it uses the
// decompiled data in data/, which cmd/asset-build verifies.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var directories = []string{"cursor", "data", "font", "images", "info", "jma", "menu", "Mp3s", "pic", "sound", "sty", "txt", "upimg", "voice"}

type entry struct {
	Path   string `json:"path"`
	Size   int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type manifest struct {
	Version int     `json:"version"`
	Source  string  `json:"source_variant"`
	Files   []entry `json:"files"`
}

type packageEntry struct {
	File   string `json:"file"`
	Size   int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

func main() {
	source := flag.String("source", "", "installed client resource source (required for staging)")
	dest := flag.String("dest", "var/client-original-assets", "local directory for the original client's files")
	manifestPath := flag.String("manifest", "client/assets-manifest.json", "tracked checksum manifest")
	pack := flag.String("pack", "", "optional output directory for upload ZIPs")
	verify := flag.Bool("verify", false, "verify local files against manifest without accessing installation")
	unpack := flag.String("unpack", "", "extract and verify all ZIPs from this download directory")
	packages := flag.String("packages", "client/asset-packages.json", "trusted upload package checksum catalog")
	flag.Parse()
	if *unpack != "" {
		if *source != "" || *pack != "" || *verify {
			log.Fatal("-unpack cannot be combined with -source, -pack or -verify")
		}
		if err := unpackAssets(*unpack, *dest, *manifestPath, *packages); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(*source, *dest, *manifestPath, *pack, *verify); err != nil {
		log.Fatal(err)
	}
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

func safePath(root, name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") || strings.Contains(name, ":") || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("unsafe resource path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." || part == "." || part == "" {
			return "", fmt.Errorf("unsafe resource path %q", name)
		}
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}

func run(source, dest, manifestPath, pack string, verify bool) error {
	if verify {
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
		for _, e := range m.Files {
			path, err := safePath(dest, e.Path)
			if err != nil {
				return err
			}
			n, sum, err := digest(path)
			if err != nil {
				return err
			}
			if n != e.Size || sum != e.SHA256 {
				return fmt.Errorf("checksum mismatch: %s", e.Path)
			}
		}
		fmt.Printf("Verified %d local asset files\n", len(m.Files))
		return nil
	}
	if source == "" {
		return fmt.Errorf("-source is required to stage resources")
	}
	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	if strings.EqualFold(sourceAbs, destAbs) || strings.HasPrefix(strings.ToLower(destAbs), strings.ToLower(sourceAbs)+string(os.PathSeparator)) {
		return fmt.Errorf("destination must be outside original installation")
	}
	m := manifest{Version: 1, Source: filepath.Base(sourceAbs)}
	for _, dir := range directories {
		root := filepath.Join(sourceAbs, dir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("resource symlink rejected: %s", path)
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("non-regular resource: %s", path)
			}
			rel, err := filepath.Rel(sourceAbs, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			target, err := safePath(destAbs, rel)
			if err != nil {
				return err
			}
			if err = os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			out, err := os.CreateTemp(filepath.Dir(target), ".asset-*")
			if err != nil {
				in.Close()
				return err
			}
			temp := out.Name()
			h := sha256.New()
			n, copyErr := io.Copy(io.MultiWriter(out, h), in)
			in.Close()
			closeErr := out.Close()
			if copyErr != nil {
				os.Remove(temp)
				return copyErr
			}
			if closeErr != nil {
				os.Remove(temp)
				return closeErr
			}
			if n != info.Size() {
				os.Remove(temp)
				return fmt.Errorf("source size changed: %s", rel)
			}
			if err = os.Rename(temp, target); err != nil {
				os.Remove(temp)
				return err
			}
			m.Files = append(m.Files, entry{rel, n, hex.EncodeToString(h.Sum(nil))})
			return nil
		})
		if err != nil {
			return err
		}
		fmt.Println("Staged", dir)
	}
	if len(m.Files) == 0 {
		return fmt.Errorf("source contains no resource files")
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err = os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(manifestPath, data, 0644); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(destAbs, "manifest.json"), data, 0644); err != nil {
		return err
	}
	if pack != "" {
		if err = os.MkdirAll(pack, 0755); err != nil {
			return err
		}
		var packages []packageEntry
		for _, dir := range directories {
			var files []entry
			for _, e := range m.Files {
				if strings.HasPrefix(e.Path, dir+"/") {
					files = append(files, e)
				}
			}
			if len(files) == 0 {
				continue
			}
			if err = writePack(filepath.Join(pack, dir+".zip"), destAbs, data, files); err != nil {
				return err
			}
			fmt.Println("Packaged", dir+".zip")
			n, sum, err := digest(filepath.Join(pack, dir+".zip"))
			if err != nil {
				return err
			}
			packages = append(packages, packageEntry{dir + ".zip", n, sum})
		}
		packageData, err := json.MarshalIndent(struct {
			Version     int            `json:"version"`
			Format      string         `json:"format"`
			ExtractInto string         `json:"extract_into"`
			Packages    []packageEntry `json:"packages"`
		}{1, "ZIP (stored, ZIP64 supported)", dest, packages}, "", "  ")
		if err != nil {
			return err
		}
		packageData = append(packageData, '\n')
		if err = os.WriteFile(filepath.Join(filepath.Dir(manifestPath), "asset-packages.json"), packageData, 0644); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(pack, "asset-packages.json"), packageData, 0644); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(pack, "assets-manifest.json"), data, 0644); err != nil {
			return err
		}
	}
	fmt.Printf("Staged %d files; manifest: %s\n", len(m.Files), manifestPath)
	return nil
}

func writePack(path, root string, data []byte, files []entry) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	z := zip.NewWriter(f)
	writeErr := func() error {
		w, err := z.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Store})
		if err != nil {
			return err
		}
		if _, err = w.Write(data); err != nil {
			return err
		}
		for _, e := range files {
			w, err := z.CreateHeader(&zip.FileHeader{Name: e.Path, Method: zip.Store})
			if err != nil {
				return err
			}
			path, err := safePath(root, e.Path)
			if err != nil {
				return err
			}
			in, err := os.Open(path)
			if err != nil {
				return err
			}
			_, err = io.Copy(w, in)
			in.Close()
			if err != nil {
				return err
			}
		}
		return nil
	}()
	zipErr := z.Close()
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if zipErr != nil {
		return zipErr
	}
	return closeErr
}
