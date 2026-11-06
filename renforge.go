package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"FileRenUtil/internal/filter"
	"FileRenUtil/internal/meta"
	"FileRenUtil/internal/preset"
	"FileRenUtil/internal/rename"
	"FileRenUtil/internal/undo"
)

/* -------------------- App State -------------------- */

type AppState struct {
	folderPath string
	recursive  bool

	allFiles      []string
	filteredFiles []string

	filters       []filter.Rule
	nextFilterID  int
	matchAll      bool
	caseSensitive bool

	steps      []rename.Step
	nextStepID int

	// new name of every *selected* file (numbering depends on the selection)
	previewNames map[string]string
	// key: rename.DupKey(dir, previewName) -> count among selected files
	previewCounts map[string]int
	// paths the user has explicitly excluded from the apply operation
	deselected map[string]bool

	// order of the file list (and therefore of numbering)
	sortMode string

	// width of the preview list, used to estimate how many lines names wrap to
	listWidth float32

	// which rows the preview shows (All / Changed / Problems) and the counts
	viewMode    string
	problemKind rowKind // narrows the Problems view to one kind (rowNone = all problems)
	visible     []string
	nChanged    int
	nProblems   int
	kindCounts  [rowTaken + 1]int
	pathIndex   map[string]string // PathKey -> path of every scanned file

	// share of the preview width given to the "Original" column (draggable)
	colRatio float32

	// related files (sidecars) renamed together with their primary file
	sidecars    bool
	sidecarExts string
	sidecarSet  map[string]bool
	sidecarIx   *rename.SidecarIndex

	// background folder scan and metadata loading
	scanID     int
	scanning   bool
	cancelScan context.CancelFunc
	metaID     int
	cancelMeta context.CancelFunc
}

// Sort modes of the file list.
const (
	sortNameAsc  = "Name A–Z"
	sortNameDesc = "Name Z–A"
	sortDateAsc  = "Oldest first"
	sortDateDesc = "Newest first"
	sortSizeAsc  = "Smallest first"
	sortSizeDesc = "Largest first"
)

var sortModes = []string{sortNameAsc, sortNameDesc, sortDateAsc, sortDateDesc, sortSizeAsc, sortSizeDesc}

// sortNeed is the metadata a sort mode needs.
func sortNeed(mode string) meta.Level {
	switch mode {
	case sortDateAsc, sortDateDesc:
		return meta.EXIF // capture date, falling back to modification time
	case sortSizeAsc, sortSizeDesc:
		return meta.Stat
	}
	return meta.None
}

func maxLevel(a, b meta.Level) meta.Level {
	if a > b {
		return a
	}
	return b
}

// neededMeta is the metadata level the current steps and sort order require.
func (s *AppState) neededMeta() meta.Level {
	return maxLevel(rename.MetaNeeds(s.steps), sortNeed(s.sortMode))
}

func (s *AppState) selected() []string {
	sel := make([]string, 0, len(s.filteredFiles))
	for _, p := range s.filteredFiles {
		if !s.deselected[p] {
			sel = append(sel, p)
		}
	}
	return sel
}

func (s *AppState) hasActiveSteps() bool {
	for _, st := range s.steps {
		if !st.Disabled {
			return true
		}
	}
	return false
}

const (
	recentFoldersKey  = "recent_folders"
	maxRecentFolders  = 5
	lastUndoLogKey    = "last_undo_log"
	seenHelpKey       = "seen_help"
	textScaleKey      = "text_scale_idx"
	sidecarsKey       = "sidecars"
	sidecarExtsKey    = "sidecar_exts"
	themeModeKey      = "theme_mode"
	colRatioKey       = "col_ratio"
	winWKey           = "win_w"
	winHKey           = "win_h"
	splitKey          = "split_offset"
	recentPlaceholder = "Recent…"
)

func main() {
	a := app.NewWithID("com.blackarck.renforge")
	w := a.NewWindow("RenForge — File Rename Utility")
	buildUI(a, w)
	w.ShowAndRun()
}

// appUI exposes a few handles to the running window (used by the screenshot
// generator in screenshots_test.go).
type appUI struct {
	state       *AppState
	loadFolder  func(path string)
	loadPreset  func(preset.Preset)
	refreshView func()
	applyBtn    *widget.Button
	collapse    func(stepIndex int) // collapse one rename step to a single line
}

