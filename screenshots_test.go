//go:build screenshots

// Generates the documentation screenshots by rendering the real UI with Fyne's
// software renderer (no screen access needed):
//
//	SHOT_DIR=/some/dir go test -tags screenshots -run TestScreenshots .
package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/software"
	"fyne.io/fyne/v2/test"

	"FileRenUtil/internal/filter"
	"FileRenUtil/internal/preset"
	"FileRenUtil/internal/rename"
)

// captureWindow is a test window whose canvas is a software-rendered canvas,
// so dialogs (overlays) are captured too.
type captureWindow struct {
	fyne.Window
	c software.WindowlessCanvas
}

func newCaptureWindow(a fyne.App, size fyne.Size, scale float32) *captureWindow {
	c := software.NewCanvas()
	c.SetPadded(false)
	c.SetScale(scale)
	c.Resize(size)
	return &captureWindow{Window: a.NewWindow("RenForge"), c: c}
}

func (w *captureWindow) Canvas() fyne.Canvas            { return w.c }
func (w *captureWindow) SetContent(o fyne.CanvasObject) { w.c.SetContent(o) }
func (w *captureWindow) Content() fyne.CanvasObject     { return w.c.Content() }
func (w *captureWindow) Resize(s fyne.Size)             { w.c.Resize(s) }

func savePNG(t *testing.T, img image.Image, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// makeFolder creates empty files (with a modification time) and returns the folder.
func makeFolder(t *testing.T, root, name string, files ...string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mod := time.Date(2024, 6, 1, 14, 5, 9, 0, time.Local)
	for i, f := range files {
		p := filepath.Join(dir, f)
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, mod.Add(time.Duration(i)*time.Minute), mod.Add(time.Duration(i)*time.Minute))
	}
	return dir
}

type scene struct {
	name   string
	folder string
	preset preset.Preset
	prep   func(ui *appUI, win *captureWindow) // optional extra state
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestScreenshots(t *testing.T) {
	out := os.Getenv("SHOT_DIR")
	if out == "" {
		t.Skip("set SHOT_DIR to generate screenshots")
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	t.Setenv("HOME", root) // folders are shown as ~/Pictures/... in the folder label

	holiday := makeFolder(t, root, "Pictures/Holiday photos",
		"IMG_4021.JPG", "IMG_4022.JPG", "IMG_4023.JPG", "IMG_4024.JPG", "IMG_4025.JPG", "IMG_4026.JPG",
		"IMG_4027.JPG", "IMG_4028.JPG", "IMG_4029 (1).JPG",
		"IMG_4030 sunset over the Seine river from the Pont des Arts - edited version.JPG",
		"IMG_4031 - Copy.JPG", "notes.txt")
	downloads := makeFolder(t, root, "Downloads",
		"Screenshot 2024-06-01 at 10.21.45.png", "Screenshot 2024-06-01 at 10.22.01.png",
		"Screenshot 2024-06-02 at 09.03.12.png", "Invoice_2024-05.PDF", "invoice_2024-06.pdf",
		"Music - Track 09.mp3", "music - track 10.mp3", "holiday.jpg", "IMG_1.jpg", "setup.dmg",
		"Quarterly report FINAL (v2).docx", "photo.jpeg")
	documents := makeFolder(t, root, "Documents/Contracts",
		"report.pdf", "report (1).pdf", "report (2).pdf", "notes.txt", "notes (1).txt",
		"contract-final.pdf", "invoice.pdf", "budget 2024.xlsx", "summary.docx")

	holidaySteps := preset.Preset{MatchAll: true,
		Filters: []filter.Rule{{Mode: filter.Extension, Value: "jpg"}},
		Steps: []rename.Step{
			{Op: rename.OpReplaceText, A: "IMG_", B: "Paris_"},
			{Op: rename.OpCleanup, A: rename.CleanCopies},
			{Op: rename.OpCase, A: rename.CaseExtLower},
		}}

	scenes := []scene{
		{name: "shot1", folder: holiday, preset: holidaySteps,
			prep: func(ui *appUI, _ *captureWindow) { ui.collapse(1); ui.collapse(2) }},
		{name: "shot2", folder: downloads, preset: preset.Preset{MatchAll: false,
			Filters: []filter.Rule{
				{Mode: filter.StartsWith, Value: "Screenshot"},
				{Mode: filter.Extension, Value: "pdf"},
				{Mode: filter.Regex, Value: "^music"},
			},
			Steps: []rename.Step{
				{Op: rename.OpRegex, A: `\s+`, B: "_"},
				{Op: rename.OpCase, A: rename.CaseLower},
			}},
			prep: func(ui *appUI, _ *captureWindow) { ui.collapse(1) }},
		{name: "shot3", folder: documents, preset: preset.Preset{MatchAll: true,
			Steps: []rename.Step{
				{Op: rename.OpCleanup, A: rename.CleanCopies},
				{Op: rename.OpReplaceText, A: "-final", B: "?"},
			}},
			prep: func(ui *appUI, _ *captureWindow) {
				ui.state.deselected[filepath.Join(documents, "notes.txt")] = true // stays, so "notes (1).txt" is blocked
				recomputePreview(ui.state)
				ui.state.viewMode = viewProblems
				ui.collapse(0)
			}},
		{name: "shot4", folder: holiday, preset: holidaySteps,
			prep: func(ui *appUI, win *captureWindow) {
				ui.collapse(1)
				ui.collapse(2)
				ui.refreshView()
				test.Tap(ui.applyBtn)
				waitFor(t, "confirm dialog", func() bool { return win.c.Overlays().Top() != nil })
			}},
	}

	for _, sc := range scenes {
		t.Run(sc.name, func(t *testing.T) {
			a := test.NewApp()
			a.Preferences().SetBool(seenHelpKey, true)
			a.Preferences().SetInt(themeModeKey, modeLight)
			win := newCaptureWindow(a, fyne.NewSize(1440, 900), 2)
			ui := buildUI(a, win)
			ui.loadFolder(sc.folder)
			waitFor(t, "folder scan", func() bool { return !ui.state.scanning && len(ui.state.allFiles) > 0 })
			ui.loadPreset(sc.preset)
			if sc.prep != nil {
				sc.prep(ui, win)
			}
			win.c.Resize(fyne.NewSize(1440, 900))
			ui.refreshView()
			time.Sleep(300 * time.Millisecond)
			win.c.Resize(fyne.NewSize(1440, 900))
			img := win.c.Capture()
			savePNG(t, img, filepath.Join(out, sc.name+".png"))
			fmt.Println("wrote", filepath.Join(out, sc.name+".png"), img.Bounds())
		})
	}
}
