package main

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"wonderland-gonline/internal/clientassets"
)

const nativeActionRecordBytes = 80

// Export one sprite's frames into independently editable sheet rectangles.
// Pages bound both editor image sizes and the exporter's working memory.
func exportSpriteSheets(directory string, spriteIndex int, name string, source *clientassets.Sprite) ([]clientassets.EditableFrame, error) {
	frames := make([]clientassets.EditableFrame, len(source.Frames))
	var sheet *image.NRGBA
	page, x, y, rowHeight, usedWidth, usedHeight := 0, 0, 0, 0, 0, 0
	folder := spriteSheetFolder(spriteIndex, name)
	flush := func() error {
		if sheet == nil {
			return nil
		}
		path := filepath.Join(directory, folder, fmt.Sprintf("page_%03d.png", page))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(path), ".sheet-*")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		encoder := png.Encoder{CompressionLevel: png.BestSpeed}
		err = encoder.Encode(file, sheet.SubImage(image.Rect(0, 0, usedWidth, usedHeight)))
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if err = os.Chmod(file.Name(), 0644); err != nil {
			return err
		}
		return os.Rename(file.Name(), path)
	}
	for i, f := range source.Frames {
		frame := clientassets.EditableFrame{Rect: clientassets.SpriteRect{Width: f.Width, Height: f.Height}, Name: f.Name, CanvasWidth: f.CanvasWidth, CanvasHeight: f.CanvasHeight, AnchorX: f.X, AnchorY: f.Y}
		if f.Width == 0 || f.Height == 0 {
			frames[i] = frame
			continue
		}
		side := max(clientassets.EditableSheetSide, f.Width, f.Height)
		if sheet == nil {
			sheet = image.NewNRGBA(image.Rect(0, 0, side, side))
		}
		if f.Width > sheet.Bounds().Dx() || f.Height > sheet.Bounds().Dy() {
			if err := flush(); err != nil {
				return nil, err
			}
			page++
			x, y, rowHeight, usedWidth, usedHeight = 0, 0, 0, 0, 0
			sheet = image.NewNRGBA(image.Rect(0, 0, side, side))
		}
		if x+f.Width > sheet.Bounds().Dx() {
			x = 0
			y += rowHeight
			rowHeight = 0
		}
		if y+f.Height > sheet.Bounds().Dy() {
			if err := flush(); err != nil {
				return nil, err
			}
			page++
			x, y, rowHeight, usedWidth, usedHeight = 0, 0, 0, 0, 0
			sheet = image.NewNRGBA(image.Rect(0, 0, side, side))
		}
		frame.Sheet = filepath.ToSlash(filepath.Join(folder, fmt.Sprintf("page_%03d.png", page)))
		frame.Rect = clientassets.SpriteRect{X: x, Y: y, Width: f.Width, Height: f.Height}
		for row := 0; row < f.Height; row++ {
			for col := 0; col < f.Width; col++ {
				r, g, b, a := clientassets.NativeSpriteColor(f.Pixels[row*f.Stride+col], source.Palette)
				at := (y+row)*sheet.Stride + (x+col)*4
				sheet.Pix[at], sheet.Pix[at+1], sheet.Pix[at+2], sheet.Pix[at+3] = r, g, b, a
			}
		}
		x += f.Width
		rowHeight = max(rowHeight, f.Height)
		usedWidth = max(usedWidth, x)
		usedHeight = max(usedHeight, y+f.Height)
		frames[i] = frame
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return frames, nil
}

func exportEditable(root string, selected string, overwrite bool) error {
	directories, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	// Refuse to overwrite existing user edits unless explicitly requested.
	if !overwrite {
		for _, directory := range directories {
			if !directory.IsDir() || selected != "" && !strings.EqualFold(selected, directory.Name()) {
				continue
			}
			path := filepath.Join(root, directory.Name(), "editable.json")
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("editable sprites already exist at %s; use -overwrite-edits to regenerate", path)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
	}
	total := 0
	for _, directory := range directories {
		if !directory.IsDir() || selected != "" && !strings.EqualFold(selected, directory.Name()) {
			continue
		}
		path := filepath.Join(root, directory.Name())
		file, err := clientassets.OpenSpriteJSON(filepath.Join(path, "sprites.json"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		archive := file.Archive
		// Reconstruct the small native JXA record array from the lossless JSON.
		var actions struct {
			Records []struct {
				Counts []int `json:"action_frame_counts"`
			}
		}
		if err = readJSON(filepath.Join(path, "actions.json"), &actions); err != nil {
			file.Close()
			return err
		}
		if len(actions.Records) != len(archive.Entries) {
			file.Close()
			return fmt.Errorf("action/sprite count mismatch")
		}
		document := clientassets.EditableSprites{Version: clientassets.EditableSpriteVersion}
		for i, entry := range archive.Entries {
			source, err := archive.Sprite(i)
			if err != nil {
				file.Close()
				return err
			}
			frames, err := exportSpriteSheets(path, i, entry.Name, source)
			if err != nil {
				file.Close()
				return err
			}
			sprite := clientassets.EditableSprite{Index: i, Name: entry.Name, Frames: frames}
			counts := actions.Records[i].Counts
			if len(counts) != nativeActionRecordBytes {
				file.Close()
				return fmt.Errorf("invalid native action record")
			}
			offset := 0
			for action, count := range counts {
				if count < 0 || count > 255 {
					file.Close()
					return fmt.Errorf("invalid native action frame count")
				}
				animation := clientassets.SpriteAnimation{Name: fmt.Sprintf("action_%02d", action), Frames: make([]int, count)}
				for n := range count {
					index := offset + n
					if index >= len(frames) {
						index = -1
					}
					animation.Frames[n] = index
				}
				offset += count
				sprite.Animations = append(sprite.Animations, animation)
			}
			document.Sprites = append(document.Sprites, sprite)
		}
		file.Close()
		if err = jsonWrite(filepath.Join(path, "editable.json"), document); err != nil {
			return err
		}
		total += len(document.Sprites)
		fmt.Printf("%s: %d editable sprites\n", directory.Name(), len(document.Sprites))
	}
	if total == 0 {
		return fmt.Errorf("no decoded sprite archives selected")
	}
	fmt.Printf("Exported %d editable sprites as PNG sheets and JSON\n", total)
	return nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, value)
}

func spriteSheetFolder(index int, name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if stem != "" {
		valid := true
		for _, c := range stem {
			if c < '0' || c > '9' {
				valid = false
				break
			}
		}
		if valid {
			return stem
		}
	}
	return fmt.Sprintf("sprite_%04d", index)
}