// buildUI creates all widgets and puts them into w.
func buildUI(a fyne.App, w fyne.Window) *appUI {
	a.SetIcon(appIcon)
	textScaleIdx := a.Preferences().IntWithFallback(textScaleKey, defaultTextScaleIdx)
	if textScaleIdx < 0 || textScaleIdx >= len(textScales) {
		textScaleIdx = defaultTextScaleIdx
	}
	themeMode := a.Preferences().IntWithFallback(themeModeKey, modeAuto)
	if themeMode < 0 || themeMode >= len(modeNames) {
		themeMode = modeAuto
	}
	a.Settings().SetTheme(newScaledTheme(textScales[textScaleIdx], themeMode))
	if ww, hh := a.Preferences().FloatWithFallback(winWKey, 0), a.Preferences().FloatWithFallback(winHKey, 0); ww >= 700 && hh >= 450 && ww < 6000 && hh < 4000 {
		w.Resize(fyne.NewSize(float32(ww), float32(hh)))
	} else {
		w.Resize(fyne.NewSize(1100, 700))
	}
	colRatio := float32(a.Preferences().FloatWithFallback(colRatioKey, 0.5))
	if colRatio < 0.2 || colRatio > 0.8 {
		colRatio = 0.5
	}

	state := &AppState{
		matchAll:      true,
		sortMode:      sortNameAsc,
		viewMode:      viewAll,
		colRatio:      colRatio,
		sidecars:      a.Preferences().Bool(sidecarsKey),
		sidecarExts:   a.Preferences().StringWithFallback(sidecarExtsKey, rename.DefaultSidecarExts),
		sidecarSet:    rename.ParseExts(a.Preferences().StringWithFallback(sidecarExtsKey, rename.DefaultSidecarExts)),
		previewNames:  map[string]string{},
		previewCounts: map[string]int{},
		deselected:    map[string]bool{},
	}

	/* -------------------- Background helper -------------------- */

	// runBusy shows a modal "working" dialog, runs work off the UI thread,
	// then calls done back on the UI thread.
	runBusy := func(title string, work func(), done func()) {
		d := dialog.NewCustomWithoutButtons(title, widget.NewProgressBarInfinite(), w)
		d.Resize(fyne.NewSize(320, 100))
		d.Show()
		go func() {
			work()
			fyne.Do(func() {
				d.Hide()
				done()
			})
		}()
	}

	/* -------------------- Recent Folders -------------------- */

	getRecentFolders := func() []string {
		raw := a.Preferences().String(recentFoldersKey)
		if raw == "" {
			return nil
		}
		return strings.Split(raw, "|")
	}

	// forward declaration so saveRecentFolder can reference it after creation
	var recentSelect *widget.Select

	saveRecentFolder := func(path string) {
		next := []string{path}
		for _, r := range getRecentFolders() {
			if r != path {
				next = append(next, r)
			}
		}
		if len(next) > maxRecentFolders {
			next = next[:maxRecentFolders]
		}
		a.Preferences().SetString(recentFoldersKey, strings.Join(next, "|"))
		if recentSelect != nil {
			recentSelect.SetOptions(append([]string{recentPlaceholder}, next...))
			recentSelect.SetSelected(recentPlaceholder)
		}
	}

	/* -------------------- Right: Preview (virtualized list) -------------------- */

	resultsHeader := widget.NewLabel("No folder selected.")
	resultsHeader.TextStyle = fyne.TextStyle{Bold: true}

	scanBar := widget.NewProgressBarInfinite()
	scanLabel := widget.NewLabel("")
	scanRow := container.NewBorder(nil, nil, nil, scanLabel, scanBar)
	scanRow.Hide()

	metaLabel := widget.NewLabel("Reading file details…")
	metaRow := container.NewBorder(nil, nil, nil, metaLabel, widget.NewProgressBarInfinite())
	metaRow.Hide()

	emptyText := widget.NewRichText()
	emptyText.Wrapping = fyne.TextWrapWord
	emptyView := container.NewVScroll(emptyText)

	var list *widget.List
	var refreshView func()
	var updateSamples func()
	strip := container.NewHBox() // summary strip: counts by outcome
	viewBtns := map[string]*widget.Button{}
	for _, mode := range []string{viewAll, viewChanged, viewProblems} {
		mode := mode
		viewBtns[mode] = widget.NewButton(mode, func() {
			state.viewMode = mode
			state.problemKind = rowNone
			refreshView()
		})
	}

	toggleFile := func(full string, checked bool) {
		if checked {
			delete(state.deselected, full)
		} else {
			state.deselected[full] = true
		}
		recomputePreview(state)
		refreshView()
	}

	// rowView computes what a row shows. stat=false skips the (disk-touching)
	// "target exists" check, which is only done for rows actually on screen.
	rowView := func(full string, stat bool) rowInfo {
		orig := filepath.Base(full)
		if state.deselected[full] {
			return rowInfo{orig: orig, parts: []diffPart{{text: orig}}, note: "   (not selected)", kind: widget.LowImportance}
		}
		name := state.previewNames[full]
		changed := name != orig
		info := rowInfo{orig: orig, parts: []diffPart{{text: name}}, kind: widget.MediumImportance, selected: true}
		if changed && stat { // the (costlier) character diff is only needed for rows on screen
			info.parts = diffRunes(orig, name)
		}
		switch {
		case changed && rename.InvalidReason(name) != "":
			info.note = "  ⚠ " + rename.InvalidReason(name)
			info.kind = widget.DangerImportance
		case changed && state.previewCounts[rename.DupKey(filepath.Dir(full), name)] > 1:
			info.note = "  ⚠ conflict: another file gets the same name"
			info.kind = widget.WarningImportance
		case changed && stat && rename.TargetBlocked(full, filepath.Join(filepath.Dir(full), name)):
			info.note = "  ⚠ target exists"
			info.kind = widget.WarningImportance
		case !changed && state.hasActiveSteps():
			info.note = "  (unchanged)"
			info.kind = widget.LowImportance
		}
		if changed && state.sidecars && info.kind == widget.MediumImportance {
			if n := relatedCount(state, full); n > 0 {
				info.note = fmt.Sprintf("  (+%d related)", n)
			}
		}
		return info
	}

	list = widget.NewList(
		func() int { return len(state.visible) },
		func() fyne.CanvasObject { return newFileRow(&state.colRatio) },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(state.visible) {
				return
			}
			full := state.visible[id]
			o.(*fileRow).set(rowView(full, true), func(checked bool) { toggleFile(full, checked) })
		},
	)

	// updateRowHeights gives rows with long names enough height to wrap. The
	// number of lines is estimated from the text length and the average glyph
	// width (cheap, so it also works for tens of thousands of files).
	rowBase := newFileRow(&state.colRatio).MinSize().Height
	updateRowHeights := func() {
		if state.listWidth <= 0 {
			return
		}
		th := fyne.CurrentApp().Settings().Theme()
		textSize := th.Size(theme.SizeNameText)
		sample := "The quick brown fox jumps over 0123456789 _-.() lazy dogs"
		sz, _ := fyne.CurrentApp().Driver().RenderedTextSize(sample, textSize, fyne.TextStyle{}, nil)
		avg := sz.Width / float32(utf8.RuneCountInString(sample))
		lineH := sz.Height

		inner := th.Size(theme.SizeNameInnerPadding)
		total := state.listWidth - 36 - 14 - th.Size(theme.SizeNamePadding)
		colO := total*state.colRatio - 2*inner
		colP := total*(1-state.colRatio) - 4*inner // the preview text sits in an extra padded box
		if colO < 50 {
			colO = 50
		}
		if colP < 50 {
			colP = 50
		}
		lines := func(chars int, colW float32) int {
			n := int(math.Ceil(float64(float32(chars) * avg * 1.15 / colW)))
			if n < 1 {
				n = 1
			}
			return n
		}
		for i, p := range state.visible {
			info := rowView(p, false)
			n := lines(utf8.RuneCountInString(info.orig), colO)
			if m := lines(info.previewLen(), colP); m > n {
				n = m
			}
			list.SetItemHeight(i, rowBase+float32(n-1)*lineH)
		}
	}
	list.OnSelected = func(widget.ListItemID) { list.UnselectAll() }

	// updateStrip rebuilds the summary line: "12 will rename · 3 invalid · 40 unchanged".
	// Clicking a problem count shows just those files.
	updateStrip := func() {
		strip.Objects = nil
		defer strip.Refresh()
		if state.folderPath == "" || state.scanning || len(state.filteredFiles) == 0 || !state.hasActiveSteps() {
			return
		}
		c := state.kindCounts
		strip.Add(widget.NewLabelWithStyle(fmt.Sprintf("%d will rename", c[rowOK]), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
		problem := func(kind rowKind, label string, imp widget.Importance) {
			if c[kind] == 0 {
				return
			}
			b := widget.NewButton(fmt.Sprintf("%d %s", c[kind], label), func() {
				state.viewMode, state.problemKind = viewProblems, kind
				refreshView()
			})
			b.Importance = imp
			strip.Add(b)
		}
		problem(rowInvalid, "invalid", widget.DangerImportance)
		problem(rowDup, "duplicate names", widget.WarningImportance)
		problem(rowTaken, "name taken", widget.WarningImportance)
		if c[rowUnchanged] > 0 {
			lbl := widget.NewLabel(fmt.Sprintf("%d unchanged", c[rowUnchanged]))
			lbl.Importance = widget.LowImportance
			strip.Add(lbl)
		}
	}

	refreshView = func() {
		rebuildVisible(state)
		total, matches := len(state.allFiles), len(state.filteredFiles)

		switch {
		case state.folderPath == "":
			resultsHeader.SetText("No folder selected. Click “Select Folder…” to begin.")
			emptyText.ParseMarkdown(welcomeMarkdown)
		case state.scanning:
			resultsHeader.SetText("Scanning folder…")
			emptyText.ParseMarkdown("Scanning the folder, please wait…")
		case matches == 0:
			resultsHeader.SetText(fmt.Sprintf("No matches (0 of %d files).", total))
			if total > 0 {
				emptyText.ParseMarkdown("No files match your filters. Change or clear the filters on the left.")
			} else {
				emptyText.ParseMarkdown("This folder has no files. Tick **Include subfolders** to look inside sub-folders.")
			}
		case len(state.visible) == 0:
			resultsHeader.SetText(fmt.Sprintf("%d matches · %d selected · %d total files.", matches, len(state.previewNames), total))
			if state.viewMode == viewProblems {
				emptyText.ParseMarkdown("**No problems found.** Every selected file can be renamed safely.")
			} else {
				emptyText.ParseMarkdown("**Nothing would change** with the current steps and selection.")
			}
		default:
			resultsHeader.SetText(fmt.Sprintf("%d matches · %d selected · %d total files.",
				matches, len(state.previewNames), total))
		}

		// view buttons with live counts
		counts := map[string]int{viewAll: matches, viewChanged: state.nChanged, viewProblems: state.nProblems}
		for mode, btn := range viewBtns {
			btn.SetText(fmt.Sprintf("%s (%d)", mode, counts[mode]))
			switch {
			case mode == state.viewMode:
				btn.Importance = widget.HighImportance
			case mode == viewProblems && counts[mode] > 0:
				btn.Importance = widget.WarningImportance
			default:
				btn.Importance = widget.MediumImportance
			}
			btn.Refresh()
		}
		updateStrip()

		if len(state.visible) == 0 || state.folderPath == "" {
			list.Hide()
			emptyView.Show()
		} else {
			emptyView.Hide()
			list.Show()
		}
		updateRowHeights()
		list.Refresh()
		if updateSamples != nil {
			updateSamples()
		}
	}

	/* -------------------- Select All / Deselect All -------------------- */

	// Select All / Deselect All act on the rows currently listed (see "Show").
	selectAllBtn := widget.NewButton("Select All", func() {
		for _, p := range state.visible {
			delete(state.deselected, p)
		}
		recomputePreview(state)
		refreshView()
	})

	deselectAllBtn := widget.NewButton("Deselect All", func() {
		for _, p := range state.visible {
			state.deselected[p] = true
		}
		recomputePreview(state)
		refreshView()
	})

	var headerCols *fyne.Container
	headerCols = container.New(&twoCol{ratio: &state.colRatio},
		widget.NewLabelWithStyle("Original (full file name)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabelWithStyle("Preview (changes highlighted)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		newDragHandle(func(dx float32) {
			total := state.listWidth - 36 - 14 - theme.Padding()
			if total <= 0 {
				return
			}
			r := state.colRatio + dx/total
			if r < 0.2 {
				r = 0.2
			} else if r > 0.8 {
				r = 0.8
			}
			state.colRatio = r
			headerCols.Refresh()
			updateRowHeights()
			list.Refresh()
		}, func() { a.Preferences().SetFloat(colRatioKey, float64(state.colRatio)) }),
	)
	columnHeader := container.NewBorder(nil, nil,
		widget.NewLabelWithStyle("✓", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}), nil,
		headerCols)

	var ensureMeta func()
	sortSelect := widget.NewSelect(sortModes, func(mode string) {
		state.sortMode = mode
		applyAll(state)
		refreshView()
		ensureMeta()
	})
	sortSelect.Selected = state.sortMode

	rightTop := container.NewVBox(
		resultsHeader,
		container.NewBorder(nil, nil,
			container.NewHBox(selectAllBtn, deselectAllBtn),
			container.NewHBox(widget.NewLabel("Order:"), sortSelect),
		),
		container.NewHBox(widget.NewLabel("Show:"), viewBtns[viewAll], viewBtns[viewChanged], viewBtns[viewProblems]),
		strip,
		scanRow,
		metaRow,
		widget.NewSeparator(),
		columnHeader,
	)

	/* -------------------- Background metadata loading -------------------- */

	// ensureMeta loads file details (size, dates, EXIF) that the current steps
	// or sort order need, off the UI thread, then refreshes the preview.
	ensureMeta = func() {
		need := state.neededMeta()
		if state.cancelMeta != nil {
			state.cancelMeta()
			state.cancelMeta = nil
		}
		state.metaID++
		id := state.metaID
		if state.scanning || need == meta.None || meta.Missing(state.filteredFiles, need) == 0 {
			metaRow.Hide()
			return
		}
		files := append([]string(nil), state.filteredFiles...)
		ctx, cancel := context.WithCancel(context.Background())
		state.cancelMeta = cancel
		metaLabel.SetText("Reading file details…")
		metaRow.Show()
		go func() {
			err := meta.Ensure(ctx, files, need, func(done, total int) {
				fyne.Do(func() {
					if state.metaID == id {
						metaLabel.SetText(fmt.Sprintf("Reading file details… %d / %d", done, total))
					}
				})
			})
			fyne.Do(func() {
				if state.metaID != id || err != nil {
					return
				}
				metaRow.Hide()
				applyAll(state)
				refreshView()
			})
		}()
	}

	/* -------------------- Background folder scan -------------------- */

	// reloadFiles rescans the folder off the UI thread. A newer scan cancels an
	// older one. done (optional) receives whether the scan succeeded.
	reloadFiles := func(keepSelection bool, done func(ok bool)) {
		if state.cancelScan != nil {
			state.cancelScan()
		}
		ctx, cancel := context.WithCancel(context.Background())
		state.cancelScan = cancel
		state.scanID++
		id := state.scanID
		folder, recursive := state.folderPath, state.recursive

		state.scanning = true
		if !keepSelection {
			meta.Reset()
			state.allFiles, state.deselected = nil, map[string]bool{}
			applyAll(state)
		}
		scanLabel.SetText("")
		scanRow.Show()
		refreshView()

		go func() {
			files, err := rename.ListFiles(ctx, folder, recursive, func(n int) {
				fyne.Do(func() {
					if state.scanID == id {
						scanLabel.SetText(fmt.Sprintf("%d files…", n))
					}
				})
			})
			fyne.Do(func() {
				if state.scanID != id { // superseded by a newer scan
					return
				}
				state.scanning = false
				scanRow.Hide()
				if err != nil {
					dialog.ShowError(err, w)
					state.allFiles = nil
				} else {
					state.allFiles = files
				}
				applyAll(state)
				refreshView()
				ensureMeta()
				if done != nil {
					done(err == nil)
				}
			})
		}()
	}

	/* -------------------- Bottom Actions (Dry Run / Apply / Undo) -------------------- */

	dryRunCheck := widget.NewCheck("Dry run (don't rename)", nil)
	dryRunCheck.SetChecked(true)

	undoLogCheck := widget.NewCheck("Create undo log (CSV)", nil)
	undoLogCheck.SetChecked(true)

	// chooseUndoLog asks where to save the undo log, then calls run with the
	// chosen path ("" means no log). Cancelling the dialog asks before going on.
	chooseUndoLog := func(run func(logPath string)) {
		if !undoLogCheck.Checked {
			run("")
			return
		}
		d := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if uc == nil { // cancelled
				dialog.ShowConfirm("No undo log",
					"You did not choose where to save the undo log.\n\nContinue WITHOUT an undo log?",
					func(yes bool) {
						if yes {
							run("")
						}
					}, w)
				return
			}
			path := uc.URI().Path()
			_ = uc.Close()
			run(path)
		}, w)
		d.SetFileName(fmt.Sprintf("undo_log_%s.csv", time.Now().Format("20060102_150405")))
		if lister, err := storage.ListerForURI(storage.NewFileURI(state.folderPath)); err == nil {
			d.SetLocation(lister)
		}
		d.Show()
	}

	saveLog := func(path string, items []rename.Item) string {
		if path == "" {
			return ""
		}
		if err := undo.WriteFile(path, items); err != nil {
			dialog.ShowError(fmt.Errorf("could not write the undo log: %w", err), w)
			return ""
		}
		return path
	}

	var showConfirm func(plan []rename.Item, summary rename.Summary)

	applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
		switch {
		case state.scanning:
			dialog.ShowInformation("Please wait", "The folder is still being scanned.", w)
			return
		case state.folderPath == "" || len(state.filteredFiles) == 0:
			dialog.ShowInformation("Nothing to do", "Select a folder and make sure some files match your filters.", w)
			return
		case len(state.selected()) == 0:
			dialog.ShowInformation("Nothing selected", "No files are selected. Use the checkboxes or Select All.", w)
			return
		case !state.hasActiveSteps():
			dialog.ShowInformation("No rename steps", "Add at least one enabled rename step first (“+ Add rename step”).", w)
			return
		}

		files := state.selected()
		steps := append([]rename.Step(nil), state.steps...)
		opt := rename.PlanOptions{Sidecars: state.sidecars, SidecarExts: state.sidecarExts, All: state.allFiles}
		var plan []rename.Item
		var summary rename.Summary
		runBusy("Preparing rename plan…", func() { plan, summary = rename.BuildPlan(files, steps, opt) }, func() {
			if summary.OkCount == 0 {
				dialog.ShowInformation("Nothing to rename", buildConfirmMessage(summary, false), w)
				return
			}
			showConfirm(plan, summary)
		})
	})

	showConfirm = func(plan []rename.Item, summary rename.Summary) {
		confirm := dialog.NewCustomConfirm("Confirm rename", "Proceed", "Cancel",
			container.NewVScroll(widget.NewLabel(buildConfirmMessage(summary, dryRunCheck.Checked))),
			func(ok bool) {
				if !ok {
					return
				}
				chooseUndoLog(func(logPath string) {
					if dryRunCheck.Checked {
						for i := range plan {
							if plan[i].Status == rename.StatusOK {
								plan[i].Status = rename.StatusDryRun
							}
						}
						saved := saveLog(logPath, plan)
						dialog.ShowInformation("Dry run complete", fmt.Sprintf(
							"%s\n\nNo files were changed. Untick “Dry run” and click Apply to rename for real.\n\nPlan CSV: %s",
							buildResultMessage(plan, true), prettyPath(saved)), w)
						return
					}

					var results []rename.Item
					runBusy("Renaming files…", func() { results = rename.Execute(plan) }, func() {
						saved := saveLog(logPath, results)
						if saved != "" {
							a.Preferences().SetString(lastUndoLogKey, saved)
						}
						msg := fmt.Sprintf("%s\n\nUndo log: %s", buildResultMessage(results, false), prettyPath(saved))
						dialog.ShowInformation("Apply complete", msg+strandedMessage(results), w)
						reloadFiles(true, nil)
					})
				})
			},
			w,
		)
		if len(summary.Invalid)+len(summary.Duplicate)+len(summary.TargetExists)+len(summary.GroupSkipped) > 0 {
			confirm.Resize(fyne.NewSize(700, 420)) // room for the lists of skipped files
		} else {
			confirm.Resize(fyne.NewSize(500, 280))
		}
		confirm.Show()
	}

	// runUndo reverts the rename log at path after confirmation.
	runUndo := func(path string) {
		entries, err := undo.ReadFile(path)
		if err != nil {
			dialog.ShowError(fmt.Errorf("could not read %s: %w", path, err), w)
			return
		}
		plan, sum := undo.BuildRevertPlan(entries)
		if sum.Total == 0 {
			dialog.ShowInformation("Nothing to undo",
				"This log contains no completed renames (it may be from a dry run).", w)
			return
		}
		if sum.Ready == 0 {
			dialog.ShowInformation("Cannot undo", buildUndoMessage(sum, path), w)
			return
		}
		dialog.ShowConfirm("Undo renames", buildUndoMessage(sum, path), func(ok bool) {
			if !ok {
				return
			}
			var results []rename.Item
			runBusy("Restoring names…", func() { results = rename.Execute(plan) }, func() {
				restored, _, errs := rename.Counts(results)
				if errs == 0 && len(sum.Missing) == 0 && len(sum.Blocked) == 0 {
					a.Preferences().SetString(lastUndoLogKey, "")
				}
				dialog.ShowInformation("Undo complete",
					fmt.Sprintf("Restored: %d\nErrors: %d", restored, errs)+strandedMessage(results), w)
				if state.folderPath != "" {
					reloadFiles(true, nil)
				}
			})
		}, w)
	}

	chooseUndoFile := func() {
		d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if rc == nil {
				return
			}
			path := rc.URI().Path()
			_ = rc.Close()
			runUndo(path)
		}, w)
		d.SetFilter(storage.NewExtensionFileFilter([]string{".csv"}))
		if state.folderPath != "" {
			if lister, err := storage.ListerForURI(storage.NewFileURI(state.folderPath)); err == nil {
				d.SetLocation(lister)
			}
		}
		d.Show()
	}

	undoBtn := widget.NewButtonWithIcon("Undo…", theme.HistoryIcon(), func() {
		last := a.Preferences().String(lastUndoLogKey)
		if last != "" {
			if _, err := os.Stat(last); err == nil {
				dialog.NewCustomConfirm("Undo a rename",
					"Use last log", "Choose another…",
					widget.NewLabel("Restore the original names using the most recent undo log?\n\n"+last),
					func(useLast bool) {
						if useLast {
							runUndo(last)
						} else {
							chooseUndoFile()
						}
					}, w).Show()
				return
			}
		}
		chooseUndoFile()
	})

	actionsBar := container.NewBorder(
		nil, nil,
		container.NewHBox(dryRunCheck, undoLogCheck),
		container.NewHBox(undoBtn, applyBtn),
	)

	right := container.NewBorder(
		rightTop,
		actionsBar,
		nil, nil,
		container.New(&resizeWatcher{onWidth: func(w float32) {
			state.listWidth = w
			updateRowHeights()
			list.Refresh()
		}}, container.NewStack(list, emptyView)),
	)

	/* -------------------- Left: Filters + Steps -------------------- */

	applyAllUI := func() {
		applyAll(state)
		refreshView()
		ensureMeta()
	}

	// Filters UI
	const matchAllLabel, matchAnyLabel = "Match ALL (AND)", "Match ANY (OR)"
	matchModeSelect := widget.NewSelect([]string{matchAllLabel, matchAnyLabel}, func(sel string) {
		state.matchAll = (sel == matchAllLabel)
		applyAllUI()
	})
	matchModeSelect.SetSelected(matchAllLabel)

	caseSensitiveCheck := widget.NewCheck("Case sensitive", func(v bool) {
		state.caseSensitive = v
		applyAllUI()
	})

	updateRule := func(id int, f func(*filter.Rule)) {
		for i := range state.filters {
			if state.filters[i].ID == id {
				f(&state.filters[i])
				return
			}
		}
	}

	filtersBox := container.NewVBox()
	var renderFilters func()
	renderFilters = func() {
		filtersBox.Objects = nil
		if len(state.filters) == 0 {
			lbl := widget.NewLabel("No filters: all files are included. Click “+ Add filter” to narrow them down.")
			lbl.Wrapping = fyne.TextWrapWord
			filtersBox.Add(lbl)
			filtersBox.Refresh()
			return
		}

		for _, rule := range state.filters {
			rid := rule.ID

			errLbl := widget.NewLabel("")
			errLbl.Importance = widget.DangerImportance
			errLbl.Wrapping = fyne.TextWrapWord
			showErr := func() {
				msg := ""
				updateRule(rid, func(r *filter.Rule) { msg = filter.Validate(*r) })
				errLbl.SetText(msg)
				if msg == "" {
					errLbl.Hide()
				} else {
					errLbl.Show()
				}
			}
			showErr()

			modeSel := widget.NewSelect(filter.Modes, func(sel string) {
				changed := false
				updateRule(rid, func(r *filter.Rule) { changed = r.Mode != sel; r.Mode = sel })
				if !changed {
					return
				}
				renderFilters() // refresh the hint for the new mode
				applyAllUI()
			})
			modeSel.Selected = rule.Mode // direct set: avoid re-triggering OnChanged

			valEntry := widget.NewEntry()
			valEntry.SetText(rule.Value)
			valEntry.SetPlaceHolder(`value… e.g. IMG_, holiday, png`)
			valEntry.OnChanged = func(s string) {
				updateRule(rid, func(r *filter.Rule) { r.Value = s })
				showErr()
				applyAllUI()
			}

			removeBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				next := make([]filter.Rule, 0, len(state.filters))
				for _, r := range state.filters {
					if r.ID != rid {
						next = append(next, r)
					}
				}
				state.filters = next
				renderFilters()
				applyAllUI()
			})

			hint := widget.NewLabelWithStyle(filterHelp[rule.Mode], fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			hint.Wrapping = fyne.TextWrapWord

			filtersBox.Add(container.NewBorder(nil, nil, nil, removeBtn,
				container.NewVBox(container.NewGridWithColumns(2, modeSel, valEntry), hint, errLbl),
			))
			filtersBox.Add(widget.NewSeparator())
		}
		filtersBox.Refresh()
	}

	addFilterBtn := widget.NewButton("+ Add filter", func() {
		state.nextFilterID++
		state.filters = append(state.filters, filter.Rule{ID: state.nextFilterID, Mode: filter.Contains})
		renderFilters()
		applyAllUI()
	})

	clearFiltersBtn := widget.NewButton("Clear filters", func() {
		state.filters = nil
		renderFilters()
		applyAllUI()
	})

	// Steps UI
	opNames := make([]string, len(rename.AllOps))
	for i, op := range rename.AllOps {
		opNames[i] = string(op)
	}

	updateStep := func(id int, f func(*rename.Step)) {
		for i := range state.steps {
			if state.steps[i].ID == id {
				f(&state.steps[i])
				return
			}
		}
	}

	stepsBox := container.NewVBox()
	var renderSteps func()
	collapsed := map[int]bool{} // step ID -> shown as a one-line summary
	var sampleFns []func()

	moveStep := func(id, delta int) {
		for i := range state.steps {
			if state.steps[i].ID == id {
				j := i + delta
				if j < 0 || j >= len(state.steps) {
					return
				}
				state.steps[i], state.steps[j] = state.steps[j], state.steps[i]
				break
			}
		}
		renderSteps()
		applyAllUI()
	}

	renderSteps = func() {
		stepsBox.Objects = nil
		sampleFns = nil
		if len(state.steps) > 1 {
			setAll := func(v bool) {
				for _, st := range state.steps {
					collapsed[st.ID] = v
				}
				renderSteps()
			}
			collapseAll := widget.NewButton("Collapse all", func() { setAll(true) })
			expandAll := widget.NewButton("Expand all", func() { setAll(false) })
			collapseAll.Importance, expandAll.Importance = widget.LowImportance, widget.LowImportance
			stepsBox.Add(container.NewHBox(collapseAll, expandAll))
		}
		if len(state.steps) == 0 {
			lbl := widget.NewLabel("No rename steps yet. Click “+ Add rename step” (or load a preset); the preview updates as you type.")
			lbl.Wrapping = fyne.TextWrapWord
			stepsBox.Add(lbl)
			stepsBox.Refresh()
			return
		}

		for n := range state.steps {
			n, step := n, state.steps[n]
			sid := step.ID

			// pick-list ops always hold a valid option
			if opts := rename.Options(step.Op); opts != nil && !contains(opts, step.A) {
				state.steps[n].A = opts[0]
				step.A = opts[0]
			}

			errLbl := widget.NewLabel("")
			errLbl.Importance = widget.DangerImportance
			errLbl.Wrapping = fyne.TextWrapWord
			showErr := func() {
				msg := ""
				updateStep(sid, func(s *rename.Step) { msg = rename.ValidateStep(*s) })
				errLbl.SetText(msg)
				if msg == "" {
					errLbl.Hide()
				} else {
					errLbl.Show()
				}
			}
			showErr()

			enabled := widget.NewCheck(fmt.Sprintf("Step %d", n+1), nil)
			enabled.Checked = !step.Disabled
			enabled.OnChanged = func(on bool) {
				updateStep(sid, func(s *rename.Step) { s.Disabled = !on })
				applyAllUI()
			}

			upBtn := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() { moveStep(sid, -1) })
			downBtn := widget.NewButtonWithIcon("", theme.MoveDownIcon(), func() { moveStep(sid, 1) })
			if n == 0 {
				upBtn.Disable()
			}
			if n == len(state.steps)-1 {
				downBtn.Disable()
			}
			remove := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				next := make([]rename.Step, 0, len(state.steps))
				for _, s := range state.steps {
					if s.ID != sid {
						next = append(next, s)
					}
				}
				state.steps = next
				renderSteps()
				applyAllUI()
			})

			opSel := widget.NewSelect(opNames, func(sel string) {
				changed := false
				updateStep(sid, func(s *rename.Step) {
					newOp := rename.Op(sel)
					if s.Op == newOp {
						return
					}
					changed = true
					if rename.Options(s.Op) != nil || rename.Options(newOp) != nil {
						s.A, s.B = "", "" // do not carry pick-list values into text fields
					}
					s.Op = newOp
				})
				if !changed {
					return
				}
				renderSteps() // refresh placeholders, pick-lists and hint
				applyAllUI()
			})
			opSel.Selected = string(step.Op) // direct set: avoid re-triggering OnChanged

			aPlace, bPlace, bUsed := stepPlaceholders(step.Op)

			var aObj fyne.CanvasObject
			if opts := rename.Options(step.Op); opts != nil {
				aSel := widget.NewSelect(opts, func(v string) {
					updateStep(sid, func(s *rename.Step) { s.A = v })
					applyAllUI()
				})
				aSel.Selected = step.A
				aObj = aSel
			} else {
				aEntry := widget.NewEntry()
				aEntry.SetText(step.A)
				aEntry.SetPlaceHolder(aPlace)
				aEntry.OnChanged = func(v string) {
					updateStep(sid, func(s *rename.Step) { s.A = v })
					showErr()
					applyAllUI()
				}
				aObj = aEntry
			}

			bEntry := widget.NewEntry()
			bEntry.SetText(step.B)
			bEntry.SetPlaceHolder(bPlace)
			bEntry.OnChanged = func(v string) {
				updateStep(sid, func(s *rename.Step) { s.B = v })
				showErr()
				applyAllUI()
			}
			if !bUsed {
				bEntry.Disable()
			}

			hint := widget.NewLabelWithStyle(opHelp[step.Op], fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
			hint.Wrapping = fyne.TextWrapWord

			isCollapsed := collapsed[sid]
			chevron := theme.MenuDropDownIcon()
			if isCollapsed {
				chevron = theme.MenuExpandIcon()
			}
			toggle := widget.NewButtonWithIcon("", chevron, func() {
				collapsed[sid] = !collapsed[sid]
				renderSteps()
			})
			toggle.Importance = widget.LowImportance
			headerObjs := []fyne.CanvasObject{}
			if isCollapsed { // one-line summary between the toggle and the buttons
				summary := widget.NewLabel(describeStep(step))
				summary.Truncation = fyne.TextTruncateEllipsis
				headerObjs = append(headerObjs, summary)
			}
			stepsBox.Add(container.NewBorder(nil, nil,
				container.NewHBox(toggle, enabled), container.NewHBox(upBtn, downBtn, remove), headerObjs...))

			if !isCollapsed {
				stepsBox.Add(opSel)
				stepsBox.Add(container.NewGridWithColumns(2, aObj, bEntry))
				if step.Op == rename.OpNumber || step.Op == rename.OpTemplate {
					perFolder := widget.NewCheck("Restart numbering in each folder", nil)
					perFolder.Checked = step.Flag
					perFolder.OnChanged = func(v bool) {
						updateStep(sid, func(s *rename.Step) { s.Flag = v })
						applyAllUI()
					}
					stepsBox.Add(perFolder)
				}
				stepsBox.Add(hint)

				// sample: what this step does to the first selected file
				sample := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
				sample.Wrapping = fyne.TextWrapWord

				sampleFns = append(sampleFns, func() {
					idx := -1
					for i := range state.steps {
						if state.steps[i].ID == sid {
							idx = i
						}
					}
					// show the first selected file this step actually changes
					// (or just the first selected file if it changes none)
					if idx < 0 {
						sample.SetText("")
						sample.Hide()
						return
					}
					var first, before, after string
					scanned := 0
					for _, p := range state.filteredFiles {
						if state.deselected[p] {
							continue
						}
						b := rename.PreviewNames([]string{p}, state.steps[:idx])[0]
						a := rename.PreviewNames([]string{p}, state.steps[:idx+1])[0]
						if first == "" {
							first, before, after = p, b, a
						}
						if b != a {
							before, after = b, a
							break
						}
						if scanned++; scanned >= 300 {
							break
						}
					}
					if idx < 0 || first == "" {
						sample.SetText("")
						sample.Hide()
						return
					}
					sample.Show()
					if state.steps[idx].Disabled {
						sample.SetText("This step is switched off.")
						return
					}
					if before == after {
						sample.SetText(fmt.Sprintf("Example: %s (no change for this file)", before))
					} else {
						sample.SetText(fmt.Sprintf("Example: %s  ->  %s", before, after))
					}
				})
				stepsBox.Add(sample)
			}
			stepsBox.Add(errLbl)
			stepsBox.Add(widget.NewSeparator())
		}
		stepsBox.Refresh()
		if updateSamples != nil {
			updateSamples()
		}
	}
	updateSamples = func() {
		for _, f := range sampleFns {
			f()
		}
	}

	addStepBtn := widget.NewButton("+ Add rename step", func() {
		state.nextStepID++
		state.steps = append(state.steps, rename.Step{ID: state.nextStepID, Op: rename.OpReplaceText})
		renderSteps()
		applyAllUI()
	})

	clearStepsBtn := widget.NewButton("Clear steps", func() {
		state.steps = nil
		renderSteps()
		applyAllUI()
	})

	/* -------------------- Presets -------------------- */

	currentPreset := func() preset.Preset {
		return preset.Preset{
			Steps:         append([]rename.Step(nil), state.steps...),
			Filters:       append([]filter.Rule(nil), state.filters...),
			MatchAll:      state.matchAll,
			CaseSensitive: state.caseSensitive,
		}
	}

	loadPreset := func(p preset.Preset) {
		state.steps, state.filters = nil, nil
		for _, s := range p.Steps {
			state.nextStepID++
			s.ID = state.nextStepID
			state.steps = append(state.steps, s)
		}
		for _, r := range p.Filters {
			state.nextFilterID++
			r.ID = state.nextFilterID
			state.filters = append(state.filters, r)
		}
		state.matchAll, state.caseSensitive = p.MatchAll, p.CaseSensitive
		// direct field sets: do not fire OnChanged
		matchModeSelect.Selected = matchAnyLabel
		if p.MatchAll {
			matchModeSelect.Selected = matchAllLabel
		}
		matchModeSelect.Refresh()
		caseSensitiveCheck.Checked = p.CaseSensitive
		caseSensitiveCheck.Refresh()
		renderFilters()
		renderSteps()
		applyAllUI()
	}

	presetsBtn := widget.NewButtonWithIcon("Presets…", theme.DocumentIcon(), func() {
		showPresets(w, presetHooks{current: currentPreset, apply: loadPreset})
	})

	sidecarCheck := widget.NewCheck("Also rename related files (.xmp sidecars, RAW+JPEG pairs)", func(v bool) {
		state.sidecars = v
		a.Preferences().SetBool(sidecarsKey, v)
		applyAllUI()
	})
	sidecarCheck.Checked = state.sidecars
	sidecarExtLbl := widget.NewLabel("")
	updateSidecarLbl := func() { sidecarExtLbl.SetText(fmt.Sprintf("%d file types", len(state.sidecarSet))) }
	updateSidecarLbl()
	sidecarExtBtn := widget.NewButton("Edit file types…", func() {
		entry := widget.NewMultiLineEntry()
		entry.Wrapping = fyne.TextWrapWord
		entry.SetMinRowsVisible(4)
		entry.SetText(orDefault(state.sidecarExts, rename.DefaultSidecarExts))
		intro := widget.NewLabel("File types that are renamed together with the main file (separate with commas or spaces):")
		intro.Wrapping = fyne.TextWrapWord
		reset := widget.NewButton("Reset to default", func() { entry.SetText(rename.DefaultSidecarExts) })
		d := dialog.NewCustomConfirm("Related file types", "Save", "Cancel",
			container.NewVBox(intro, entry, reset),
			func(ok bool) {
				if !ok {
					return
				}
				v := strings.TrimSpace(entry.Text)
				state.sidecarExts = v
				state.sidecarSet = rename.ParseExts(orDefault(v, rename.DefaultSidecarExts))
				a.Preferences().SetString(sidecarExtsKey, v)
				updateSidecarLbl()
				applyAllUI()
			}, w)
		d.Resize(fyne.NewSize(520, 340))
		d.Show()
	})
	sidecarHint := widget.NewLabelWithStyle(sidecarHelp, fyne.TextAlignLeading, fyne.TextStyle{Italic: true})
	sidecarHint.Wrapping = fyne.TextWrapWord

	needHelp := widget.NewLabel("Click Help (or press F1) for a guide to filters, rename steps, presets, conflicts and undo.")
	needHelp.Wrapping = fyne.TextWrapWord

	left := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("1. Filters (optional)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		matchModeSelect,
		caseSensitiveCheck,
		container.NewHBox(addFilterBtn, clearFiltersBtn),
		widget.NewSeparator(),
		filtersBox,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("2. Rename steps (applied top to bottom)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(addStepBtn, clearStepsBtn, presetsBtn),
		widget.NewSeparator(),
		stepsBox,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("3. Options", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		sidecarCheck,
		container.NewHBox(sidecarExtBtn, sidecarExtLbl),
		sidecarHint,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("Need help?", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		needHelp,
	))

	renderFilters()
	renderSteps()

	/* -------------------- Top bar -------------------- */

	selectedFolderLabel := widget.NewLabel("Folder: (none)")
	selectedFolderLabel.Truncation = fyne.TextTruncateEllipsis

	// Recursive toggle — reloads current folder when toggled
	recursiveCheck := widget.NewCheck("Include subfolders", func(v bool) {
		state.recursive = v
		if state.folderPath == "" {
			return
		}
		reloadFiles(false, nil)
	})

	loadFolder := func(path string) {
		state.folderPath = path
		selectedFolderLabel.SetText("Folder: " + displayPath(path))
		reloadFiles(false, func(ok bool) {
			if ok {
				saveRecentFolder(path)
			}
		})
	}

	// Recent folders dropdown — populated from persisted preferences
	recentSelect = widget.NewSelect(append([]string{recentPlaceholder}, getRecentFolders()...), func(sel string) {
		if sel == recentPlaceholder {
			return
		}
		loadFolder(sel)
	})
	recentSelect.SetSelected(recentPlaceholder)

	refreshBtn := widget.NewButton("Refresh", func() {
		if state.folderPath == "" {
			return
		}
		loadFolder(state.folderPath)
	})

	selectFolderBtn := widget.NewButton("Select Folder…", func() {
		dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			loadFolder(uri.Path())
		}, w).Show()
	})

	// Text size: A− / A+ (remembered). Row heights are recomputed for the new font.
	var smallerBtn, largerBtn *widget.Button
	syncScaleButtons := func() {
		if textScaleIdx == 0 {
			smallerBtn.Disable()
		} else {
			smallerBtn.Enable()
		}
		if textScaleIdx == len(textScales)-1 {
			largerBtn.Disable()
		} else {
			largerBtn.Enable()
		}
	}
	applyTextScale := func(idx int) {
		textScaleIdx = idx
		a.Preferences().SetInt(textScaleKey, idx)
		a.Settings().SetTheme(newScaledTheme(textScales[idx], themeMode))
		syncScaleButtons()
		refreshView()
	}
	smallerBtn = widget.NewButton("A−", func() { applyTextScale(textScaleIdx - 1) })
	largerBtn = widget.NewButton("A+", func() { applyTextScale(textScaleIdx + 1) })
	syncScaleButtons()

	// Colour mode: Auto (follow the system) → Light → Dark (remembered).
	var themeBtn *widget.Button
	themeBtn = widget.NewButtonWithIcon(modeNames[themeMode], theme.ColorPaletteIcon(), func() {
		themeMode = (themeMode + 1) % len(modeNames)
		a.Preferences().SetInt(themeModeKey, themeMode)
		themeBtn.SetText(modeNames[themeMode])
		a.Settings().SetTheme(newScaledTheme(textScales[textScaleIdx], themeMode))
		refreshView()
	})

	helpBtn := widget.NewButtonWithIcon("Help", theme.HelpIcon(), func() { showHelp(w, 0) })

	aboutBtn := widget.NewButton("About", func() { showAbout(w) })

	brand := container.NewHBox(
		brandImage(28),
		widget.NewLabelWithStyle("RenForge", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(brand, widget.NewSeparator(), selectFolderBtn, recentSelect, refreshBtn, recursiveCheck),
		container.NewHBox(smallerBtn, largerBtn, themeBtn, helpBtn, aboutBtn),
		selectedFolderLabel,
	)

	/* -------------------- Layout -------------------- */

	split := container.NewHSplit(left, right)
	split.Offset = 0.40
	if off := a.Preferences().FloatWithFallback(splitKey, 0.40); off > 0.15 && off < 0.85 {
		split.Offset = off
	}

	root := container.NewBorder(topBar, nil, nil, nil, split)

	// Remember window size and the split position between sessions.
	var lastSize fyne.Size
	var geomTimer *time.Timer
	saveGeometry := func() {
		if lastSize.Width > 0 {
			a.Preferences().SetFloat(winWKey, float64(lastSize.Width))
			a.Preferences().SetFloat(winHKey, float64(lastSize.Height))
		}
		a.Preferences().SetFloat(splitKey, split.Offset)
	}
	w.SetContent(container.New(&resizeWatcher{onSize: func(sz fyne.Size) {
		lastSize = sz
		if geomTimer != nil {
			geomTimer.Stop()
		}
		off := split.Offset
		geomTimer = time.AfterFunc(700*time.Millisecond, func() {
			a.Preferences().SetFloat(winWKey, float64(sz.Width))
			a.Preferences().SetFloat(winHKey, float64(sz.Height))
			a.Preferences().SetFloat(splitKey, off)
		})
	}}, root))
	a.Lifecycle().SetOnStopped(saveGeometry)

	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) {
		switch ev.Name {
		case fyne.KeyF1:
			showHelp(w, 0)
		case fyne.KeyEscape:
			closeInfoDialog() // Help / About
		}
	})

	// Drop a folder (or a file: its folder) on the window to open it.
	w.SetOnDropped(func(_ fyne.Position, uris []fyne.URI) {
		for _, u := range uris {
			p := u.Path()
			fi, err := os.Stat(p)
			if err != nil {
				continue
			}
			if !fi.IsDir() {
				p = filepath.Dir(p)
			}
			loadFolder(p)
			return
		}
	})

	// Show the guide automatically the first time the app is opened.
	a.Lifecycle().SetOnStarted(func() {
		if !a.Preferences().Bool(seenHelpKey) {
			a.Preferences().SetBool(seenHelpKey, true)
			showHelp(w, 0)
		}
	})

	refreshView()

	return &appUI{
		state:       state,
		loadFolder:  loadFolder,
		loadPreset:  loadPreset,
		refreshView: refreshView,
		applyBtn:    applyBtn,
		collapse: func(i int) {
			if i >= 0 && i < len(state.steps) {
				collapsed[state.steps[i].ID] = true
				renderSteps()
			}
		},
	}
}

