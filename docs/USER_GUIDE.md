# RenForge User Guide

RenForge renames many files at once. You describe the change, **see the result before anything happens**, and can undo it afterwards.

- [The window at a glance](#the-window-at-a-glance)
- [Step by step: your first rename](#step-by-step-your-first-rename)
- [Filters](#filters)
- [Rename steps](#rename-steps)
- [Numbering, regex and presets](#numbering-regex-and-presets)
- [Photos, image sequences and related files](#photos-image-sequences-and-related-files)
- [Choosing files](#choosing-files)
- [Dry run, Apply and safety checks](#dry-run-apply-and-safety-checks)
- [Undo](#undo)
- [Recipes](#recipes)
- [Troubleshooting](#troubleshooting)
- [FAQ](#faq)

> In the app, click **Help** (or press **F1**) for a short version of this guide. It opens automatically the first time you start RenForge.

---

## The window at a glance

| Area | What it does |
|---|---|
| **Top bar** | **A− / A+** (text size) and **Auto / Light / Dark** (colours), both remembered, *Select Folder…* (or just **drag a folder onto the window**), *Recent…* (last 5 folders), *Refresh*, *Include subfolders*, *Help*, *About* |
| **Left: 1. Filters** | Choose *which* files are affected (optional) |
| **Left: 3. Options** | *Also rename related files* (sidecars, RAW+JPEG pairs); **Edit file types…** opens the list of extensions |
| **Right: Order** | Sort the list by name, date or size; numbering follows this order |
| **Left: 2. Rename steps** | Describe *how* names change; each step has a short explanation and example, an on/off checkbox and ▲ ▼ buttons. **Presets…** saves and loads whole setups |
| **Right: Preview** | Original name next to the new name, with a checkbox per file and ⚠ warnings The changed part of each new name is highlighted; drag the divider in the column header to resize the columns. A summary line shows what will rename, what is invalid, duplicated or taken, and what stays unchanged; click a problem count to see just those files. |
| **Bottom** | *Dry run*, *Create undo log*, **Undo…** and **Apply** |

## Step by step: your first rename

Goal: turn `IMG_0042.JPG`, `IMG_0043.JPG`, … into `Holiday_0042.jpg`, `Holiday_0043.jpg`, …

1. Click **Select Folder…** and pick the folder.
2. Under *Filters* click **+ Add filter**, choose **extension** and type `jpg` (case does not matter).
3. Under *Rename steps* click **+ Add rename step**:
   1. **Replace text**: find `IMG_`, replace with `Holiday_`.
   2. Add another step, **Change extension**: `jpg`.
4. Look at the Preview column. Every row shows the new name. Untick anything you want to keep.
5. Leave **Dry run** ticked and click **Apply** → **Proceed**. Choose where to save the undo log. RenForge reports what it *would* do.
6. Happy? Untick **Dry run** and click **Apply** again. Files are renamed.

## Filters

Filters decide which files appear in the preview. With no filters every file in the folder is included.

| Mode | Matches when… | Example value |
|---|---|---|
| contains | the name contains the text | `holiday` |
| starts with | the name begins with the text | `IMG_` |
| ends with | the name ends with the text (extension included) | `_edit.jpg` |
| extension | the extension equals the value | `png`, `.png`, `tar.gz` |
| regex | the name matches a [regular expression](#regex-replace-and-regex-filter) | `^IMG_\d{4}` |

- **Match ALL (AND)**: a file must satisfy every filter. **Match ANY (OR)**: at least one.
- **Case sensitive** is off by default.
- A filter with an empty value is ignored.
- Filters look at the file *name* only, never at the folder path.
- An invalid regular expression is flagged in red and matches nothing.

## Rename steps

Steps are a pipeline: the output of step 1 feeds step 2, and so on. Each step shows an **Example** line (the first selected file, before → after that step), and the arrow button collapses a step to one line (*Collapse all* / *Expand all* above the steps). Use ▲ ▼ to reorder, the checkbox to switch a step off without deleting it, and the trash button to remove it. Order matters: *Prepend* then *Change case* is not the same as the reverse.

| Step | Does | Example |
|---|---|---|
| **Remove text** | Deletes the text everywhere in the name | `IMG_0042.jpg` → `0042.jpg` (text: `IMG_`) |
| **Replace text** | Replaces A with B everywhere. Empty B deletes | `my_pic.jpg` → `my pic.jpg` (A: `_`, B: a space) |
| **Insert before extension** | Adds text before the extension | `cut.mp4` → `cut_final.mp4` |
| **Append** | Same as *Insert before extension* | `cut.mp4` → `cut_v2.mp4` |
| **Prepend** | Adds text at the very start | `beach.jpg` → `2024_beach.jpg` |
| **Change extension** | Replaces the extension; empty removes it | `a.jpeg` → `a.png` |
| **Regex replace** | Pattern find & replace with groups (`$1`) | `IMG_(\d+)` → `Holiday-$1` |
| **Change case** | lowercase, UPPERCASE, Title Case, Sentence case, snake_case, kebab-case; or just the extension's case | `My Photo.JPG` → `my-photo.JPG` |
| **Clean up** | Collapse spaces, trim, remove accents, dots/underscores to spaces, drop `(1)` / `- Copy` | `report (2).pdf` → `report.pdf` |
| **Number files** | Sequence numbers from a template | `beach.jpg` → `Trip_0001.jpg` |
| **Rename from template** | Name from the photo's capture date, camera, number, folder… | `IMG_4021.CR2` → `2024-06-01_003.CR2` |
| **Shift frame numbers** | Adds an offset to the frame number at the end of the name | `shot.0001.exr` → `shot.1001.exr` |

Details worth knowing:

- Rename steps are **case sensitive** (except *Change case*, which is about case by definition): *Replace `img`* does not touch `IMG`.
- Extensions are understood properly: `.tar.gz`, `.tar.bz2`, `.tar.xz`, `.tar.zst` count as **one** extension; a dotfile like `.bashrc` has no extension.
- RenForge does **not** trim spaces for you. A result that starts or ends with a space is flagged as invalid so you notice it.
- Only the name changes; files never move to another folder.
- A red message under a step means a setting is wrong (for example an invalid regex); the preview skips that step until it is fixed.

## Numbering, regex and presets

### Number files

Gives every **selected** file a sequence number, in the order shown (natural order: `img2` before `img10`). Untick a file and the others are renumbered.

- **A (template):** `{n}` = number, `{n:3}` = zero-padded to 3 digits, `{name}` = original name without extension. The extension is kept automatically. Empty means `{name}_{n:3}`.
- **B (start, step):** `1` starts at 1; `100,10` gives 100, 110, 120, …

| Template | 3rd file `beach.jpg` becomes |
|---|---|
| `Trip_{n:4}` | `Trip_0003.jpg` |
| `{n:2}-{name}` | `03-beach.jpg` |
| `{name}_{n}` | `beach_3.jpg` |

Numbering runs across the whole selection, even when *Include subfolders* is on. If you need each folder numbered separately, process one folder at a time.

### Regex replace and regex filter

A regular expression (regex) describes a pattern. In **Regex replace**, A is the pattern and B the replacement. `$1`, `$2`… insert the parts captured by brackets in the pattern. The whole name, extension included, is matched. Start the pattern with `(?i)` to ignore case. The **regex** filter uses the same syntax and follows the *Case sensitive* box. The syntax is Go's [RE2](https://github.com/google/re2/wiki/Syntax).

| Pattern (A) | Replacement (B) | Example |
|---|---|---|
| `IMG_(\d+)` | `Holiday-$1` | `IMG_0042.jpg` → `Holiday-0042.jpg` |
| `\s+` | `_` | `my  file.txt` → `my_file.txt` |
| `^(\d{4})-(\d{2})-(\d{2})` | `$3.$2.$1` | `2024-06-01_x.jpg` → `01.06.2024_x.jpg` |
| `\[.*?\]` | *(empty)* | `a [1080p] b.mp4` → `a  b.mp4` |

### Presets

Click **Presets…**:

- **Load** a built-in (*Clean up downloads*, *Lowercase names and extensions*, *kebab-case names*, *Photos: Trip_0001*, *Add number suffix: name_001*) or one of your own. Loading replaces the current filters and steps.
- **Save current as…** stores your filters, steps and match options under a name. Saving under an existing name asks before replacing it.
- **Delete** removes your own presets (built-ins cannot be deleted).

Your presets are plain JSON files in `renforge/presets` inside your user configuration folder (on macOS `~/Library/Application Support`, on Windows `%AppData%`, on Linux `~/.config`). You can copy them to another computer.

## Choosing files

- Every matching file has a checkbox. Untick files to leave them alone.
- **Select All** / **Deselect All** act on the files currently listed (all matches, or only the changed / problem rows when you use *Show*).
- **Show: All / Changed / Problems** (above the list) narrows the preview. *Changed* hides files whose name stays the same or that are unticked; *Problems* shows only files with a ⚠ (invalid name, duplicate name, or a name already taken by a file that is not being renamed). The buttons show live counts, and *Problems* turns amber when there is something to fix. Counts and filtering use the scanned file list; the final check against the disk happens when you press Apply.
- The header shows the numbers: `Showing 1–10 of 42 matches · 38 selected · 100 total files`.
- Long file names wrap onto several lines instead of being cut off.
- The list scrolls smoothly even for tens of thousands of files, and the folder is scanned in the background (a new scan cancels the previous one).
- Rows marked `(unchanged)` already have the target name and are skipped.

## Dry run, Apply and safety checks

When you click **Apply**, RenForge checks every selected file and skips any that would cause trouble:

| Problem | Meaning |
|---|---|
| **Invalid name** | Empty; contains `\ / : * ? " < > |` or control characters; starts with a space; ends with a space or dot; longer than 255 bytes; or a reserved Windows name (`CON`, `NUL`, `COM1`…). These rules keep files usable on any system |
| **Conflict** | Two selected files in the same folder would get the same name |
| **Target exists** | A different file that is *not* being renamed already has the new name. (A target held by a file that is itself being renamed away, as in `+1` frame shifts or swaps, is fine.) |

Things that are **allowed**:

- Swapping names (`a.txt` ↔ `b.txt`).
- Changing only the letter case (`IMG.jpg` → `img.jpg`), even on Windows and macOS where names are case-insensitive.

**How it renames.** Each file first moves to a temporary name (`.renforge_tmp_…`), then to its final name. Existing files are never overwritten. If a file cannot be renamed it goes back to its old name; in the very unlikely case that fails too, the result dialog lists the file left under its temporary name.

**Dry run** is ticked by default and changes nothing. A dry run can still save the plan as CSV.

## Undo

Keep **Create undo log (CSV)** ticked. RenForge asks where to save the log (it suggests the folder you are working in) and writes it after the rename, so it records what *really* happened.

To undo, click **Undo…**:

1. RenForge offers the **most recent log**, or lets you choose any log file.
2. It shows how many renames can be restored, and lists any it cannot:
   - **Not found**: the renamed file was moved or deleted since.
   - **In use again**: another file now has the original name.
3. Confirm, and the original names come back (using the same safe two-phase method).

A log from a dry run contains no completed renames, so there is nothing to undo.

The CSV columns are `old_path, new_path, old_name, new_name, status, reason`. It is plain text you can open in a spreadsheet.

## Photos, image sequences and related files

### Rename from template (capture date, camera, EXIF)

Builds the whole name from tokens. The **capture date** is read from the EXIF data inside the picture (JPEG and TIFF-based RAW formats such as CR2, NEF, DNG, ARW). Files without EXIF (and HEIC/CR3, whose metadata RenForge cannot read yet) use their *modified* date, so the template still works.

| Token | Meaning |
|---|---|
| `{date}` / `{date:YYYY-MM-DD_HHmmss}` | capture date; format letters `YYYY YY MM DD HH mm ss` (default `YYYY-MM-DD`) |
| `{modified}` / `{modified:YYYY}` | file modification date, same format letters |
| `{n}` / `{n:3}` | sequence number, padded to 3 digits |
| `{name}` | original name without extension |
| `{parent}` | name of the folder the file is in |
| `{camera}` `{make}` `{lens}` `{iso}` | from EXIF (shows `unknown` when missing) |

The extension is kept automatically. Start and step work as in *Number files* (`B` = `1` or `100,10`). With several cameras or bursts, keep `{n}` in the template so names stay unique; RenForge warns if it is missing. Tick **Restart numbering in each folder** to number every folder from the start.

Reading dates and EXIF can take a moment for large folders; it happens in the background (a progress bar appears above the list) and the preview fills in when it is done.

### Shift frame numbers (image sequences)

For `shot.0001.exr … shot.0240.exr`: **A** is an offset (`+1000`, `-1`), **B** an optional padding (empty keeps the current width). Only the last number in the name changes. `shot.0001.exr` + `+1000` → `shot.1001.exr`. Shifts where each file takes the name of the next (`+1`) are fine: RenForge moves all files out of the way first. If a file that is *not* part of the batch occupies a target, the whole affected chain is skipped and reported, so nothing is half-shifted. Undo restores such shifts correctly too.

### Related files (sidecars and RAW+JPEG pairs)

Under **Options**, tick *Also rename related files*. For every file that gets a new base name, RenForge also renames files next to it that:

- have the **same base name and another extension from your list** (`IMG_001.CR2` → `IMG_001.xmp`, `IMG_001.JPG`), or
- are **Adobe-style sidecars of the full name** (`IMG_001.CR2.xmp`).

The preview shows `(+2 related)`. Rules:

- A group is **all or nothing**: if any member would conflict, none of that group is renamed.
- Files that are themselves selected are renamed once, as normal files.
- Changing only the extension does not touch related files.
- The extension list is editable (default: `xmp, aae, dop, pp3, jpg, jpeg, tif, tiff, png, heic, cr2, cr3, nef, arw, dng, raf, orf, rw2, mov, thm`). The option and the list are remembered.
- Related files only come from the folders that were scanned, so tick *Include subfolders* if they live in sub-folders.

### Order of files

The **Order** menu sorts by name (natural), date (capture date, else modified) or size. Numbering follows the order shown, so *Date, oldest first* + *Rename from template* with `{n}` numbers photos in the order they were taken.

## Recipes

| I want to… | Filters | Steps |
|---|---|---|
| Replace underscores with spaces | – | Replace `_` → ` ` |
| Remove “ (1)” copies | contains ` (1)` | Remove text ` (1)` |
| Add a date prefix | – | Prepend `2024-06-01_` |
| Mark finals | extension `mp4` | Insert before extension `_final` |
| Convert `.jpeg` to `.jpg` | extension `jpeg` | Change extension `jpg` |
| Strip a download-site tag | contains `[site.com]` | Remove text `[site.com]` |
| Lower-case an extension | extension `JPG` (case sensitive on) | Change case → *lowercase extension* |
| Number photos | extension `jpg` | Number files `Trip_{n:4}` |
| Name photos by date | extension `jpg` (order: *Date, oldest first*) | Rename from template `{date:YYYY-MM-DD}_{n:3}` |
| Name by exact time | – | Preset *Photos by date and time* |
| RAW + XMP + JPEG together | extension `cr2`, option *related files* on | any step, e.g. Prepend `Trip_` |
| Start a render sequence at 1001 | extension `exr` | Shift frame numbers `+1000` |
| Tidy a download | – | Preset *Clean up downloads* |
| kebab-case everything | – | Change case → *kebab-case* |

## Troubleshooting

**The preview is empty.** Check that a folder is selected, that filters aren’t too strict, and tick *Include subfolders* if the files are inside sub-folders.

**⚠ conflict.** Two files would get the same name. Change a step so names stay unique, or untick one of the files.

**⚠ target exists.** A file with the new name is already in that folder. Rename or move it first.

**Files were changed by another program.** Click **Refresh**.

**“Cannot undo”.** The files in the log were moved, deleted or renamed again. Nothing was changed.

**A `.renforge_tmp_…` file is left behind.** This only happens if a rename failed and the file could not be put back. The result dialog names the file; rename it by hand to what it should be (the undo log has the original name).

## FAQ

**Will RenForge change the contents of my files?** No, only their names.

**Does it rename folders?** No, only files. Folders are listed neither in the preview nor renamed.

**Does it change the photo itself or its EXIF?** No. EXIF is only read (never written) to build names.

**Does it send data anywhere?** No. RenForge works entirely on your computer. See [PRIVACY.md](../PRIVACY.md).

**Where are settings stored?** The app remembers the window size, the width of the left panel and of the preview columns, text size and colour mode, your recent folders, the location of your last undo log, and whether you have seen the first-run help. Presets are stored as JSON files in your user configuration folder. The *related files* option and its extension list are remembered too. Nothing else is stored.
