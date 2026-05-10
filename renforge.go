package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

/* -------------------- Filters -------------------- */

type FilterRule struct {
	ID    int
	Mode  string // "contains", "starts with", "ends with", "extension"
	Value string
}

/* -------------------- Rename Steps -------------------- */

type RenameOp string

const (
	OpRemoveText      RenameOp = "Remove text"
	OpReplaceText     RenameOp = "Replace text"
	OpInsertBeforeExt RenameOp = "Insert before extension"
	OpChangeExt       RenameOp = "Change extension"
	OpAppend          RenameOp = "Append"
	OpPrepend         RenameOp = "Prepend"
)

type RenameStep struct {
	ID int
	Op RenameOp
	A  string
	B  string
}

/* -------------------- App State -------------------- */

type AppState struct {
	folderPath string
	recursive  bool

	allFiles      []string
	filteredFiles []string
	viewFiles     []string

	page     int
	pageSize int

	filters       []FilterRule
	nextFilterID  int
	matchAll      bool
	caseSensitive bool

	steps      []RenameStep
	nextStepID int

	// key: "dir\x00previewName" -> count among selected files in that dir
	previewCounts map[string]int
	// paths the user has explicitly excluded from the apply operation
	deselected map[string]bool
}

type RenamePlanItem struct {
	OldPath string
	NewPath string
	OldName string
	NewName string
	Status  string // "ok" | "skip" | "renamed" | "error" | "dry-run"
	Reason  string
}

const (
	recentFoldersKey = "recent_folders"
	maxRecentFolders = 5
)