/* -------------------- Apply Pipeline Helpers -------------------- */

func applyAll(state *AppState) {
	state.filteredFiles = nil
	if len(state.allFiles) > 0 {
		state.filteredFiles = filter.Files(state.allFiles, state.filters, state.matchAll, state.caseSensitive)
	}
	state.indexPaths()
	state.sidecarIx = nil
	if state.sidecars {
		state.sidecarIx = rename.NewSidecarIndex(state.allFiles)
	}
	sortFiles(state)
	recomputePreview(state)
}

// sortFiles orders the file list. Date and size use cached metadata (files not
// loaded yet count as equal and keep natural name order until they are).
func sortFiles(state *AppState) {
	files := state.filteredFiles
	need := sortNeed(state.sortMode)
	byName := func(i, j int) bool { return rename.NaturalLess(files[i], files[j]) }

	key := func(p string) (t time.Time, size int64) {
		if info, ok := meta.Cached(p, need); ok {
			return info.Date(), info.Size
		}
		return
	}
	switch state.sortMode {
	case sortNameDesc:
		sort.SliceStable(files, func(i, j int) bool { return byName(j, i) })
	case sortDateAsc, sortDateDesc:
		desc := state.sortMode == sortDateDesc
		sort.SliceStable(files, func(i, j int) bool {
			ti, _ := key(files[i])
			tj, _ := key(files[j])
			if ti.Equal(tj) {
				return byName(i, j)
			}
			return ti.Before(tj) != desc
		})
	case sortSizeAsc, sortSizeDesc:
		desc := state.sortMode == sortSizeDesc
		sort.SliceStable(files, func(i, j int) bool {
			_, si := key(files[i])
			_, sj := key(files[j])
			if si == sj {
				return byName(i, j)
			}
			return (si < sj) != desc
		})
	default:
		// natural name order is the order ListFiles already produced
		sort.SliceStable(files, byName)
	}
}

