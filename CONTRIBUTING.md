# Contributing to RenForge

Thanks for helping! This page explains how to report problems, suggest ideas, build the app and send changes. The website has the same information: https://renforgeapp.web.app/#support

## Report a bug

Open an issue: https://github.com/blackarck/renforge/issues/new/choose (pick **Bug report**).

The more of this you include, the faster it gets fixed:

- **Operating system** and version (macOS 15 Apple Silicon, Windows 11, Ubuntu 24.04 …)
- **Which build**: the download file name and date, or the commit if you built from source
- **What you did**: the folder setup (a few example file names), the filters and rename steps you used
- **What you expected** and **what happened** instead (a screenshot of the preview helps a lot)
- For rename problems: the **undo log (CSV)** if you saved one. Remove any private file names first.

Please do not paste personal documents or file names you would not want public.

## Suggest a feature

Open an issue and choose **Feature request**. Describe the problem you are trying to solve first (for example "I need to number photos per folder"), then your idea. Real examples of file names before and after are very welcome.

## Build and run

Requirements:

- **Go**: the version in [`go.mod`](go.mod) or newer
- A **C compiler and graphics libraries**, because the UI toolkit ([Fyne](https://developer.fyne.io/started/)) uses them:
  - **macOS**: Xcode command line tools (`xcode-select --install`)
  - **Windows**: a MinGW-w64 gcc, for example from [MSYS2](https://www.msys2.org/)
  - **Linux**: `gcc pkg-config libgl1-mesa-dev xorg-dev libasound2-dev`

```bash
git clone https://github.com/blackarck/renforge.git
cd renforge
go run .                 # run the app
go test ./...            # run the tests
go build -o renforge .   # build a plain binary
```

A plain `go build` has no app icon, and on Windows opens a console window. For a proper app:

```bash
go install fyne.io/tools/cmd/fyne@latest
fyne package -os darwin     # or: windows, linux  (uses FyneApp.toml and icon.png)
```

### Screenshots

The screenshots on the website and in the readme are rendered from the real UI with Fyne's software renderer (no screen access needed). Regenerate them after UI changes:

```bash
SHOT_DIR=/tmp/shots go test -tags screenshots -run TestScreenshots .
```

The scenes (sample folders, steps, filters) are defined in `screenshots_test.go`.

### Project layout

```
renforge.go, rows.go, presets.go, help.go, icon.go   Fyne user interface, in-app help, window icon
internal/rename    rename steps, name validation, plan building, sidecars, two-phase apply
internal/filter    filter rules (contains, starts/ends with, extension, regex)
internal/undo      undo log (CSV) reading, writing and reverting
internal/meta      cached file size, dates and EXIF
internal/preset    saved and built-in presets
docs/              user guide, release and signing notes
```

The rename engine lives in `internal/` and has no UI code, so most changes can be tested without opening a window.

## Send a change

1. **Fork** the repository and create a branch: `git checkout -b fix-short-description`
2. Make a **small, focused** change. Open an issue first for anything large.
3. **Add or update tests** for engine changes (`internal/...`). Table-driven tests with `t.TempDir()` are the pattern used here.
4. Run `gofmt -l .`, `go vet ./...` and `go test ./...`. All must be clean.
5. **Update the docs** when behaviour changes: `docs/USER_GUIDE.md`, the in-app help text in `help.go`, and `readme.md` where relevant.
6. Open a **pull request** that explains *what* changed and *why*, with before/after file-name examples for rename behaviour.

Guiding principles: nothing is renamed without a preview, files are never overwritten, and every rename can be undone. Changes that weaken those guarantees will not be accepted.

## License of contributions

RenForge is available under the terms in [`license.md`](license.md) (CC BY-NC 4.0 for non-commercial use, with a separate commercial license). By opening a pull request you confirm you have the right to submit the code and that it may be distributed under the project's license terms. If that is a problem for you, please open an issue to discuss it first.
