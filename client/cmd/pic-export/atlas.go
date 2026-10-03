package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"wonderland-go/internal/clientassets"
	"wonderland-go/internal/spritepack"
)

// Small pictures share atlas pages; large terrain/background images stay loose.
const smallPictureMaxSide = 512
const pictureAtlasVersion = 2

func atlasPictures(root string, doc *manifest) error {
	groups := map[string][]*picture{}
	for i := range doc.Files {
		for j := range doc.Files[i].Pictures {
			p := &doc.Files[i].Pictures[j]
			if p.Rect != nil {
				return fmt.Errorf("picture is already atlased: %s", p.PNG)
			}
			if p.Width > smallPictureMaxSide || p.Height > smallPictureMaxSide {
				continue
			}
			group := filepath.ToSlash(filepath.Dir(p.PNG))
			if group == "." {
				group = "loose"
			}
			p.LogicalPath = p.PNG
			groups[group] = append(groups[group], p)
		}
	}
	keys := make([]string, 0, len(groups))
	for group := range groups {
		keys = append(keys, group)
	}
	sort.Strings(keys)
	for _, group := range keys {
		pictures := groups[group]
		directory := filepath.Join(root, "atlases", filepath.FromSlash(group))
		if err := os.MkdirAll(directory, 0755); err != nil {
			return err
		}
		builder := spritepack.NewBuilder(group, func(page spritepack.Page) error {
			file, err := os.OpenFile(filepath.Join(directory, spritepack.PageName(page.Index)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				return err
			}
			err = png.Encode(file, page.RGBA)
			closeErr := file.Close()
			if err != nil {
				return err
			}
			return closeErr
		})
		for i, p := range pictures {
			file, err := os.Open(filepath.Join(root, filepath.FromSlash(p.PNG)))
			if err != nil {
				return err
			}
			image, err := png.Decode(file)
			file.Close()
			if err != nil {
				return err
			}
			if image.Bounds().Dx() != p.Width || image.Bounds().Dy() != p.Height {
				return fmt.Errorf("picture dimensions differ: %s", p.PNG)
			}
			if err = builder.AddSprite(i, i, p.Resource, []spritepack.FrameImage{{Name: p.Resource, Image: image}}, nil, nil); err != nil {
				return err
			}
		}
		pack, err := builder.Finish()
		if err != nil {
			return err
		}
		for pageIndex, pageName := range pack.Pages {
			atlas := filepath.ToSlash(filepath.Join("atlases", group, pageName))
			sheet, err := clientassets.ReadPicturePNG(filepath.Join(root, filepath.FromSlash(atlas)), nil)
			if err != nil {
				return err
			}
			for i, p := range pictures {
				frame := pack.Sprites[i].Frames[0]
				if frame.Page != pageIndex {
					continue
				}
				old, err := clientassets.ReadPicturePNG(filepath.Join(root, filepath.FromSlash(p.PNG)), nil)
				if err != nil {
					return err
				}
				rect := clientassets.SpriteRect{X: frame.Rect.X, Y: frame.Rect.Y, Width: frame.Rect.W, Height: frame.Rect.H}
				cropped := sheet.SubImage(image.Rect(rect.X, rect.Y, rect.X+rect.Width, rect.Y+rect.Height))
				if !samePicturePixels(old, cropped) {
					return fmt.Errorf("atlas pixels differ: %s", p.PNG)
				}
				original := p.PNG
				p.PNG = atlas
				p.Rect = &rect
				if err = os.Remove(filepath.Join(root, filepath.FromSlash(original))); err != nil {
					return err
				}
			}
		}

	}
	doc.SchemaVersion = pictureAtlasVersion
	_, err := clientassets.PruneEmptyDirectories(root)
	return err
}

func samePicturePixels(a, b interface {
	Bounds() image.Rectangle
	At(int, int) color.Color
}) bool {
	if a.Bounds().Size() != b.Bounds().Size() {
		return false
	}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			left := color.NRGBAModel.Convert(a.At(a.Bounds().Min.X+x, a.Bounds().Min.Y+y))
			right := color.NRGBAModel.Convert(b.At(b.Bounds().Min.X+x, b.Bounds().Min.Y+y))
			if left != right {
				return false
			}
		}
	}
	return true
}

// optimizePictures converts an existing editable export without using it as an
// authoritative source for regeneration. Fresh native export also calls this.
func optimizePictures(root string) error {
	temporary, err := os.MkdirTemp(filepath.Dir(root), ".picture-optimize-*")
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			os.RemoveAll(temporary)
		}
	}()
	prepared := filepath.Join(temporary, "prepared")
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		target := filepath.Join(prepared, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("expected regular picture asset: %s", path)
		}
		if err = os.Link(path, target); err == nil {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, 0644)
	})
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(prepared, "manifest.json"))
	if err != nil {
		return err
	}
	var doc manifest
	if err = json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	if doc.SchemaVersion >= pictureAtlasVersion {
		return fmt.Errorf("pictures already have atlas metadata")
	}
	if err = atlasPictures(prepared, &doc); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	metadata := filepath.Join(prepared, "manifest.json")
	if err = os.Remove(metadata); err != nil {
		return err
	}
	if err = os.WriteFile(metadata, append(encoded, '\n'), 0644); err != nil {
		return err
	}
	backup := filepath.Join(temporary, "previous")
	if err = os.Rename(root, backup); err != nil {
		return err
	}
	if err = os.Rename(prepared, root); err != nil {
		if restoreErr := os.Rename(backup, root); restoreErr != nil {
			cleanup = false
			return fmt.Errorf("picture publication failed: %v; restore failed: %v; original retained at %s", err, restoreErr, backup)
		}
		return err
	}
	return nil
}