// relatedCount is the number of related files that would be renamed along
// with the primary file (those that are not selected files themselves).
func relatedCount(state *AppState, primary string) int {
	n := 0
	for _, r := range state.sidecarIx.For(primary, state.sidecarSet) {
		if _, isPrimary := state.previewNames[r.Path]; !isPrimary {
			n++
		}
	}
	return n
}

// displayPath shows the home folder as "~" to keep the folder label short.
func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// recomputePreview recalculates the new name of every selected file. A file's
// position among the selected files is its number for "Number files".
func recomputePreview(state *AppState) {
	sel := state.selected()
	names := rename.PreviewNames(sel, state.steps)
	state.previewNames = make(map[string]string, len(sel))
	state.previewCounts = make(map[string]int, len(sel))
	for i, p := range sel {
		state.previewNames[p] = names[i]
		// keyed by (dir, name) so files in different subdirs don't false-conflict
		state.previewCounts[rename.DupKey(filepath.Dir(p), names[i])]++
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

/* -------------------- Messages -------------------- */

func appendList(b *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	b.WriteString(title + "\n")
	for _, s := range firstN(lines, 20) {
		b.WriteString(" - " + s + "\n")
	}
	if len(lines) > 20 {
		b.WriteString(fmt.Sprintf(" ... and %d more\n", len(lines)-20))
	}
	b.WriteString("\n")
}

func buildConfirmMessage(sum rename.Summary, dryRun bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Files selected: %d\n", sum.Total))
	b.WriteString(fmt.Sprintf("Will rename: %d\n", sum.OkCount))
	b.WriteString(fmt.Sprintf("Unchanged (skipped): %d\n", sum.Unchanged))

	if sum.Sidecars > 0 {
		b.WriteString(fmt.Sprintf("  of which related files (sidecars, pairs): %d\n", sum.Sidecars))
	}
	b.WriteString("\n")
	appendList(&b, "Invalid names (skipped):", sum.Invalid)
	appendList(&b, "Duplicate preview conflicts (skipped):", sum.Duplicate)
	appendList(&b, "Target already exists on disk (skipped):", sum.TargetExists)
	appendList(&b, "Skipped because a related file cannot be renamed:", sum.GroupSkipped)

	switch {
	case sum.OkCount == 0:
		b.WriteString("Nothing can be renamed with the current steps.")
	case dryRun:
		b.WriteString("Dry run is ON: no files will be changed.\nProceed?")
	default:
		b.WriteString("Dry run is OFF: files WILL be renamed.\nProceed?")
	}
	return b.String()
}

func buildResultMessage(items []rename.Item, dryRun bool) string {
	renamed, skipped, errs := rename.Counts(items)
	if dryRun {
		return fmt.Sprintf("Would rename: %d\nSkipped: %d\nErrors: %d", renamed, skipped, errs)
	}
	return fmt.Sprintf("Renamed: %d\nSkipped: %d\nErrors: %d", renamed, skipped, errs)
}

// strandedMessage warns about files left under a temporary name, if any.
func strandedMessage(items []rename.Item) string {
	st := rename.Stranded(items)
	if len(st) == 0 {
		return ""
	}
	msg := "\n\n⚠ Some files could not be restored and are still under a temporary name:\n"
	for _, it := range firstN(st, 10) {
		msg += " - " + it.TempPath + "\n"
	}
	return msg
}

func buildUndoMessage(sum undo.RevertSummary, path string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Log: %s\n\n", path))
	b.WriteString(fmt.Sprintf("Renames in log: %d\nCan be restored: %d\n\n", sum.Total, sum.Ready))
	appendList(&b, "Not found (moved or deleted since):", sum.Missing)
	appendList(&b, "Original name is in use again (skipped):", sum.Blocked)
	if sum.Ready > 0 {
		b.WriteString("Restore the original names?")
	}
	return b.String()
}

/* -------------------- small helpers -------------------- */

func firstN[T any](in []T, n int) []T {
	if len(in) <= n {
		return in
	}
	return in[:n]
}

func prettyPath(p string) string {
	if strings.TrimSpace(p) == "" {
		return "(not saved)"
	}
	return p
}
