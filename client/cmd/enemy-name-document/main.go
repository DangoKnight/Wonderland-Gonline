// Command enemy-name-document creates a self-contained visual evidence report.
// Client-derived sprite pixels belong in the generated local report, not Git.
package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/csv"
	"flag"
	"fmt"
	"html/template"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"wonderland-gonline/client/wlo/role"
	"wonderland-gonline/client/wlo/surface"
)

//go:embed report.html
var reportHTML string

//go:embed review.js
var reviewJS string

const (
	nativeActionCount       = 256 // Native action IDs occupy one byte.
	preferredStandingAction = 11
	nativePaletteGroups     = 4 // NPC template color groups 1..4.
)

type spriteView struct {
	URL    template.URL
	Image  image.Image
	Action int
	Error  string
}

type entry struct {
	ID, Name, Candidates, Metadata, Notes, Level, Element, Sprite string
	View                                                          spriteView
}

func render(lib *role.Library, look uint16, palette [nativePaletteGroups]uint32) spriteView {
	n := role.NewNPC(lib, look, palette)
	n.Hold(0, false)
	// Editable archives load native palette indices asynchronously. Wait before
	// producing a permanent thumbnail instead of caching the uncolored fallback.
	n.FrameSize(preferredStandingAction)
	lib.Sprites.WaitNative()
	// Prefer a standing direction, then the first available native action.
	actions := []int{preferredStandingAction}
	for action := range nativeActionCount {
		if action != preferredStandingAction {
			actions = append(actions, action)
		}
	}
	for _, action := range actions {
		bounds := n.Bounds(0, 0, action)
		if bounds.Empty() {
			continue
		}
		canvas := surface.New(bounds.Dx(), bounds.Dy())
		n.Draw(canvas, -bounds.Min.X, -bounds.Min.Y, action)
		img := image.NewNRGBA(image.Rect(0, 0, canvas.W, canvas.H))
		visible := false
		for i, pixel := range canvas.Pix {
			if pixel == 0 {
				continue
			}
			visible = true
			c := surface.Expand(pixel)
			img.SetNRGBA(i%canvas.W, i/canvas.W, color.NRGBA{R: c.R, G: c.G, B: c.B, A: 255})
		}
		if !visible {
			continue
		}
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, img); err != nil {
			return spriteView{Error: err.Error()}
		}
		return spriteView{URL: template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes())), Image: img, Action: action}
	}
	return spriteView{Error: "No visible frame in the installed sprite assets"}
}

func run(input, root, output string) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return fmt.Errorf("empty evidence report")
	}
	columns := map[string]int{}
	for i, name := range records[0] {
		columns[strings.TrimPrefix(name, "\ufeff")] = i
	}
	for _, field := range []string{"id", "display_name", "decision", "candidates", "exact_metadata_names", "notes", "level", "element", "sprite", "palette"} {
		if _, ok := columns[field]; !ok {
			return fmt.Errorf("missing column %s", field)
		}
	}
	lib := role.NewLibrary(root)
	defer lib.Sprites.Close()
	cache := map[string]spriteView{}
	var entries []entry
	withCandidates, missing := 0, 0
	for _, record := range records[1:] {
		get := func(name string) string { return record[columns[name]] }
		if get("decision") != "unresolved" {
			continue
		}
		look, err := strconv.ParseUint(get("sprite"), 10, 16)
		if err != nil {
			return fmt.Errorf("entity %s sprite: %w", get("id"), err)
		}
		parts := strings.Split(get("palette"), ";")
		var palette [nativePaletteGroups]uint32
		if len(parts) != len(palette) {
			return fmt.Errorf("entity %s: invalid palette", get("id"))
		}
		for i, part := range parts {
			value, err := strconv.ParseUint(part, 10, 32)
			if err != nil {
				return err
			}
			palette[i] = uint32(value)
		}
		key := get("sprite") + ":" + get("palette")
		view, known := cache[key]
		if !known {
			view = render(lib, uint16(look), palette)
			cache[key] = view
		}
		if view.Error != "" {
			missing++
		}
		if get("candidates") != "" {
			withCandidates++
		}
		entries = append(entries, entry{ID: get("id"), Name: get("display_name"), Candidates: get("candidates"), Metadata: get("exact_metadata_names"), Notes: get("notes"), Level: get("level"), Element: get("element"), Sprite: get("sprite"), View: view})
	}
	if err := os.MkdirAll(filepath.Dir(output), 0755); err != nil {
		return err
	}
	out, err := os.Create(output)
	if err != nil {
		return err
	}
	// Only trusted embedded source becomes JavaScript. Entity data remains in
	// HTML-escaped attributes, never interpolated into executable code.
	t, err := template.New("report").Funcs(template.FuncMap{
		"reviewScript": func() template.JS { return template.JS(reviewJS) },
	}).Parse(reportHTML)
	if err != nil {
		out.Close()
		return err
	}
	err = t.Execute(out, struct {
		Entries                 []entry
		WithCandidates, Missing int
	}{entries, withCandidates, missing})
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := preview(entries, output+".preview.png"); err != nil {
		return err
	}
	fmt.Printf("Wrote %s: %d unresolved entities, %d with prefix candidates, %d missing sprites, %d unique appearances\n", output, len(entries), withCandidates, missing, len(cache))
	return nil
}

func preview(entries []entry, path string) error {
	wanted := map[string]bool{"11013": true, "17174": true, "17879": true, "20048": true, "30404": true, "40162": true, "31229": true, "40516": true}
	canvas := image.NewRGBA(image.Rect(0, 0, 960, 480))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{R: 225, G: 231, B: 235, A: 255}), image.Point{}, draw.Src)
	i := 0
	for _, row := range entries {
		if !wanted[row.ID] {
			continue
		}
		x, y := i%4*240, i/4*240
		if row.View.Image != nil {
			b := row.View.Image.Bounds()
			draw.Draw(canvas, image.Rect(x+(240-b.Dx())/2, y+25, x+(240+b.Dx())/2, y+25+b.Dy()).Intersect(image.Rect(x, y, x+240, y+210)), row.View.Image, b.Min, draw.Over)
		}
		d := font.Drawer{Dst: canvas, Src: image.NewUniform(color.Black), Face: basicfont.Face7x13, Dot: fixed.P(x+8, y+225)}
		d.DrawString(row.ID + " " + row.Name)
		i++
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, canvas)
}

func main() {
	input := flag.String("review", "../tools/data_export/reference/wlohub/enemy_name_review.csv", "enemy-name evidence CSV")
	root := flag.String("data", "../data", "extracted client data directory")
	output := flag.String("output", "../var/reports/enemy-names.html", "self-contained HTML document")
	flag.Parse()
	if err := run(*input, *root, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
