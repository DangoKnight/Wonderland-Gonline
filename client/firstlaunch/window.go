package firstlaunch

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"wonderland-gonline/internal/clientimport"
	"wonderland-gonline/internal/localization"
)

//go:embed messages.json
var messageData []byte

const (
	windowWidth        = 800
	windowHeight       = 540
	setupPathLimit     = 4096
	setupProgressLines = 3
)
const (
	stepLanguage = iota + 1
	stepInstall
	stepReady
)

type importResult struct {
	assets string
	err    error
}
type folderResult struct {
	path string
	err  error
}
type progressLog struct {
	sync.Mutex
	lines []string
}

func (p *progressLog) Write(b []byte) (int, error) {
	p.Lock()
	defer p.Unlock()
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line != "" {
			p.lines = append(p.lines, line)
		}
	}
	if len(p.lines) > setupProgressLines {
		p.lines = p.lines[len(p.lines)-setupProgressLines:]
	}
	return len(b), nil
}
func (p *progressLog) snapshot() []string {
	p.Lock()
	defer p.Unlock()
	return append([]string(nil), p.lines...)
}

type wizard struct {
	r                               Request
	step                            int
	source, sourceLanguage, problem string
	face                            font.Face
	chineseFont                     bool
	pathSelected                    bool
	messages                        map[string]map[string]string
	progress                        progressLog
	job                             <-chan importResult
	folder                          <-chan folderResult
	cancel                          context.CancelFunc
	pickerCancel                    context.CancelFunc
	closing, finished               bool
	caret                           int
}

func setupFont() (font.Face, bool, error) {
	// No extracted font is needed. Use an installed CJK font when available;
	// otherwise the setup copy remains English, including the language names.
	paths := []string{}
	switch runtime.GOOS {
	case "windows":
		paths = []string{filepath.Join(os.Getenv("WINDIR"), "Fonts", "msjh.ttc"), filepath.Join(os.Getenv("WINDIR"), "Fonts", "msyh.ttc")}
	case "darwin":
		paths = []string{"/System/Library/Fonts/PingFang.ttc"}
	default:
		paths = []string{"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc", "/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc"}
	}
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		collection, err := opentype.ParseCollection(b)
		if err != nil {
			continue
		}
		f, err := collection.Font(0)
		if err != nil {
			continue
		}
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 17, DPI: 72, Hinting: font.HintingFull})
		if err == nil {
			return face, true, nil
		}
	}
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		return nil, false, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 17, DPI: 72, Hinting: font.HintingFull})
	return face, false, err
}

func runWindow(r Request) error {
	face, chinese, err := setupFont()
	if err != nil {
		return err
	}
	defer face.Close()
	w := &wizard{r: r, step: stepLanguage, sourceLanguage: r.Language, face: face, chineseFont: chinese}
	w.problem = r.Reason
	if err = json.Unmarshal(messageData, &w.messages); err != nil {
		return err
	}
	ebiten.SetWindowSize(windowWidth, windowHeight)
	ebiten.SetWindowTitle("Wonderland Gonline - Setup")
	ebiten.SetWindowClosingHandled(true)
	ebiten.SetRunnableOnUnfocused(true)
	err = ebiten.RunGame(w)
	if err != nil {
		return err
	}
	if !w.finished {
		return ErrCancelled
	}
	return nil
}

