package app

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"wonderland-gonline/client/wlo/render"
	"wonderland-gonline/internal/localization"
)

func TestDisplaySettingsPersistenceAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs", "display.json")
	defaults := defaultDisplaySettings()
	if got, err := loadDisplaySettings(path); err != nil || got != defaults {
		t.Fatal(got, err)
	}
	saved := DisplaySettings{Mode: displayBorderless, Width: 1920, Height: 1080, FPS: 144, Scaling: render.ScalingBicubic, Language: localization.Chinese}
	for range 2 {
		if err := saved.save(path); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := loadDisplaySettings(path); err != nil || got != saved {
		t.Fatal(got, err)
	}
	for _, raw := range []string{`{`, `{"mode":"unknown"}`, `{"width":-1}`, `{"fps":1}`, `{"width":99999}`, `{"mode":"windowed","width":1280,"height":720,"fps":1000}`} {
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if got, err := loadDisplaySettings(path); err == nil || got != defaults {
			t.Fatal("invalid preference not rejected", got, err)
		}
	}
}

func TestSettingsLanguageSelectionPersistsOnlyOnApply(t *testing.T) {
	d := newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	d.apply = func(DisplaySettings) { t.Fatal("language-only change must not reconfigure the window") }
	d.openMenu()
	before := d.settings
	d.openSelect(displayLanguageRow, image.Pt(-1, -1))
	if len(d.selectLabels()) != 2 {
		t.Fatal("language choices missing")
	}
	d.chooseSelect(1)
	if d.draft.Language != localization.Chinese || d.settings != before {
		t.Fatal("selection applied prematurely")
	}
	d.revert()
	if d.draft != before {
		t.Fatal("cancel retained language draft")
	}
	d.openSelect(displayLanguageRow, image.Pt(-1, -1))
	d.chooseSelect(1)
	d.applyDraft(time.Unix(0, 0))
	got, err := loadDisplaySettings(d.path)
	if err != nil || got.Language != localization.Chinese || d.preview || d.open {
		t.Fatal("language preference not saved", got, err)
	}
	got.Language = "unsupported"
	if err := got.validate(); err == nil {
		t.Fatal("unsupported language accepted")
	}
}

func TestDisplayPreviewRollbackAndKeep(t *testing.T) {
	for _, scenario := range []string{"timeout", "cancel", "keep", "save failure"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "display.json")
			d := newDisplayControls(path)
			var applied []DisplaySettings
			d.apply = func(s DisplaySettings) { applied = append(applied, s) }
			d.openMenu()
			old := d.settings
			draft := old
			draft.Mode = displayFullscreen
			draft.Width, draft.Height, draft.FPS = 1920, 1080, 144
			d.draft = draft
			now := time.Unix(100, 0)
			d.applyDraft(now)
			if !d.preview || d.settings != old || d.current() != draft {
				t.Fatal("preview committed prematurely")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatal("preview persisted")
			}
			d.expire(now.Add(displayConfirmDuration - time.Nanosecond))
			if !d.preview {
				t.Fatal("preview expired early")
			}
			switch scenario {
			case "timeout":
				d.expire(now.Add(displayConfirmDuration))
			case "cancel":
				d.revert()
			case "keep":
				d.keep()
			case "save failure":
				d.path = filepath.Join(path, "child.json")
				if err := os.WriteFile(path, []byte("blocked"), 0600); err != nil {
					t.Fatal(err)
				}
				d.keep()
			}
			if scenario == "keep" {
				if d.preview || d.open || d.settings != draft {
					t.Fatal("keep failed")
				}
				if got, err := loadDisplaySettings(path); err != nil || got != draft {
					t.Fatal(got, err)
				}
				if len(applied) != 1 {
					t.Fatal("keep reapplied window")
				}
			} else {
				if d.preview || d.settings != old || d.current() != old || !reflect.DeepEqual(applied, []DisplaySettings{draft, old}) {
					t.Fatal("rollback failed", applied)
				}
			}
		})
	}
}

func TestDisplayPanelLayout(t *testing.T) {
	c, _, _ := enteredClient(t)
	g := NewGame(c)
	g.display = newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	g.display.openMenu()
	g.display.openSelect(displayScalingRow, image.Pt(-1, -1))
	before := g.Workspace.Compose()
	g.drawDisplay(before)
	if path := os.Getenv("DISPLAY_SETTINGS_SNAPSHOT"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = png.Encode(f, before.RGBA()); err != nil {
			t.Fatal(err)
		}
	}
	// Every selectable value and action stays inside the modal panel.
	for row := 0; row < displayRowCount; row++ {
		r := displayValueRect(row)
		if r.Min.X < displayPanelLeft || r.Max.X > displayPanelLeft+displayPanelWidth {
			t.Fatal("value outside panel")
		}
	}
}

