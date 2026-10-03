package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wonderland-go/internal/clientassets"
)

func verifyEditable(root, selected string, compare bool) error {
	directories, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	sprites, frames := 0, 0
	for _, directory := range directories {
		if !directory.IsDir() || selected != "" && !strings.EqualFold(selected, directory.Name()) {
			continue
		}
		path := filepath.Join(root, directory.Name())
		editable, err := clientassets.OpenEditableSprites(filepath.Join(path, "editable.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		var file *clientassets.SpriteJSON
		var archive *clientassets.SpriteArchive
		if compare {
			file, err = clientassets.OpenSpriteJSON(filepath.Join(path, "sprites.json"))
			if err != nil {
				return err
			}
			archive = file.Archive
			if len(archive.Entries) != len(editable.Sprites) {
				file.Close()
				return fmt.Errorf("sprite count mismatch: %s", path)
			}
		}
		err = func() error {
			if file != nil {
				defer file.Close()
			}
			for _, sprite := range editable.Sprites {
				i := sprite.Index
				var original *clientassets.Sprite
				if compare {
					if i < 0 || i >= len(archive.Entries) || archive.Entries[i].Name != sprite.Name {
						return fmt.Errorf("sprite name mismatch: %s", path)
					}
					original, err = archive.Sprite(i)
					if err != nil {
						return err
					}
					if len(original.Frames) != len(sprite.Frames) {
						return fmt.Errorf("frame count mismatch: %s", sprite.Name)
					}
				}
				for j, frame := range sprite.Frames {
					pixels, err := editable.FrameImage(frame)
					if err != nil {
						return fmt.Errorf("%s %s frame %d: %w", directory.Name(), sprite.Name, j, err)
					}
					if compare {
						f := original.Frames[j]
						if f.Width != frame.Rect.Width || f.Height != frame.Rect.Height || f.X != frame.AnchorX || f.Y != frame.AnchorY || f.CanvasWidth != frame.CanvasWidth || f.CanvasHeight != frame.CanvasHeight {
							return fmt.Errorf("frame placement mismatch: %s", sprite.Name)
						}
						for y := 0; y < f.Height; y++ {
							for x := 0; x < f.Width; x++ {
								r, g, b, a := clientassets.NativeSpriteColor(f.Pixels[y*f.Stride+x], original.Palette)
								c := pixels.NRGBAAt(pixels.Bounds().Min.X+x, pixels.Bounds().Min.Y+y)
								if c.R != r || c.G != g || c.B != b || c.A != a {
									return fmt.Errorf("pixel mismatch: %s frame %d pixel %d,%d", sprite.Name, j, x, y)
								}
							}
						}
					}
					frames++
				}
				editable.ClearImageCache()
				sprites++
			}
			return nil
		}()
		if err != nil {
			return err
		}
	}
	if sprites == 0 {
		return fmt.Errorf("no editable sprites selected")
	}
	fmt.Printf("Verified %d editable sprites and %d frames (compare original: %t)\n", sprites, frames, compare)
	return nil
}