func (w *wizard) tr(key string) string {
	lang := w.r.Language
	if lang != localization.Chinese || !w.chineseFont {
		lang = localization.English
	}
	return w.messages[lang][key]
}
func (w *wizard) Layout(int, int) (int, int) { return windowWidth, windowHeight }
func (w *wizard) click(r image.Rectangle) bool {
	x, y := ebiten.CursorPosition()
	return inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) && image.Pt(x, y).In(r)
}
func (w *wizard) buttonRect(x, y, width int) image.Rectangle { return image.Rect(x, y, x+width, y+38) }
func (w *wizard) finish() {
	w.problem = ""
	if err := saveState(w.r.StatePath, State{Language: w.r.Language, Assets: w.r.Assets}); err != nil {
		w.problem = err.Error()
		return
	}
	w.finished = true
}
func (w *wizard) Update() error {
	if w.finished {
		return ebiten.Termination
	}
	if ebiten.IsWindowBeingClosed() || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || w.click(w.buttonRect(32, 482, 150)) {
		w.closing = true
		if w.cancel != nil {
			w.cancel()
		}
		if w.pickerCancel != nil {
			w.pickerCancel()
		}
	}
	if w.job != nil {
		select {
		case result := <-w.job:
			w.job = nil
			w.cancel = nil
			if result.err != nil {
				w.problem = result.err.Error()
			} else {
				w.r.Assets = result.assets
				w.step = stepReady
			}
		default:
		}
	}
	if w.closing && w.job == nil && w.folder == nil {
		return ebiten.Termination
	}
	if w.folder != nil {
		select {
		case result := <-w.folder:
			w.folder = nil
			w.pickerCancel = nil
			if result.err != nil {
				w.problem = w.tr("picker_error")
			} else if result.path != "" {
				w.source = result.path
				w.caret = len([]rune(w.source))
			}
		default:
		}
		return nil
	}
	if w.job != nil || w.closing {
		return nil
	}
	enter := inpututil.IsKeyJustPressed(ebiten.KeyEnter)
	switch w.step {
	case stepLanguage:
		if w.click(w.buttonRect(100, 205, 270)) {
			w.r.Language = localization.English
		}
		if w.click(w.buttonRect(425, 205, 270)) {
			w.r.Language = localization.Chinese
		}
		if enter || w.click(w.buttonRect(580, 482, 188)) {
			if w.r.NeedAssets {
				w.step = stepInstall
			} else {
				w.finish()
			}
		}
	case stepInstall:
		if w.click(w.buttonRect(620, 190, 148)) {
			ch := make(chan folderResult, 1)
			w.folder = ch
			ctx, cancel := context.WithCancel(context.Background())
			w.pickerCancel = cancel
			go func() { defer cancel(); p, err := pickFolder(ctx); ch <- folderResult{p, err} }()
		}
		if w.click(image.Rect(32, 190, 604, 228)) {
			w.caret = len([]rune(w.source))
			w.pathSelected = false
		}
		if w.click(w.buttonRect(220, 252, 240)) {
			w.sourceLanguage = localization.English
		}
		if w.click(w.buttonRect(480, 252, 288)) {
			w.sourceLanguage = localization.Chinese
		}
		w.editPath()
		if enter || w.click(w.buttonRect(510, 482, 258)) {
			w.startImport()
		}
	case stepReady:
		if enter || w.click(w.buttonRect(510, 482, 258)) {
			w.finish()
		}
	}
	return nil
}

func repeating(k ebiten.Key) bool {
	n := inpututil.KeyPressDuration(k)
	return n == 1 || n > 30 && (n-30)%3 == 0
}
func (w *wizard) editPath() {
	r := []rune(w.source)
	w.caret = min(max(w.caret, 0), len(r))
	insert := func(s string) {
		if w.pathSelected {
			r = nil
			w.caret = 0
			w.pathSelected = false
		}
		v := []rune(s)
		v = v[:min(len(v), max(0, setupPathLimit-len(r)))]
		next := append([]rune(nil), r[:w.caret]...)
		next = append(next, v...)
		r = append(next, r[w.caret:]...)
		w.caret += len(v)
	}
	control := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta)
	if control && inpututil.IsKeyJustPressed(ebiten.KeyV) {
		if s, err := ebiten.ClipboardText(); err == nil {
			insert(strings.Trim(strings.TrimSpace(s), "\""))
		}
	}
	if control && inpututil.IsKeyJustPressed(ebiten.KeyA) {
		w.pathSelected = true
	}
	if w.pathSelected && (repeating(ebiten.KeyBackspace) || repeating(ebiten.KeyDelete)) {
		r = nil
		w.caret = 0
		w.pathSelected = false
	}
	if repeating(ebiten.KeyBackspace) && w.caret > 0 {
		r = append(r[:w.caret-1], r[w.caret:]...)
		w.caret--
	}
	if repeating(ebiten.KeyDelete) && w.caret < len(r) {
		r = append(r[:w.caret], r[w.caret+1:]...)
	}
	if repeating(ebiten.KeyArrowLeft) {
		w.pathSelected = false
		w.caret = max(0, w.caret-1)
	}
	if repeating(ebiten.KeyArrowRight) {
		w.pathSelected = false
		w.caret = min(len(r), w.caret+1)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyHome) {
		w.caret = 0
		w.pathSelected = false
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnd) {
		w.caret = len(r)
		w.pathSelected = false
	}
	if !control {
		for _, c := range ebiten.AppendInputChars(nil) {
			if c >= 32 {
				insert(string(c))
			}
		}
	}
	w.source = string(r)
}