func TestDisplaySelectListsOnlyChangeDraftOnSelection(t *testing.T) {
	d := newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	d.openMenu()
	original := d.draft
	d.openSelect(displayResolutionRow, image.Pt(-1, -1))
	if d.draft != original {
		t.Fatal("opening resolution list changed selection")
	}
	r := d.selectRect()
	if !displayResolutionPickingEnabled {
		if d.selectRow != 0 || d.draft != original {
			t.Fatal("disabled resolution picker opened or changed settings")
		}
	} else {
		if r.Max.Y > ScreenHeight {
			t.Fatal("resolution list exceeds viewport")
		}
		// Choose the 1920x1080 entry by its position in the open list.
		d.selectClick(image.Pt(r.Min.X+10, r.Min.Y+6*displaySelectItemHeight+10))
		if d.selectRow != 0 || d.draft.Width != 1920 || d.draft.Height != 1080 || d.settings != original {
			t.Fatal("resolution selection did not remain a draft")
		}
	}
	d.openSelect(displayFrameRateRow, image.Pt(-1, -1))
	before := d.draft
	if d.selectRect().Max.Y > ScreenHeight {
		t.Fatal("frame-rate list exceeds viewport")
	}
	// Clicking Apply while a select is open only dismisses the list.
	d.selectClick(displayActionRect(false).Min)
	if d.selectRow != 0 || d.preview || d.draft != before {
		t.Fatal("outside click changed or applied settings")
	}
	d.openSelect(displayFrameRateRow, image.Pt(-1, -1))
	r = d.selectRect()
	d.selectClick(image.Pt(r.Min.X+10, r.Min.Y+8*displaySelectItemHeight+10))
	if d.draft.FPS != displayFPSUnlimited || d.selectRow != 0 || d.settings != original {
		t.Fatal("frame-rate choice not selected")
	}
	before = d.draft
	d.openSelect(displayModeRow, image.Pt(-1, -1))
	if d.draft != before {
		t.Fatal("opening mode list changed selection")
	}
	r = d.selectRect()
	if r.Min.Y != displayValueRect(0).Max.Y {
		t.Fatal("mode list anchored to wrong control")
	}
	d.selectClick(image.Pt(r.Min.X+10, r.Min.Y+displaySelectItemHeight+10))
	if d.draft.Mode != displayFullscreen || d.selectRow != 0 || d.settings != original {
		t.Fatal("mode choice did not remain a draft")
	}
	d.openSelect(displayScalingRow, image.Pt(-1, -1))
	r = d.selectRect()
	if r.Max.Y > ScreenHeight {
		t.Fatal("filter list exceeds viewport")
	}
	d.selectClick(image.Pt(r.Min.X+10, r.Min.Y+2*displaySelectItemHeight+10))
	if d.draft.Scaling != render.ScalingBicubic || d.settings != original || d.selectRow != 0 {
		t.Fatal("filter choice did not remain a draft")
	}
}

func TestDisplayRenderResolutionIndependentOfWindowSize(t *testing.T) {
	d := newDisplayControls(filepath.Join(t.TempDir(), "display.json"))
	d.settings.Width, d.settings.Height = 1280, 720
	g := &Game{display: d}
	wantWidth, wantHeight := 1280, 720
	if !displayResolutionPickingEnabled {
		wantWidth, wantHeight = workspaceWidth, ScreenHeight
	}
	for _, outside := range [][2]int{{640, 360}, {1020, 600}, {1920, 1080}, {3840, 2160}} {
		w, h := g.Layout(outside[0], outside[1])
		if w != wantWidth || h != wantHeight {
			t.Fatal("window resize changed render resolution", w, h)
		}
	}
	d.draft = d.settings
	d.draft.Width, d.draft.Height = 2560, 1440
	d.preview = true
	previewWidth, previewHeight := 2560, 1440
	if !displayResolutionPickingEnabled {
		previewWidth, previewHeight = workspaceWidth, ScreenHeight
	}
	if w, h := g.Layout(1920, 1080); w != previewWidth || h != previewHeight {
		t.Fatal("preview framebuffer not resized")
	}
	d.apply = func(DisplaySettings) {}
	d.revert()
	if w, h := g.Layout(1920, 1080); w != wantWidth || h != wantHeight {
		t.Fatal("revert retained preview resolution")
	}
}