func main() {
	a := app.NewWithID("com.blackarck.renforge")
	w := a.NewWindow("File Rename Utility")
	w.Resize(fyne.NewSize(1040, 680))

	state := &AppState{
		pageSize:      10,
		matchAll:      true,
		previewCounts: map[string]int{},
		deselected:    map[string]bool{},
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
		recents := getRecentFolders()
		next := recents[:0]
		for _, r := range recents {
			if r != path {
				next = append(next, r)
			}
		}
		next = append([]string{path}, next...)
		if len(next) > maxRecentFolders {
			next = next[:maxRecentFolders]
		}
		a.Preferences().SetString(recentFoldersKey, strings.Join(next, "|"))
		if recentSelect != nil {
			opts := append([]string{"Open recent…"}, next...)
			recentSelect.SetOptions(opts)
			recentSelect.SetSelected("Open recent…")
		}
	}

	/* -------------------- Right: Preview -------------------- */

	resultsHeader := widget.NewLabel("No folder selected.")
	resultsHeader.TextStyle = fyne.TextStyle{Bold: true}

	previewBox := container.NewVBox()

	makeCell := func(text string) *widget.RichText {
		rt := widget.NewRichText(&widget.TextSegment{
			Text: text,
			Style: widget.RichTextStyle{
				SizeName: theme.SizeNameCaptionText,
			},
		})
		rt.Wrapping = fyne.TextWrapWord
		return rt
	}

	selCount := func() int {
		n := 0
		for _, p := range state.filteredFiles {
			if !state.deselected[p] {
				n++
			}
		}
		return n
	}

	// forward declaration: renderPreview and updatePageView reference each other
	var updatePageView func()

	renderPreview := func() {
		previewBox.Objects = nil

		h0 := widget.NewLabelWithStyle("✓", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		h1 := widget.NewLabelWithStyle("Original (full file name)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		h2 := widget.NewLabelWithStyle("Preview (after rename steps)", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		previewBox.Add(container.NewGridWithColumns(3, h0, h1, h2))
		previewBox.Add(widget.NewSeparator())

		for _, full := range state.viewFiles {
			full := full // per-iteration variable for closure
			origName := filepath.Base(full)
			prevName := applyRenameSteps(origName, state.steps)

			warn := ""
			if reason := invalidNameReason(prevName); reason != "" {
				warn = "  ⚠ " + reason
			} else if !state.deselected[full] {
				// only show conflict warning for selected files
				dirKey := filepath.Dir(full) + "\x00" + prevName
				if state.previewCounts[dirKey] > 1 && prevName != origName {
					warn = "  ⚠ conflict"
				} else if prevName != origName {
					target := filepath.Join(filepath.Dir(full), prevName)
					if _, err := os.Stat(target); err == nil {
						warn = "  ⚠ target exists"
					}
				}
			}

			chk := widget.NewCheck("", func(checked bool) {
				if checked {
					delete(state.deselected, full)
				} else {
					state.deselected[full] = true
				}
				recomputePreviewCounts(state)
				updatePageView()
			})
			chk.Checked = !state.deselected[full] // direct field set; SetChecked would trigger OnChanged → infinite loop

			previewBox.Add(container.NewGridWithColumns(3,
				chk,
				makeCell(origName),
				makeCell(prevName+warn),
			))
			previewBox.Add(widget.NewSeparator())
		}
		previewBox.Refresh()
	}

	/* -------------------- Pagination -------------------- */

	prevBtn := widget.NewButton("Previous", nil)
	nextBtn := widget.NewButton("Next", nil)
	pageLabel := widget.NewLabel("Page 1/1")
	pageLabel.Alignment = fyne.TextAlignCenter

	updatePageView = func() {
		totalMatches := len(state.filteredFiles)
		totalFiles := len(state.allFiles)

		pages := pageCount(totalMatches, state.pageSize)
		state.page = clamp(state.page, 0, pages-1)

		start := state.page * state.pageSize
		end := start + state.pageSize
		if start > totalMatches {
			start = totalMatches
		}
		if end > totalMatches {
			end = totalMatches
		}

		state.viewFiles = state.filteredFiles[start:end]

		sel := selCount()
		if totalMatches == 0 {
			resultsHeader.SetText(fmt.Sprintf("No matches (0 of %d files).", totalFiles))
		} else {
			resultsHeader.SetText(fmt.Sprintf(
				"Showing %d–%d of %d matches · %d selected · %d total files.",
				start+1, end, totalMatches, sel, totalFiles,
			))
		}

		pageLabel.SetText(fmt.Sprintf("Page %d/%d", state.page+1, pages))

		prevBtn.Disable()
		nextBtn.Disable()
		if state.page > 0 && totalMatches > 0 {
			prevBtn.Enable()
		}
		if state.page < pages-1 && totalMatches > 0 {
			nextBtn.Enable()
		}

		renderPreview()
	}

	prevBtn.OnTapped = func() { state.page--; updatePageView() }
	nextBtn.OnTapped = func() { state.page++; updatePageView() }

	/* -------------------- Select All / Deselect All -------------------- */

	selectAllBtn := widget.NewButton("Select All", func() {
		state.deselected = map[string]bool{}
		recomputePreviewCounts(state)
		updatePageView()
	})

	deselectAllBtn := widget.NewButton("Deselect All", func() {
		for _, p := range state.filteredFiles {
			state.deselected[p] = true
		}
		recomputePreviewCounts(state)
		updatePageView()
	})

	rightTop := container.NewVBox(
		container.NewBorder(nil, nil, nil,
			container.NewHBox(prevBtn, pageLabel, nextBtn),
			resultsHeader,
		),
		container.NewHBox(selectAllBtn, deselectAllBtn),
		widget.NewSeparator(),
	)

	/* -------------------- Bottom Actions (Dry Run / Apply / Undo CSV) -------------------- */

	dryRunCheck := widget.NewCheck("Dry run (don't rename)", nil)
	dryRunCheck.SetChecked(true)

	undoLogCheck := widget.NewCheck("Create undo log (CSV)", nil)
	undoLogCheck.SetChecked(true)

	applyBtn := widget.NewButtonWithIcon("Apply", theme.ConfirmIcon(), func() {
		if state.folderPath == "" || len(state.filteredFiles) == 0 {
			dialog.ShowInformation("Nothing to do", "Select a folder and ensure you have matching files.", w)
			return
		}
		if selCount() == 0 {
			dialog.ShowInformation("Nothing selected", "No files are selected for rename. Use the checkboxes or Select All.", w)
			return
		}

		plan, summary := buildPlan(state)
		msg := buildConfirmMessage(summary)

		doWithOptionalCSV := func(onSaved func(savePath string)) {
			if !undoLogCheck.Checked {
				onSaved("")
				return
			}
			saveName := fmt.Sprintf("undo_log_%s.csv", time.Now().Format("20060102_150405"))
			d := dialog.NewFileSave(func(uc fyne.URIWriteCloser, err error) {
				if err != nil || uc == nil {
					onSaved("")
					return
				}
				defer uc.Close()
				if err := writeUndoCSV(uc, plan); err != nil {
					dialog.ShowError(err, w)
					onSaved("")
					return
				}
				onSaved(uc.URI().Path())
			}, w)
			d.SetFileName(saveName)
			d.Show()
		}

		confirm := dialog.NewCustomConfirm("Confirm rename", "Proceed", "Cancel",
			container.NewVScroll(widget.NewLabel(msg)),
			func(ok bool) {
				if !ok {
					return
				}

				doWithOptionalCSV(func(savedCSV string) {
					if dryRunCheck.Checked {
						for i := range plan {
							if plan[i].Status == "ok" {
								plan[i].Status = "dry-run"
							}
						}
						dialog.ShowInformation("Dry run complete", fmt.Sprintf(
							"%s\n\nUndo CSV: %s",
							buildResultMessage(plan, true),
							prettyPath(savedCSV),
						), w)
						return
					}

					applyResults := applyRenames(plan)

					if savedCSV != "" {
						_ = overwriteUndoCSV(savedCSV, applyResults)
					}

					dialog.ShowInformation("Apply complete", fmt.Sprintf(
						"%s\n\nUndo CSV: %s",
						buildResultMessage(applyResults, false),
						prettyPath(savedCSV),
					), w)

					files, err := listAllFiles(state.folderPath, state.recursive)
					if err == nil {
						state.allFiles = files
						applyAll(state)
						updatePageView()
					}
				})
			},
			w,
		)
		confirm.Resize(fyne.NewSize(700, 420))
		confirm.Show()
	})

	actionsBar := container.NewBorder(
		nil, nil,
		container.NewHBox(dryRunCheck, undoLogCheck),
		nil,
		applyBtn,
	)

	right := container.NewBorder(
		rightTop,
		actionsBar,
		nil, nil,
		container.NewVScroll(previewBox),
	)

	/* -------------------- Left: Filters + Steps -------------------- */

	applyAllUI := func() {
		applyAll(state)
		updatePageView()
	}

	// Filters UI
	matchModeSelect := widget.NewSelect([]string{"Match ALL (AND)", "Match ANY (OR)"}, func(sel string) {
		state.matchAll = (sel == "Match ALL (AND)")
		applyAllUI()
	})
	matchModeSelect.SetSelected("Match ALL (AND)")

	caseSensitiveCheck := widget.NewCheck("Case sensitive", func(v bool) {
		state.caseSensitive = v
		applyAllUI()
	})

	filtersBox := container.NewVBox()
	var renderFilters func()
	renderFilters = func() {
		filtersBox.Objects = nil
		if len(state.filters) == 0 {
			filtersBox.Add(widget.NewLabel("No filters added. Add one to narrow down files."))
			filtersBox.Refresh()
			return
		}

		for _, rule := range state.filters {
			rid := rule.ID

			modeSel := widget.NewSelect([]string{"contains", "starts with", "ends with", "extension"}, func(sel string) {
				for i := range state.filters {
					if state.filters[i].ID == rid {
						state.filters[i].Mode = sel
						break
					}
				}
				applyAllUI()
			})
			modeSel.SetSelected(rule.Mode)

			valEntry := widget.NewEntry()
			valEntry.SetText(rule.Value)
			valEntry.SetPlaceHolder(`value… e.g. The, Whale, png`)
			valEntry.OnChanged = func(s string) {
				for i := range state.filters {
					if state.filters[i].ID == rid {
						state.filters[i].Value = s
						break
					}
				}
				applyAllUI()
			}

			removeBtn := widget.NewButton("✕", func() {
				next := state.filters[:0]
				for _, r := range state.filters {
					if r.ID != rid {
						next = append(next, r)
					}
				}
				state.filters = append([]FilterRule(nil), next...)
				renderFilters()
				applyAllUI()
			})

			filtersBox.Add(container.NewBorder(nil, nil, nil, removeBtn,
				container.NewGridWithColumns(2, modeSel, valEntry),
			))
		}
		filtersBox.Refresh()
	}

	addFilterBtn := widget.NewButton("+ Add filter", func() {
		state.nextFilterID++
		state.filters = append(state.filters, FilterRule{ID: state.nextFilterID, Mode: "contains", Value: ""})
		renderFilters()
		applyAllUI()
	})

	clearFiltersBtn := widget.NewButton("Clear filters", func() {
		state.filters = nil
		renderFilters()
		applyAllUI()
	})

	// Steps UI
	stepsBox := container.NewVBox()
	var renderSteps func()
	renderSteps = func() {
		stepsBox.Objects = nil
		if len(state.steps) == 0 {
			stepsBox.Add(widget.NewLabel("No rename steps. Add one to preview name changes."))
			stepsBox.Refresh()
			return
		}

		for _, step := range state.steps {
			sid := step.ID

			opSel := widget.NewSelect([]string{
				string(OpRemoveText),
				string(OpReplaceText),
				string(OpInsertBeforeExt),
				string(OpChangeExt),
				string(OpAppend),
				string(OpPrepend),
			}, func(sel string) {
				for i := range state.steps {
					if state.steps[i].ID == sid {
						state.steps[i].Op = RenameOp(sel)
						break
					}
				}
				applyAllUI()
			})
			opSel.SetSelected(string(step.Op))

			a := widget.NewEntry()
			a.SetText(step.A)
			b := widget.NewEntry()
			b.SetText(step.B)

			a.SetPlaceHolder("A")
			b.SetPlaceHolder("B (Replace only)")
			b.Enable()

			switch step.Op {
			case OpRemoveText:
				a.SetPlaceHolder(`text to remove (e.g. vivek)`)
				b.Disable()
			case OpReplaceText:
				a.SetPlaceHolder(`find (e.g. vivek)`)
				b.SetPlaceHolder(`replace with (e.g. Vivek)`)
			case OpInsertBeforeExt, OpAppend:
				a.SetPlaceHolder(`insert (e.g. (awesome))`)
				b.Disable()
			case OpPrepend:
				a.SetPlaceHolder(`prepend (e.g. NEW_)`)
				b.Disable()
			case OpChangeExt:
				a.SetPlaceHolder(`new ext (e.g. xyz or .xyz)`)
				b.Disable()
			}

			a.OnChanged = func(v string) {
				for i := range state.steps {
					if state.steps[i].ID == sid {
						state.steps[i].A = v
						break
					}
				}
				applyAllUI()
			}
			b.OnChanged = func(v string) {
				for i := range state.steps {
					if state.steps[i].ID == sid {
						state.steps[i].B = v
						break
					}
				}
				applyAllUI()
			}

			remove := widget.NewButton("✕", func() {
				next := state.steps[:0]
				for _, s := range state.steps {
					if s.ID != sid {
						next = append(next, s)
					}
				}
				state.steps = append([]RenameStep(nil), next...)
				renderSteps()
				applyAllUI()
			})

			stepsBox.Add(container.NewBorder(nil, nil, nil, remove,
				container.NewVBox(opSel, container.NewGridWithColumns(2, a, b)),
			))
			stepsBox.Add(widget.NewSeparator())
		}
		stepsBox.Refresh()
	}

	addStepBtn := widget.NewButton("+ Add rename step", func() {
		state.nextStepID++
		state.steps = append(state.steps, RenameStep{ID: state.nextStepID, Op: OpReplaceText, A: "", B: ""})
		renderSteps()
		applyAllUI()
	})

	clearStepsBtn := widget.NewButton("Clear steps", func() {
		state.steps = nil
		renderSteps()
		applyAllUI()
	})

	left := container.NewVScroll(container.NewVBox(
		widget.NewLabelWithStyle("Filters", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		matchModeSelect,
		caseSensitiveCheck,
		container.NewHBox(addFilterBtn, clearFiltersBtn),
		widget.NewSeparator(),
		filtersBox,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("Rename preview pipeline", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		container.NewHBox(addStepBtn, clearStepsBtn),
		widget.NewSeparator(),
		stepsBox,

		widget.NewSeparator(),
		widget.NewLabelWithStyle("Tip", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		widget.NewLabel(`Example:
- Remove text: "vivek"
- Insert before extension: "(awesome)"
- Change extension: "xyz"`),
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
		files, err := listAllFiles(state.folderPath, state.recursive)
		if err != nil {
			dialog.ShowError(err, w)
			return
		}
		state.allFiles = files
		state.deselected = map[string]bool{}
		applyAll(state)
		updatePageView()
	})

	loadFolder := func(path string) {
		state.folderPath = path
		selectedFolderLabel.SetText("Folder: " + path)

		files, err := listAllFiles(path, state.recursive)
		if err != nil {
			dialog.ShowError(err, w)
			state.allFiles = nil
			applyAll(state)
			updatePageView()
			return
		}

		state.allFiles = files
		state.deselected = map[string]bool{}
		applyAll(state)
		updatePageView()
		saveRecentFolder(path)
	}

	// Recent folders dropdown — populated from persisted preferences
	initialRecents := getRecentFolders()
	recentOpts := append([]string{"Open recent…"}, initialRecents...)
	recentSelect = widget.NewSelect(recentOpts, func(sel string) {
		if sel == "Open recent…" {
			return
		}
		loadFolder(sel)
	})
	recentSelect.SetSelected("Open recent…")

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

	aboutBtn := widget.NewButton("About", func() {
		dialog.ShowInformation(
			"About File Rename Utility",
			"File Rename Utility helps you filter and preview files in a folder, then bulk-rename them safely.\n\n"+
				"License: CC BY-NC 4.0 | Commercial \n\n"+
				"Download the latest version here:\n"+
				"https://github.com/blackarck/renforge",
			w,
		)
	})

	topBar := container.NewBorder(nil, nil,
		container.NewHBox(selectFolderBtn, recentSelect, refreshBtn, recursiveCheck),
		container.NewHBox(aboutBtn),
		selectedFolderLabel,
	)

	/* -------------------- Layout -------------------- */

	split := container.NewHSplit(left, right)
	split.Offset = 0.38

	root := container.NewBorder(topBar, nil, nil, nil, split)
	w.SetContent(root)

	updatePageView()

	w.ShowAndRun()
}

/* -------------------- Apply Pipeline Helpers -------------------- */

func applyAll(state *AppState) {
	applyFilters(state)
	recomputePreviewCounts(state)
	state.page = 0
}

func applyFilters(state *AppState) {
	if len(state.allFiles) == 0 {
		state.filteredFiles = nil
		return
	}
	state.filteredFiles = filterFilesMulti(state.allFiles, state.filters, state.matchAll, state.caseSensitive)
}

func recomputePreviewCounts(state *AppState) {
	counts := make(map[string]int, len(state.filteredFiles))
	for _, p := range state.filteredFiles {
		if state.deselected[p] {
			continue
		}
		orig := filepath.Base(p)
		prev := applyRenameSteps(orig, state.steps)
		// key by (dir, previewName) so files in different subdirs don't false-conflict
		counts[filepath.Dir(p)+"\x00"+prev]++
	}
	state.previewCounts = counts
}

/* -------------------- Plan / Confirm / Apply -------------------- */

type PlanSummary struct {
	Total        int
	OkCount      int
	Unchanged    int
	Invalid      []string
	Duplicate    []string
	TargetExists []string
}

func buildPlan(state *AppState) ([]RenamePlanItem, PlanSummary) {
	// operate only on selected (non-deselected) files
	var selected []string
	for _, p := range state.filteredFiles {
		if !state.deselected[p] {
			selected = append(selected, p)
		}
	}

	items := make([]RenamePlanItem, 0, len(selected))

	// key: "dir\x00newName" -> count, to detect within-dir conflicts
	dupCount := map[string]int{}
	previewByPath := map[string]string{}

	for _, p := range selected {
		newName := applyRenameSteps(filepath.Base(p), state.steps)
		previewByPath[p] = newName
		dupCount[filepath.Dir(p)+"\x00"+newName]++
	}

	var sum PlanSummary
	sum.Total = len(selected)

	for _, oldPath := range selected {
		oldName := filepath.Base(oldPath)
		newName := previewByPath[oldPath]
		newPath := filepath.Join(filepath.Dir(oldPath), newName)

		it := RenamePlanItem{
			OldPath: oldPath,
			NewPath: newPath,
			OldName: oldName,
			NewName: newName,
			Status:  "ok",
		}

		if newName == oldName {
			sum.Unchanged++
			it.Status = "skip"
			it.Reason = "unchanged"
			items = append(items, it)
			continue
		}

		if reason := invalidNameReason(newName); reason != "" {
			sum.Invalid = append(sum.Invalid, fmt.Sprintf("%s → %s (%s)", oldName, newName, reason))
			it.Status = "skip"
			it.Reason = "invalid: " + reason
			items = append(items, it)
			continue
		}

		if dupCount[filepath.Dir(oldPath)+"\x00"+newName] > 1 {
			sum.Duplicate = append(sum.Duplicate, fmt.Sprintf("%s → %s", oldName, newName))
			it.Status = "skip"
			it.Reason = "conflict: duplicate preview name"
			items = append(items, it)
			continue
		}

		if _, err := os.Stat(newPath); err == nil {
			sum.TargetExists = append(sum.TargetExists, fmt.Sprintf("%s → %s", oldName, newName))
			it.Status = "skip"
			it.Reason = "conflict: target exists on disk"
			items = append(items, it)
			continue
		}

		sum.OkCount++
		items = append(items, it)
	}

	return items, sum
}

func buildConfirmMessage(sum PlanSummary) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("You are about to process %d file(s).\n", sum.Total))
	b.WriteString(fmt.Sprintf("Will rename: %d\n", sum.OkCount))
	b.WriteString(fmt.Sprintf("Unchanged (skipped): %d\n\n", sum.Unchanged))

	if len(sum.Invalid) > 0 {
		b.WriteString("Invalid names (skipped):\n")
		for _, s := range firstN(sum.Invalid, 20) {
			b.WriteString(" - " + s + "\n")
		}
		if len(sum.Invalid) > 20 {
			b.WriteString(fmt.Sprintf(" ... and %d more\n", len(sum.Invalid)-20))
		}
		b.WriteString("\n")
	}

	if len(sum.Duplicate) > 0 {
		b.WriteString("Duplicate preview conflicts (skipped):\n")
		for _, s := range firstN(sum.Duplicate, 20) {
			b.WriteString(" - " + s + "\n")
		}
		if len(sum.Duplicate) > 20 {
			b.WriteString(fmt.Sprintf(" ... and %d more\n", len(sum.Duplicate)-20))
		}
		b.WriteString("\n")
	}

	if len(sum.TargetExists) > 0 {
		b.WriteString("Target already exists on disk (skipped):\n")
		for _, s := range firstN(sum.TargetExists, 20) {
			b.WriteString(" - " + s + "\n")
		}
		if len(sum.TargetExists) > 20 {
			b.WriteString(fmt.Sprintf(" ... and %d more\n", len(sum.TargetExists)-20))
		}
		b.WriteString("\n")
	}

	b.WriteString("Proceed?")
	return b.String()
}

// applyRenames uses a two-phase rename to safely handle circular renames
// (e.g. a→b and b→a). Phase 1 moves every file to a temp name; phase 2
// moves each temp name to its final destination.
func applyRenames(plan []RenamePlanItem) []RenamePlanItem {
	out := make([]RenamePlanItem, len(plan))
	copy(out, plan)

	type staged struct {
		idx     int
		tmpPath string
	}
	var phase2 []staged

	ts := time.Now().UnixNano()

	for i := range out {
		if out[i].Status != "ok" {
			continue
		}
		tmpPath := filepath.Join(filepath.Dir(out[i].OldPath), fmt.Sprintf(".renforge_tmp_%d_%d", ts, i))
		if err := os.Rename(out[i].OldPath, tmpPath); err != nil {
			out[i].Status = "error"
			out[i].Reason = err.Error()
			continue
		}
		phase2 = append(phase2, staged{i, tmpPath})
	}

	for _, s := range phase2 {
		if err := os.Rename(s.tmpPath, out[s.idx].NewPath); err != nil {
			out[s.idx].Status = "error"
			out[s.idx].Reason = err.Error()
			// best-effort restore to original name
			_ = os.Rename(s.tmpPath, out[s.idx].OldPath)
		} else {
			out[s.idx].Status = "renamed"
		}
	}

	return out
}

func buildResultMessage(items []RenamePlanItem, dryRun bool) string {
	var renamed, skipped, errors int
	for _, it := range items {
		switch it.Status {
		case "renamed":
			renamed++
		case "skip":
			skipped++
		case "error":
			errors++
		case "dry-run":
			renamed++
		}
	}

	if dryRun {
		return fmt.Sprintf("Dry run complete.\nWould rename: %d\nSkipped: %d\nErrors: %d", renamed, skipped, errors)
	}
	return fmt.Sprintf("Apply complete.\nRenamed: %d\nSkipped: %d\nErrors: %d", renamed, skipped, errors)
}

/* -------------------- Undo CSV -------------------- */

func writeUndoCSV(wc fyne.URIWriteCloser, plan []RenamePlanItem) error {
	cw := csv.NewWriter(wc)
	defer cw.Flush()
	_ = cw.Write([]string{"old_path", "new_path", "old_name", "new_name", "status", "reason"})
	for _, it := range plan {
		_ = cw.Write([]string{it.OldPath, it.NewPath, it.OldName, it.NewName, it.Status, it.Reason})
	}
	return cw.Error()
}

func overwriteUndoCSV(path string, plan []RenamePlanItem) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	cw := csv.NewWriter(f)
	defer cw.Flush()
	_ = cw.Write([]string{"old_path", "new_path", "old_name", "new_name", "status", "reason"})
	for _, it := range plan {
		_ = cw.Write([]string{it.OldPath, it.NewPath, it.OldName, it.NewName, it.Status, it.Reason})
	}
	return cw.Error()
}

/* -------------------- File listing -------------------- */

func listAllFiles(folder string, recursive bool) ([]string, error) {
	var files []string

	if recursive {
		err := filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := strings.TrimSpace(d.Name())
			if name == "" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(folder)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.TrimSpace(e.Name())
			if name == "" {
				continue
			}
			files = append(files, filepath.Join(folder, name))
		}
	}

	sort.Strings(files)
	return files, nil
}

/* -------------------- Filter engine -------------------- */

func filterFilesMulti(all []string, rules []FilterRule, matchAll bool, caseSensitive bool) []string {
	out := make([]string, 0, len(all))
	for _, full := range all {
		base := filepath.Base(full)
		if matchesRules(base, rules, matchAll, caseSensitive) {
			out = append(out, full)
		}
	}
	sort.Strings(out)
	return out
}

func matchesRules(filename string, rules []FilterRule, matchAll bool, caseSensitive bool) bool {
	if len(rules) == 0 {
		return true
	}

	name := filename
	if !caseSensitive {
		name = strings.ToLower(name)
	}

	ruleMatch := func(r FilterRule) bool {
		val := strings.TrimSpace(r.Value)
		if val == "" {
			return true
		}
		check := name
		v := val
		if !caseSensitive {
			v = strings.ToLower(val)
		}

		switch r.Mode {
		case "contains":
			return strings.Contains(check, v)
		case "starts with":
			return strings.HasPrefix(check, v)
		case "ends with":
			return strings.HasSuffix(check, v)
		case "extension":
			ext := filepath.Ext(filename)
			vx := v
			if !strings.HasPrefix(vx, ".") {
				vx = "." + vx
			}
			if !caseSensitive {
				ext = strings.ToLower(ext)
				vx = strings.ToLower(vx)
			}
			return ext == vx
		default:
			return strings.Contains(check, v)
		}
	}

	if matchAll {
		for _, r := range rules {
			if !ruleMatch(r) {
				return false
			}
		}
		return true
	}

	for _, r := range rules {
		if ruleMatch(r) {
			return true
		}
	}
	return false
}

/* -------------------- Rename pipeline -------------------- */

func applyRenameSteps(original string, steps []RenameStep) string {
	if len(steps) == 0 {
		return original
	}
	name := original

	for _, s := range steps {
		switch s.Op {
		case OpRemoveText:
			if s.A != "" {
				name = strings.ReplaceAll(name, s.A, "")
			}
		case OpReplaceText:
			if s.A != "" {
				name = strings.ReplaceAll(name, s.A, s.B)
			}
		case OpInsertBeforeExt, OpAppend:
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			name = base + s.A + ext
		case OpPrepend:
			base := strings.TrimSuffix(name, filepath.Ext(name))
			ext := filepath.Ext(name)
			name = s.A + base + ext
		case OpChangeExt:
			base := strings.TrimSuffix(name, filepath.Ext(name))
			newExt := strings.TrimSpace(s.A)
			if newExt == "" {
				name = base
			} else {
				if !strings.HasPrefix(newExt, ".") {
					newExt = "." + newExt
				}
				name = base + newExt
			}
		}
	}

	return strings.TrimSpace(name)
}

/* -------------------- Validations -------------------- */

func invalidNameReason(name string) string {
	trim := strings.TrimSpace(name)
	if trim == "" {
		return "empty name"
	}
	if strings.ContainsAny(trim, `<>:"/\|?*`) {
		return "invalid characters"
	}
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true,
		"COM6": true, "COM7": true, "COM8": true, "COM9": true,
		"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true,
		"LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
	}
	base := strings.TrimSuffix(trim, filepath.Ext(trim))
	if reserved[strings.ToUpper(base)] {
		return "reserved filename"
	}
	return ""
}

/* -------------------- Paging helpers -------------------- */

func pageCount(total, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	if total == 0 {
		return 1
	}
	return (total + pageSize - 1) / pageSize
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
