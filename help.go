package main

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"renforge/internal/filter"
	"renforge/internal/rename"
)

// opHelp explains each rename step with a concrete example. It is shown under
// the step in the UI and reused in the Help dialog.
var opHelp = map[rename.Op]string{
	rename.OpRemoveText:      `Deletes every occurrence of the text. "IMG_" turns IMG_0042.jpg into 0042.jpg.`,
	rename.OpReplaceText:     `Replaces every occurrence of "A" with "B". Leave B empty to delete. Replacing "_" with " " turns my_photo.jpg into my photo.jpg.`,
	rename.OpInsertBeforeExt: `Adds text at the end of the name, before the extension. "_final" turns cut.mp4 into cut_final.mp4.`,
	rename.OpAppend:          `Same as "Insert before extension": adds text after the name, before the extension.`,
	rename.OpPrepend:         `Adds text at the very start. "2024_" turns beach.jpg into 2024_beach.jpg.`,
	rename.OpChangeExt:       `Replaces the extension (with or without the dot). "png" turns a.jpeg into a.png. Leave empty to remove the extension. ".tar.gz" counts as one extension.`,
	rename.OpRegex:           `Advanced find & replace with a regular expression. B can use groups: $1, $2. Example: A = IMG_(\d+)  B = Holiday-$1. Add (?i) at the start to ignore case.`,
	rename.OpCase:            `Changes letter case of the name (the extension is untouched unless you pick an "extension" option).`,
	rename.OpCleanup:         `One-click tidy-ups: collapse double spaces, trim spaces, strip accents (café becomes cafe), dots/underscores to spaces, or drop "(1)" / "- Copy" markers.`,
	rename.OpTemplate:        `Builds the whole name from a template: {date:YYYY-MM-DD} (photo capture date, else file date), {n:3} number, {name}, {parent} folder, {camera}, {make}, {lens}, {iso}, {modified}. B = start or start,step. Empty template = {date:YYYY-MM-DD}_{n:3}.`,
	rename.OpFrames:          `For image sequences: adds an offset to the frame number at the end of the name. A = offset (+1000, -1), B = padding (empty keeps it). shot.0001.exr with +1000 becomes shot.1001.exr. Frames that already exist are handled safely.`,
	rename.OpNumber:          `Numbers the files in the order shown. A = template: {n} = number, {n:3} = padded (001), {name} = original name. Empty = {name}_{n:3}. B = start or start,step (e.g. 1 or 100,10).`,
}

// stepPlaceholders returns the input hints for a step's two fields and whether
// the second field (B) is used by that operation.
func stepPlaceholders(op rename.Op) (a, b string, bUsed bool) {
	switch op {
	case rename.OpRemoveText:
		return `text to remove (e.g. IMG_)`, `(not used)`, false
	case rename.OpReplaceText:
		return `find (e.g. _)`, `replace with (e.g. a space)`, true
	case rename.OpRegex:
		return `pattern (e.g. IMG_(\d+))`, `replacement (e.g. Holiday-$1)`, true
	case rename.OpInsertBeforeExt, rename.OpAppend:
		return `text to add (e.g. _final)`, `(not used)`, false
	case rename.OpPrepend:
		return `text to add (e.g. 2024_)`, `(not used)`, false
	case rename.OpChangeExt:
		return `new extension (e.g. png)`, `(not used)`, false
	case rename.OpCase, rename.OpCleanup:
		return ``, `(not used)`, false
	case rename.OpNumber:
		return `template (empty = {name}_{n:3})`, `start or start,step (e.g. 1)`, true
	case rename.OpTemplate:
		return `template (empty = {date:YYYY-MM-DD}_{n:3})`, `start or start,step (e.g. 1)`, true
	case rename.OpFrames:
		return `offset (e.g. +1000 or -1)`, `padding (empty = keep)`, true
	}
	return "A", "B", true
}

// filterHelp explains each filter mode.
var filterHelp = map[string]string{
	filter.Contains:   `The file name contains the text anywhere.`,
	filter.StartsWith: `The file name begins with the text.`,
	filter.EndsWith:   `The file name ends with the text (including the extension).`,
	filter.Extension:  `The extension equals the value: "png" or ".png"; "tar.gz" also works.`,
	filter.Regex:      `The name matches a regular expression, e.g. ^IMG_\d{4} . Case follows the "Case sensitive" box.`,
}

