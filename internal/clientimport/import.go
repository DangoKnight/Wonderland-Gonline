// Package clientimport orchestrates offline extraction from a native client.
// Extraction needs Python/Pillow and FFmpeg, but never a Go toolchain or checkout.
package clientimport

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	importdata "wonderland-gonline/data"
	"wonderland-gonline/internal/assets"
	"wonderland-gonline/internal/clientbundle"
	dataexport "wonderland-gonline/tools/data_export"
)

const BundleName = "client-assets.zip"
const EditableDirectory = "editable"

type Options struct {
	Source, ChineseSource, Output string
	Language, UILanguage          string
	Python, Executable            string
	Workers                       int
	Log                           io.Writer
}

// Run builds a fresh installation beside Output, publishing it only after every
// extraction and compile phase succeeds. Existing destinations are never replaced.
func Run(ctx context.Context, o Options) error {
	return run(ctx, o, extract, clientbundle.BuildRuntime)
}

type extractor func(context.Context, Options, string, string) error
type compiler func(clientbundle.Options) (clientbundle.Result, error)

func run(ctx context.Context, o Options, extract extractor, build compiler) error {
	if o.Log == nil {
		o.Log = io.Discard
	}
	if o.Language == "" {
		o.Language = "en"
	}
	if o.Language != "en" && o.Language != "zh-TW" {
		return fmt.Errorf("source language must be en or zh-TW")
	}
	if o.ChineseSource != "" && o.Language != "en" {
		return fmt.Errorf("a companion Chinese installation requires an English primary source")
	}
	if o.UILanguage == "" {
		o.UILanguage = o.Language
		if o.ChineseSource != "" {
			o.UILanguage = "both"
		}
	}
	if o.UILanguage != "en" && o.UILanguage != "zh-TW" && o.UILanguage != "both" {
		return fmt.Errorf("UI language must be en, zh-TW or both")
	}
	if o.Language == "zh-TW" && o.UILanguage != "zh-TW" || o.Language == "en" && o.UILanguage != "en" && o.ChineseSource == "" {
		return fmt.Errorf("requested UI language is not supplied by the source installation(s)")
	}
	if o.Output == "" {
		return fmt.Errorf("an output directory is required")
	}
	output, err := filepath.Abs(o.Output)
	if err != nil {
		return err
	}
	if _, err = os.Lstat(output); err == nil {
		return fmt.Errorf("output must be a new directory: %s", output)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output: %w", err)
	}
	o.Output = output
	if o.Source, err = validateSource(o.Source); err != nil {
		return err
	}
	if o.ChineseSource != "" {
		if o.ChineseSource, err = validateSource(o.ChineseSource); err != nil {
			return err
		}
	}
	// Resolve existing ancestors before creating anything: junctions/symlinks
	// must not route the destination into either original installation.
	resolved, err := resolveDestination(output)
	if err != nil {
		return err
	}
	for _, source := range []string{o.Source, o.ChineseSource} {
		if source != "" && inside(source, resolved) {
			return fmt.Errorf("output must be outside the original installation: %s", source)
		}
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(output), ".client-import-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage) // Only this invocation's newly created directory.
	toolroot, err := os.MkdirTemp(stage, ".extract-tools-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(toolroot)
	if err = materialize(toolroot); err != nil {
		return err
	}
	fmt.Fprintln(o.Log, "Extracting Legacy assets (PNG images, Ogg audio and editable metadata)...")
	if err = extract(ctx, o, toolroot, filepath.Join(stage, EditableDirectory)); err != nil {
		return err
	}
	if err = os.RemoveAll(toolroot); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	fmt.Fprintln(o.Log, "Compiling runtime assets and compressed packs...")
	r, err := build(clientbundle.Options{Source: filepath.Join(stage, EditableDirectory), Output: filepath.Join(stage, BundleName), Contract: filepath.Join(stage, "asset-contract.json"), Workers: o.Workers, Verbose: true})
	if err != nil {
		return fmt.Errorf("compile extracted assets: %w", err)
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// Compiler intermediates use the staging source's absolute path and cannot
	// be reused after publication. Only the editable sources and finished packs
	// belong in the installation; avoid retaining a second runtime asset tree.
	entries, err := os.ReadDir(stage)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), ".client-runtime-") {
			if err := os.RemoveAll(filepath.Join(stage, entry.Name())); err != nil {
				return err
			}
		} else if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".build.zip") {
			// BuildRuntime retains development intermediates alongside its
			// content-addressed packs. Only the published pack names are needed.
			if err := os.Remove(filepath.Join(stage, entry.Name())); err != nil {
				return err
			}
		}
	}
	// Recheck to protect a destination created while extraction was running.
	if _, err = os.Lstat(output); err == nil {
		return fmt.Errorf("destination appeared during import: %s", output)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect output before publication: %w", err)
	}
	if err = os.Rename(stage, output); err != nil {
		return err
	}
	fmt.Fprintf(o.Log, "Ready: %s (%d files). Editable assets: %s\n", filepath.Join(output, BundleName), r.Files, filepath.Join(output, EditableDirectory))
	return nil
}