func (w *wizard) startImport() {
	w.problem = ""
	w.progress.Lock()
	w.progress.lines = nil
	w.progress.Unlock()
	source := strings.Trim(strings.TrimSpace(w.source), "\"")
	if source == "" {
		w.problem = w.tr("choose_source")
		return
	}
	output := filepath.Join(filepath.Dir(w.r.StatePath), "assets-"+time.Now().Format("20060102-150405.000000000"))
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	ch := make(chan importResult, 1)
	w.job = ch
	lang := w.sourceLanguage
	go func() {
		defer cancel()
		logPath := filepath.Join(filepath.Dir(w.r.StatePath), "setup-import.log")
		f, err := os.Create(logPath)
		if err != nil {
			ch <- importResult{err: err}
			return
		}
		defer f.Close()
		python := "python"
		if _, err := exec.LookPath(python); err != nil {
			if path, err := exec.LookPath("python3"); err == nil {
				python = path
			}
		}
		err = clientimport.Run(ctx, clientimport.Options{Source: source, Output: output, Language: lang, Python: python, Log: io.MultiWriter(f, &w.progress)})
		bundle := filepath.Join(output, clientimport.BundleName)
		if err == nil {
			err = ValidateAssets(bundle)
		}
		if err != nil {
			fmt.Fprintln(f, err)
			err = fmt.Errorf("%w (%s)", err, logPath)
		}
		ch <- importResult{bundle, err}
	}()
}

var ink = color.RGBA{225, 232, 244, 255}