const sidecarHelp = `Renaming IMG_001.CR2 also renames IMG_001.xmp, IMG_001.CR2.xmp and IMG_001.JPG next to it, so edits and RAW+JPEG pairs stay together. Edit the list of extensions if needed.`

const welcomeMarkdown = `## Welcome to RenForge

Rename many files at once — and see exactly what will happen before anything changes.

1. Click **Select Folder…** (top left).
2. *(Optional)* Add a **filter** to work on only some files.
3. Add one or more **rename steps** on the left, or load a ready-made one with **Presets…**
4. Check the **Preview** column on the right. Untick files you want to leave alone.
5. Click **Apply**. *Dry run* is on by default, so nothing is renamed until you untick it.

Press **F1** or click **Help** at any time.`

const helpQuickStart = `## Quick start

1. **Select a folder** with *Select Folder…* or pick one from *Open recent…*. Tick *Include subfolders* to also work on files inside sub-folders.
2. **Filter** (optional): narrow down which files are affected, for example *extension = jpg*.
3. **Add rename steps**: each step changes the name a little; steps run top to bottom. Use the arrows to reorder, the checkbox to switch a step off, and **Presets…** to save or reuse a whole setup.
4. **Check the preview**: the right-hand list shows the original name and the new name. Warnings (⚠) explain anything that cannot be renamed. Use **Show: Changed / Problems** to look only at what will change or what needs fixing.
5. **Choose which files** to change with the checkboxes, *Select All* and *Deselect All*.
6. **Apply**. With *Dry run* ticked (the default) RenForge only shows what it *would* do. Untick it to really rename.

### Made a mistake?
Keep *Create undo log* ticked, then use the **Undo…** button to put the original names back.`

const helpFilters = `## Filters

Filters decide **which files** are listed and renamed. With no filters, every file in the folder is included.

| Mode | Matches when… | Example |
|---|---|---|
| contains | the name contains the text | *holiday* |
| starts with | the name begins with the text | *IMG_* |
| ends with | the name ends with the text | *_edit.jpg* |
| extension | the extension is equal | *png*, *.png*, *tar.gz* |
| regex | the name matches a regular expression | *^IMG_\d{4}* |

- **Match ALL (AND)**: a file must satisfy every filter.
- **Match ANY (OR)**: a file must satisfy at least one filter.
- **Case sensitive**: off by default, so *PNG* and *png* are the same.
- A filter with an empty value is ignored.
- Only the file *name* is checked, never the folder path.
- An invalid regular expression is flagged in red and matches nothing.`

const helpSteps = `## Rename steps

Steps form a **pipeline**: the result of step 1 is the input of step 2, and so on. Change the order of operations by removing and re-adding steps.

| Step | What it does | Example |
|---|---|---|
| Remove text | Deletes the text everywhere | *IMG_0042.jpg* → *0042.jpg* |
| Replace text | Replaces A with B everywhere | *my_pic.jpg* → *my pic.jpg* |
| Insert before extension | Adds text before the extension | *cut.mp4* → *cut_final.mp4* |
| Append | Same as above | *cut.mp4* → *cut_v2.mp4* |
| Prepend | Adds text at the start | *beach.jpg* → *2024_beach.jpg* |
| Change extension | Replaces the extension | *a.jpeg* → *a.png* |
| Regex replace | Pattern find & replace with groups | *IMG_(\d+)* → *Holiday-$1* |
| Change case | lowercase, UPPERCASE, Title Case, Sentence case, snake_case, kebab-case, or the extension's case | *My Photo.JPG* → *my-photo.JPG* |
| Clean up | Collapse spaces, trim, remove accents, dots/underscores to spaces, drop "(1)" / "- Copy" | *report (2).pdf* → *report.pdf* |
| Number files | Sequence numbers from a template | *beach.jpg* → *Trip_0001.jpg* |

### Good to know
- Each step has a checkbox (switch it off without deleting it) and ▲ ▼ buttons to change the order. Order matters: *Prepend* then *Change case* gives a different result than the other way round.
- Text matching is **case sensitive** in rename steps (*A* is not *a*).
- Extensions such as **.tar.gz** are treated as a single extension, and dotfiles such as **.bashrc** have none.
- RenForge never trims spaces for you. A name that starts or ends with a space is flagged instead.
- Steps are applied to the **file name only**; files never move to another folder.`