func inside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveDestination(path string) (string, error) {
	var tail []string
	for {
		if _, err := os.Lstat(path); err == nil {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			for i := len(tail) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, tail[i])
			}
			return resolved, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		tail = append(tail, filepath.Base(path))
		path = filepath.Dir(path)
	}
}

func validateSource(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("-client must identify the Legacy installation")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	for _, name := range []string{"aLogin.exe", "data", "jma", "pic"} {
		info, err := os.Stat(filepath.Join(absolute, name))
		if err != nil {
			return "", fmt.Errorf("Legacy source %s: %w", name, err)
		}
		if info.IsDir() != (name != "aLogin.exe") {
			return "", fmt.Errorf("invalid Legacy source %s", name)
		}
	}
	return absolute, nil
}

func copyResources(source fs.FS, root string) error {
	return fs.WalkDir(source, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out := filepath.Join(root, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(out, 0755)
		}
		raw, err := fs.ReadFile(source, path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, raw, 0644)
	})
}

func materialize(root string) error {
	for _, resource := range []struct {
		source fs.FS
		path   string
	}{
		{dataexport.Resources, "tools/data_export"},
		{importdata.Resources, "data"},
		{assets.ImportResources, "internal/assets"},
	} {
		if err := copyResources(resource.source, filepath.Join(root, resource.path)); err != nil {
			return err
		}
	}
	return os.MkdirAll(filepath.Join(root, "client"), 0755)
}

func extract(ctx context.Context, o Options, root, destination string) error {
	python := o.Python
	if python == "" {
		python = "python"
	}
	// Resolve relative virtual-environment executables before changing the
	// subprocess working directory to the embedded extraction tools.
	python, err := exec.LookPath(python)
	if err != nil {
		return fmt.Errorf("extraction needs Python 3.11+ with Pillow: %w", err)
	}
	python, err = filepath.Abs(python)
	if err != nil {
		return err
	}
	preflight := exec.CommandContext(ctx, python, "-X", "utf8", "-c", "import sys, shutil; assert sys.version_info >= (3,11), 'Python 3.11+ required'; import PIL; assert shutil.which('ffmpeg'), 'FFmpeg missing from PATH'; assert shutil.which('ffprobe'), 'FFprobe missing from PATH'")
	if raw, err := preflight.CombinedOutput(); err != nil {
		return fmt.Errorf("extraction needs Python 3.11+, Pillow and FFmpeg/FFprobe: %w\n%s", err, raw)
	}
	executable := o.Executable
	if executable == "" {
		if executable, err = os.Executable(); err != nil {
			return err
		}
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return err
	}
	args := []string{"-X", "utf8", "-u", filepath.Join(root, "tools/data_export/regenerate.py"), "--client", o.Source, "--output", destination, "--client-language", o.Language, "--ui-language", o.UILanguage}
	if o.ChineseSource != "" {
		args = append(args, "--chinese-client", o.ChineseSource)
	}
	cmd := exec.CommandContext(ctx, python, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = root, o.Log, o.Log
	// Keep native/Python scratch files inside our fresh staging directory too,
	// regardless of the user's TEMP setting or the source installation's drive.
	cmd.Env = append(os.Environ(), "WLO_ASSET_TOOL="+executable, "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8", "TMP="+root, "TEMP="+root, "TMPDIR="+root)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Legacy extraction: %w", err)
	}
	return nil
}