func (w *wizard) label(screen *ebiten.Image, x, y int, s string) {
	text.Draw(screen, s, w.face, x, y, ink)
}
func (w *wizard) fit(s string, width int) string {
	r := []rune(s)
	for len(r) > 0 && font.MeasureString(w.face, string(r)).Ceil() > width {
		r = r[:len(r)-1]
	}
	if len(r) < len([]rune(s)) && len(r) > 3 {
		return string(r[:len(r)-3]) + "..."
	}
	return string(r)
}
func (w *wizard) drawButton(screen *ebiten.Image, x, y, width int, label string, selected bool) {
	c := color.RGBA{46, 65, 94, 255}
	if selected {
		c = color.RGBA{36, 106, 145, 255}
	}
	vector.DrawFilledRect(screen, float32(x), float32(y), float32(width), 38, c, false)
	w.label(screen, x+12, y+26, w.fit(label, width-24))
}
func (w *wizard) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{19, 27, 42, 255})
	w.label(screen, 32, 48, w.tr("title"))
	w.drawButton(screen, 32, 482, 150, w.tr("cancel"), false)
	switch w.step {
	case stepLanguage:
		w.label(screen, 32, 110, w.tr("language_prompt"))
		w.drawButton(screen, 100, 205, 270, "English", w.r.Language == localization.English)
		label := "Chinese (Traditional)"
		if w.chineseFont {
			label = "繁體中文"
		}
		w.drawButton(screen, 425, 205, 270, label, w.r.Language == localization.Chinese)
		w.label(screen, 32, 330, w.tr("change_later"))
		w.drawButton(screen, 580, 482, 188, w.tr("next"), false)
	case stepInstall:
		w.label(screen, 32, 96, w.tr("missing"))
		w.label(screen, 32, 128, w.tr("installation"))
		w.label(screen, 32, 158, w.tr("originals_safe"))
		fieldColor := color.RGBA{34, 46, 66, 255}
		if w.pathSelected {
			fieldColor = color.RGBA{36, 106, 145, 255}
		}
		vector.DrawFilledRect(screen, 32, 190, 572, 38, fieldColor, false)
		path := w.source
		caretWidth := 0
		if path != "" {
			runes := []rune(path)
			caret := min(w.caret, len(runes))
			start := 0
			for start < caret && font.MeasureString(w.face, string(runes[start:caret])).Ceil() > 550 {
				start++
			}
			path = string(runes[start:])
			caretWidth = font.MeasureString(w.face, string(runes[start:caret])).Ceil()
		}
		if path == "" {
			path = w.tr("path_hint")
		}
		w.label(screen, 42, 216, w.fit(path, 550))
		if w.job == nil && w.source != "" && time.Now().UnixMilli()%1000 < 500 {
			x := float32(42 + caretWidth)
			vector.DrawFilledRect(screen, x, 197, 1, 24, ink, false)
		}
		w.drawButton(screen, 620, 190, 148, w.tr("browse"), false)
		w.label(screen, 32, 278, w.tr("source_language"))
		w.drawButton(screen, 220, 252, 240, w.tr("english_source"), w.sourceLanguage == localization.English)
		w.drawButton(screen, 480, 252, 288, w.tr("chinese_source"), w.sourceLanguage == localization.Chinese)
		w.label(screen, 32, 325, w.tr("dependencies"))
		w.label(screen, 32, 355, w.tr("duration"))
		for i, line := range w.progress.snapshot() {
			w.label(screen, 32, 391+i*22, w.fit(line, 735))
		}
		if w.job == nil {
			w.drawButton(screen, 510, 482, 258, w.tr("extract"), false)
		} else {
			w.label(screen, 510, 510, w.tr("working"))
			if !w.closing {
				x := float32(32 + int(time.Now().UnixMilli()/12)%660)
				vector.DrawFilledRect(screen, x, 465, 76, 3, color.RGBA{60, 178, 224, 255}, false)
			}
		}
	case stepReady:
		w.label(screen, 32, 130, w.tr("ready"))
		w.label(screen, 32, 170, w.tr("no_restart"))
		w.label(screen, 32, 225, w.fit(w.r.Assets, 735))
		w.drawButton(screen, 510, 482, 258, w.tr("start"), false)
	}
	if w.problem != "" {
		w.label(screen, 32, 469, w.fit(strings.Join(strings.Fields(w.problem), " "), 735))
	}
	if w.closing && w.job != nil {
		w.label(screen, 32, 469, w.tr("cancelling"))
	}
}

func pickFolder(ctx context.Context) (string, error) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-Command", `Add-Type -AssemblyName System.Windows.Forms; $picker=New-Object System.Windows.Forms.FolderBrowserDialog; $picker.Description='Select the Wonderland Online Rhodes Island installation'; $picker.ShowNewFolderButton=$false; if($picker.ShowDialog() -eq 'OK') { [Console]::OutputEncoding=New-Object System.Text.UTF8Encoding; [Console]::Write($picker.SelectedPath) }; $picker.Dispose()`)
	case "darwin":
		cmd = exec.CommandContext(ctx, "osascript", "-e", `POSIX path of (choose folder with prompt "Select the Wonderland Online Rhodes Island installation")`)
	default:
		if _, err := exec.LookPath("zenity"); err == nil {
			cmd = exec.CommandContext(ctx, "zenity", "--file-selection", "--directory", "--title=Select Wonderland Online Rhodes Island")
		} else if _, err = exec.LookPath("kdialog"); err == nil {
			cmd = exec.CommandContext(ctx, "kdialog", "--getexistingdirectory", os.Getenv("HOME"))
		} else {
			return "", errors.New("no native folder picker")
		}
	}
	hidePickerConsole(cmd)
	b, err := cmd.Output()
	if err != nil {
		var status *exec.ExitError
		if errors.As(err, &status) && (len(strings.TrimSpace(string(status.Stderr))) == 0 || strings.Contains(string(status.Stderr), "(-128)")) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimPrefix(strings.TrimSpace(string(b)), "\ufeff"), nil
}