const helpAdvanced = `## Numbering, regex and presets

### Number files
Gives every selected file a sequence number **in the order shown** (natural order: *img2* comes before *img10*). Unticking a file renumbers the others.

- **A (template)**: *{n}* = number, *{n:3}* = padded to 3 digits (*007*), *{name}* = the original name without extension. The extension is kept automatically. Empty means *{name}_{n:3}*.
- **B (start, step)**: *1* starts at 1; *100,10* gives 100, 110, 120…

| Template | Result for beach.jpg (3rd file) |
|---|---|
| *Trip_{n:4}* | Trip_0003.jpg |
| *{n:2}-{name}* | 03-beach.jpg |
| *{name}_{n}* | beach_3.jpg |

### Rename from template (dates, camera, EXIF)
Builds the whole name from tokens. Perfect for photos: the **capture date** comes from the EXIF data inside the picture (JPEG and TIFF-based RAW such as CR2, NEF, DNG, ARW). Files without it use their *modified* date.

| Token | Meaning |
|---|---|
| *{date:YYYY-MM-DD}* | capture date (format letters: YYYY YY MM DD HH mm ss) |
| *{modified}* | file modification date, same format letters |
| *{n:3}* | sequence number padded to 3 digits |
| *{name}* / *{parent}* | original name / name of the folder |
| *{camera}* *{make}* *{lens}* *{iso}* | from EXIF; "unknown" if missing |

Example: *{date:YYYYMMDD_HHmmss}_{camera}* turns IMG_4021.CR2 into 20240601_140509_EOS R5.CR2. Tick *Restart numbering in each folder* to number every folder from the start. File details are read in the background; the preview fills in when done.

### Image sequences (VFX / renders)
**Shift frame numbers** adds an offset to the number at the end of the name: *shot.0001.exr* with offset *+1000* becomes *shot.1001.exr*; set padding to change the width. Shifts such as +1 (each file takes the name of the next) are handled safely.

### Related files (sidecars)
Tick **Also rename related files** under *Options*. When a file is renamed, files next to it with the same base name (*.xmp*, *.JPG*, *.CR2*, …, editable list) or the Adobe-style *IMG.CR2.xmp* are renamed with it. All files of a group are renamed, or none if one of them has a problem.

### Order of files
Use **Order** above the list to sort by name, date (capture date, else modified) or size. Numbering follows this order.

### Regex replace and regex filter
A regular expression describes a *pattern*. In **Regex replace**, A is the pattern and B the replacement; **$1**, **$2** insert the parts matched in brackets.

| Pattern (A) | Replacement (B) | Example |
|---|---|---|
| *IMG_(\d+)* | *Holiday-$1* | IMG_0042.jpg → Holiday-0042.jpg |
| *\s+* | *_* | my  file.txt → my_file.txt |
| *^(\d{4})-(\d{2})-(\d{2})* | *$3.$2.$1* | 2024-06-01_x.jpg → 01.06.2024_x.jpg |
| *\[.*?\]* | (empty) | a [1080p] b.mp4 → a  b.mp4 |

Start the pattern with *(?i)* to ignore case. The *regex* filter works the same way and follows the *Case sensitive* box. The whole file name (extension included) is matched.

### Presets
Click **Presets…** to load a ready-made setup (such as *Clean up downloads*), or save your current filters and steps under a name. Loading a preset replaces the current filters and steps. Saved presets are plain JSON files in your user configuration folder (*renforge/presets*).

### Large folders
The preview list scrolls smoothly even with tens of thousands of files, and folders are scanned in the background. Starting a new scan cancels the previous one.`

const helpSafety = `## Safety, conflicts and undo

### What RenForge checks before renaming
A file is **skipped** (never half-renamed) when its new name would be:

- **Invalid**: empty, contains \ / : * ? " < > |, ends with a dot or space, is longer than 255 bytes, or is a reserved Windows name such as CON or NUL. These rules keep your files usable on every system.
- **A duplicate**: two selected files in the same folder would get the same name.
- **Already taken**: a different file with that name already exists.

Changing only the **letter case** (IMG.jpg → img.jpg) is allowed, even on Windows and macOS.

### How renaming is done
Files are first moved to temporary names and then to their final names, so swaps (a↔b) work and nothing is ever overwritten. If a file cannot be renamed it is put back, and the result dialog tells you if any file is left under a ".renforge_tmp_…" name.

### Dry run
Ticked by default. RenForge builds the plan and reports what *would* happen, without touching any file. Untick it, and click **Apply** again, to rename for real.

### Undo log and the Undo… button
With *Create undo log (CSV)* ticked, RenForge saves a list of every rename. Later, click **Undo…** and choose that file (the last one is offered automatically) to restore the original names. Undo skips files that were moved, deleted or whose original name is in use again, and tells you which.

Tip: keep the log; it is also a handy record of what changed.`

const helpTips = `## Tips and troubleshooting

- **Nothing shows in the preview?** Check that a folder is selected, that filters are not too strict, and whether the files are in sub-folders (tick *Include subfolders*).
- **A file shows ⚠ conflict?** Two files would get the same name. Make the name more unique, or untick one.
- **⚠ target exists?** A file with the new name is already in that folder.
- **Files changed outside RenForge?** Click **Refresh**.
- **Try it safely first.** Copy a few files into a test folder, keep *Dry run* ticked, and experiment.
- **Order of files.** Files are sorted in natural order (*img2* before *img10*), and numbering follows this order.
- **Too many rows?** *Show → Problems* lists only files with a ⚠; *Changed* hides files that stay the same.
- **Red text under a step** means a setting is wrong (for example an invalid regex). The preview ignores that step until it is fixed.
- **Long file names** wrap onto several lines in the preview. **A−** / **A+** (top right) make all text smaller or larger, and the **Auto / Light / Dark** button switches the colours. Both are remembered, as are the window size and the width of the left panel.
- **Changes are highlighted** in the Preview column. Drag the thin divider in the column header to give the Original or the Preview column more room.
- **Summary line:** under *Show* RenForge counts what will rename, what is invalid, duplicated or taken, and what stays unchanged. Click a problem count to see just those files.
- **Each step shows an example** with the first selected file (before → after that step). Use the arrow button to collapse a step to one line.
- **Shortcuts:** F1 opens this help, Esc closes Help and About. You can also **drag a folder** onto the window to open it.

### Planned
Photo (EXIF) tokens, sidecar files (.xmp), frame-sequence support and a command-line mode.`

// showHelp opens the tabbed help dialog. tab selects the initial tab (0 = quick start).
func showHelp(w fyne.Window, tab int) {
	page := func(md string) fyne.CanvasObject {
		rt := widget.NewRichTextFromMarkdown(md)
		rt.Wrapping = fyne.TextWrapWord
		return container.NewVScroll(rt)
	}

	tabs := container.NewAppTabs(
		container.NewTabItem("Quick start", page(helpQuickStart)),
		container.NewTabItem("Filters", page(helpFilters)),
		container.NewTabItem("Rename steps", page(helpSteps)),
		container.NewTabItem("Numbering & regex", page(helpAdvanced)),
		container.NewTabItem("Safety & undo", page(helpSafety)),
		container.NewTabItem("Tips", page(helpTips)),
	)
	if tab >= 0 && tab < len(tabs.Items) {
		tabs.SelectIndex(tab)
	}

	content := container.NewBorder(
		container.NewVBox(brandHeader("File Rename Utility · Help"), widget.NewSeparator()),
		nil, nil, nil, tabs)
	d := dialog.NewCustom("RenForge Help", "Close", content, w)
	d.Resize(fyne.NewSize(760, 580))
	showInfoDialog(d)
}

const (
	repoURL    = "https://github.com/blackarck/renforge"
	websiteURL = "https://renforgeapp.web.app"
)

// showAbout opens the About dialog with the RenForge logo and name.
func showAbout(w fyne.Window) {
	link := func(text, raw string) fyne.CanvasObject {
		u, err := url.Parse(raw)
		if err != nil {
			return widget.NewLabel(text)
		}
		h := widget.NewHyperlink(text, u)
		h.Alignment = fyne.TextAlignCenter
		return h
	}
	center := func(text string) *widget.Label {
		l := widget.NewLabelWithStyle(text, fyne.TextAlignCenter, fyne.TextStyle{})
		l.Wrapping = fyne.TextWrapWord
		return l
	}

	body := container.NewVBox(
		container.NewCenter(brandImage(88)),
		brandTitle("File Rename Utility", true),
		widget.NewSeparator(),
		center("Filter files, preview every change, then bulk-rename them safely. Undo any time."),
		center("License: CC BY-NC 4.0 for non-commercial use · commercial license available"),
		link("renforgeapp.web.app", websiteURL),
		link("github.com/blackarck/renforge", repoURL),
	)
	d := dialog.NewCustom("About RenForge", "Close", body, w)
	d.Resize(fyne.NewSize(460, 420))
	showInfoDialog(d)
}
